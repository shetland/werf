package cleaning

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type ScanReference struct {
	*plumbing.Reference
	Commit *object.Commit
	// TODO: DepthLimit       int
	ReachedCommitsLimit int
}

func getScanReferences(r *git.Repository) ([]*ScanReference, error) {
	rs, err := r.References()
	if err != nil {
		return nil, fmt.Errorf("get repository references failed: %s", err)
	}

	var refs []*ScanReference
	if err := rs.ForEach(func(reference *plumbing.Reference) error {
		n := reference.Name()

		// Get all remote branches and tags
		if !(n.IsRemote() || n.IsTag()) {
			return nil
		}

		// Use only origin upstream
		if n.IsRemote() && !strings.HasPrefix(n.Short(), "origin/") {
			return nil
		}

		refHash := reference.Hash()
		if n.IsTag() {
			revHash, err := r.ResolveRevision(plumbing.Revision(n.Short()))
			if err != nil {
				return fmt.Errorf("resolve revision %s failed: %s", n.Short(), err)
			}

			refHash = *revHash
		}

		refCommit, err := r.CommitObject(refHash)
		if err != nil {
			return fmt.Errorf("commit object failed: %s", err)
		}

		refs = append(refs, &ScanReference{
			Reference: reference,
			Commit:    refCommit,
		})

		return nil
	}); err != nil {
		return nil, err
	}

	// Sort by committer when
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Commit.Committer.When.After(refs[j].Commit.Committer.When)
	})

	// Split branches and tags references
	var branchesRefs, tagsRefs []*ScanReference
	for _, ref := range refs {
		if ref.Name().IsTag() {
			tagsRefs = append(tagsRefs, ref)
		} else {
			branchesRefs = append(branchesRefs, ref)
		}
	}

	// TODO: Skip by regexps

	// Skip by committer when
	//skipByPeriod := func(refs []*ScanReference, period time.Duration) (result []*ScanReference) {
	//	for _, ref := range refs {
	//		if ref.Commit.Committer.When.Before(time.Now().Add(-period)) {
	//			logboek.Debug.LogF("Reference %s skipped by period\n", ref.Name().Short())
	//			continue
	//		}
	//
	//		result = append(result, ref)
	//	}
	//
	//	return
	//}
	//
	//branchesRefs = skipByPeriod(branchesRefs, period)
	//tagsRefs = skipByPeriod(tagsRefs, period)

	// Skip by limit
	//skipByLimit := func(refs []*ScanReference, limit int) []*ScanReference {
	//	if len(refs) < limit {
	//		return refs
	//	}
	//
	//	for _, ref := range refs[limit:] {
	//		logboek.Debug.LogF("Reference %s skipped by limit\n", ref.Name().Short())
	//	}
	//
	//	return refs[:limit]
	//}
	//
	//branchesRefs = skipByLimit(branchesRefs, limit)
	//tagsRefs = skipByLimit(tagsRefs, limit)

	// TODO: Set depth

	// TODO: Set reached commits limit
	for _, tagRef := range tagsRefs {
		tagRef.ReachedCommitsLimit = 1
	}

	// Unite tags and branches references
	result := append(branchesRefs, tagsRefs...)

	return result, nil
}

func scanReferencesHistory(r *git.Repository, refs []*ScanReference, expectedCommitHashList []plumbing.Hash) ([]plumbing.Hash, error) {
	var reachedCommitHashList, stopCommitHashListList []plumbing.Hash

	for _, ref := range refs {
		refReachedCommitHashList, refStopCommitHashListList, err := ScanReferenceHistory(r, ref, expectedCommitHashList, stopCommitHashListList)
		if err != nil {
			return nil, err
		}

		stopCommitHashListList = append(stopCommitHashListList, refStopCommitHashListList...)

	outerLoop:
		for _, c1 := range refReachedCommitHashList {
			for _, c2 := range reachedCommitHashList {
				if c1 == c2 {
					continue outerLoop
				}
			}

			reachedCommitHashList = append(reachedCommitHashList, c1)
		}
	}

	return reachedCommitHashList, nil
}

type commitHistoryScanner struct {
	r                      *git.Repository
	expectedCommitHashList []plumbing.Hash
	reachedCommitHashList  []plumbing.Hash
	stopCommitHashListList []plumbing.Hash

	depthLimit          int
	reachedCommitsLimit int
}

func ScanReferenceHistory(r *git.Repository, ref *ScanReference, expectedCommitHashList, stopCommitHashListList []plumbing.Hash) ([]plumbing.Hash, []plumbing.Hash, error) {
	s := &commitHistoryScanner{
		r:                      r,
		expectedCommitHashList: expectedCommitHashList,
		reachedCommitHashList:  []plumbing.Hash{},
		stopCommitHashListList: stopCommitHashListList,

		// TODO: depthLimit
		reachedCommitsLimit: ref.ReachedCommitsLimit,
	}

	if err := s.scanCommitHistory(ref.Commit.Hash); err != nil {
		return nil, nil, err
	}

	if s.reachedCommitsLimit != 0 && len(s.reachedCommitHashList) == s.reachedCommitsLimit {
		return s.reachedCommitHashList, s.stopCommitHashListList, nil
	}

	if len(s.reachedCommitHashList) != 0 {
		s.stopCommitHashListList = append(s.stopCommitHashListList, s.reachedCommitHashList[len(s.reachedCommitHashList)])
	} else {
		s.stopCommitHashListList = append(s.stopCommitHashListList, ref.Commit.Hash)
	}

	return s.reachedCommitHashList, s.stopCommitHashListList, nil
}

func (s *commitHistoryScanner) scanCommitHistory(commitHash plumbing.Hash) error {
	if s.isStopCommitHash(commitHash) {
		return nil
	}

	s.processCommit(commitHash)

	if s.reachedCommitsLimit != 0 && len(s.reachedCommitHashList) == s.reachedCommitsLimit {
		return nil
	}

	if len(s.expectedCommitHashList) == 0 {
		return nil
	}

	co, err := s.r.CommitObject(commitHash)
	if err != nil {
		return err
	}

	for _, p := range co.ParentHashes {
		if err := s.scanCommitHistory(p); err != nil {
			return err
		}
	}

	return nil
}

func (s *commitHistoryScanner) processCommit(commitHash plumbing.Hash) {
	n := 0
	for _, c := range s.expectedCommitHashList {
		if c == commitHash {
			s.reachedCommitHashList = append(s.reachedCommitHashList, c)
		} else {
			s.expectedCommitHashList[n] = c
			n++
		}
	}

	s.expectedCommitHashList = s.expectedCommitHashList[:n]
}

func (s *commitHistoryScanner) isStopCommitHash(commitHash plumbing.Hash) bool {
	for _, c := range s.stopCommitHashListList {
		if commitHash == c {
			return true
		}
	}

	return false
}
