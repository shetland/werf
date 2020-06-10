package git_repo

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/werf/logboek"
	"github.com/werf/werf/pkg/path_matcher"
	"github.com/werf/werf/pkg/true_git"
	"github.com/werf/werf/pkg/true_git/ls_tree"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	errNotABranch   = errors.New("cannot get branch name: HEAD refers to a specific revision that is not associated with a branch name")
	errHeadNotFound = errors.New("HEAD not found")
)

type Base struct {
	Name   string
	TmpDir string
}

func (repo *Base) HeadCommit() (string, error) {
	panic("not implemented")
}

func (repo *Base) LatestBranchCommit(branch string) (string, error) {
	panic("not implemented")
}

func (repo *Base) TagCommit(branch string) (string, error) {
	panic("not implemented")
}

func (repo *Base) remoteOriginUrl(repoPath string) (string, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	cfg, err := repository.Config()
	if err != nil {
		return "", fmt.Errorf("cannot access repo config: %s", err)
	}

	if originCfg, hasKey := cfg.Remotes["origin"]; hasKey {
		return originCfg.URLs[0], nil
	}

	return "", nil
}

func (repo *Base) isEmpty(repoPath string) (bool, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return false, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	commitIter, err := repository.CommitObjects()
	if err != nil {
		return false, err
	}

	_, err = commitIter.Next()
	if err == io.EOF {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

func (repo *Base) getHeadCommit(repoPath string) (string, error) {
	if res, err := true_git.ShowRef(repoPath); err != nil {
		return "", fmt.Errorf("show ref for %s failed: %s", repoPath, err)
	} else {
		for _, ref := range res.Refs {
			if ref.IsHEAD {
				return ref.Commit, nil
			}
		}
	}
	return "", errHeadNotFound
}

func (repo *Base) String() string {
	return repo.GetName()
}

func (repo *Base) GetName() string {
	return repo.Name
}

func (repo *Base) createPatch(repoPath, gitDir, workTreeCacheDir string, opts PatchOptions) (Patch, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	fromHash, err := newHash(opts.FromCommit)
	if err != nil {
		return nil, fmt.Errorf("bad `from` commit hash `%s`: %s", opts.FromCommit, err)
	}

	_, err = repository.CommitObject(fromHash)
	if err != nil {
		return nil, fmt.Errorf("bad `from` commit `%s`: %s", opts.FromCommit, err)
	}

	toHash, err := newHash(opts.ToCommit)
	if err != nil {
		return nil, fmt.Errorf("bad `to` commit hash `%s`: %s", opts.ToCommit, err)
	}

	toCommit, err := repository.CommitObject(toHash)
	if err != nil {
		return nil, fmt.Errorf("bad `to` commit `%s`: %s", opts.ToCommit, err)
	}

	hasSubmodules, err := HasSubmodulesInCommit(toCommit)
	if err != nil {
		return nil, err
	}

	patch := NewTmpPatchFile()

	fileHandler, err := os.OpenFile(patch.GetFilePath(), os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, fmt.Errorf("cannot open patch file `%s`: %s", patch.GetFilePath(), err)
	}

	patchOpts := true_git.PatchOptions{
		FromCommit: opts.FromCommit,
		ToCommit:   opts.ToCommit,
		PathMatcher: path_matcher.NewGitMappingPathMatcher(
			opts.BasePath,
			opts.IncludePaths,
			opts.ExcludePaths,
			false,
		),
		WithEntireFileContext: opts.WithEntireFileContext,
		WithBinary:            opts.WithBinary,
	}

	var desc *true_git.PatchDescriptor
	if hasSubmodules {
		desc, err = true_git.PatchWithSubmodules(fileHandler, gitDir, workTreeCacheDir, patchOpts)
	} else {
		desc, err = true_git.Patch(fileHandler, gitDir, patchOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("error creating patch between `%s` and `%s` commits: %s", opts.FromCommit, opts.ToCommit, err)
	}

	patch.Descriptor = desc

	err = fileHandler.Close()
	if err != nil {
		return nil, fmt.Errorf("error creating patch file `%s`: %s", patch.GetFilePath(), err)
	}

	return patch, nil
}

func HasSubmodulesInCommit(commit *object.Commit) (bool, error) {
	_, err := commit.File(".gitmodules")
	if err == object.ErrFileNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (repo *Base) createDetachedMergeCommit(gitDir, path, workTreeCacheDir string, fromCommit, toCommit string) (string, error) {
	repository, err := git.PlainOpen(path)
	if err != nil {
		return "", fmt.Errorf("cannot open repo at %s: %s", path, err)
	}
	commitHash, err := newHash(toCommit)
	if err != nil {
		return "", fmt.Errorf("bad commit hash %s: %s", toCommit, err)
	}
	v1MergeIntoCommitObj, err := repository.CommitObject(commitHash)
	if err != nil {
		return "", fmt.Errorf("bad commit %s: %s", toCommit, err)
	}
	hasSubmodules, err := HasSubmodulesInCommit(v1MergeIntoCommitObj)
	if err != nil {
		return "", err
	}

	return true_git.CreateDetachedMergeCommit(gitDir, workTreeCacheDir, fromCommit, toCommit, true_git.CreateDetachedMergeCommitOptions{HasSubmodules: hasSubmodules})
}

func (repo *Base) getMergeCommitParents(gitDir, commit string) ([]string, error) {
	repository, err := git.PlainOpen(gitDir)
	if err != nil {
		return nil, fmt.Errorf("cannot open repo at %s: %s", gitDir, err)
	}
	commitHash, err := newHash(commit)
	if err != nil {
		return nil, fmt.Errorf("bad commit hash %s: %s", commit, err)
	}
	commitObj, err := repository.CommitObject(commitHash)
	if err != nil {
		return nil, fmt.Errorf("bad commit %s: %s", commit, err)
	}

	var res []string

	for _, parent := range commitObj.ParentHashes {
		res = append(res, parent.String())
	}

	return res, nil
}

func (repo *Base) createArchive(repoPath, gitDir, workTreeCacheDir string, opts ArchiveOptions) (Archive, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	commitHash, err := newHash(opts.Commit)
	if err != nil {
		return nil, fmt.Errorf("bad commit hash `%s`: %s", opts.Commit, err)
	}

	commit, err := repository.CommitObject(commitHash)
	if err != nil {
		return nil, fmt.Errorf("bad commit `%s`: %s", opts.Commit, err)
	}

	hasSubmodules, err := HasSubmodulesInCommit(commit)
	if err != nil {
		return nil, err
	}

	archive := NewTmpArchiveFile()

	fileHandler, err := os.OpenFile(archive.GetFilePath(), os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, fmt.Errorf("cannot open archive file: %s", err)
	}

	archiveOpts := true_git.ArchiveOptions{
		Commit: opts.Commit,
		PathMatcher: path_matcher.NewGitMappingPathMatcher(
			opts.BasePath,
			opts.IncludePaths,
			opts.ExcludePaths,
			true,
		),
	}

	var desc *true_git.ArchiveDescriptor
	if hasSubmodules {
		desc, err = true_git.ArchiveWithSubmodules(fileHandler, gitDir, workTreeCacheDir, archiveOpts)
	} else {
		desc, err = true_git.Archive(fileHandler, gitDir, workTreeCacheDir, archiveOpts)
	}

	if err != nil {
		return nil, fmt.Errorf("error creating archive for commit `%s`: %s", opts.Commit, err)
	}

	archive.Descriptor = desc

	return archive, nil
}

func (repo *Base) isCommitExists(repoPath,
	gitDir string, commit string) (bool, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return false, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	commitHash, err := newHash(commit)
	if err != nil {
		return false, fmt.Errorf("bad commit hash `%s`: %s", commit, err)
	}

	_, err = repository.CommitObject(commitHash)
	if err == plumbing.ErrObjectNotFound {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("bad commit `%s`: %s", commit, err)
	}

	return true, nil
}

func (repo *Base) tagsList(repoPath string) ([]string, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	tags, err := repository.Tags()
	if err != nil {
		return nil, err
	}

	res := make([]string, 0)

	if err := tags.ForEach(func(ref *plumbing.Reference) error {
		obj, err := repository.TagObject(ref.Hash())
		switch err {
		case nil:
			res = append(res, obj.Name)
		case plumbing.ErrObjectNotFound:
			res = append(res, strings.TrimPrefix(ref.Name().String(), "refs/tags/"))
		default:
			// Some other error
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return res, nil
}

func (repo *Base) remoteBranchesList(repoPath string) ([]string, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	branches, err := repository.References()
	if err != nil {
		return nil, err
	}

	remoteBranchPrefix := "refs/remotes/origin/"

	res := make([]string, 0)
	err = branches.ForEach(func(r *plumbing.Reference) error {
		refName := r.Name().String()
		if strings.HasPrefix(refName, remoteBranchPrefix) {
			value := strings.TrimPrefix(refName, remoteBranchPrefix)
			if value != "HEAD" {
				res = append(res, value)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (repo *Base) checksumWithLsTree(repoPath, gitDir, workTreeCacheDir string, opts ChecksumOptions) (Checksum, error) {
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open repo `%s`: %s", repoPath, err)
	}

	commitHash, err := newHash(opts.Commit)
	if err != nil {
		return nil, fmt.Errorf("bad commit hash `%s`: %s", opts.Commit, err)
	}

	commit, err := repository.CommitObject(commitHash)
	if err != nil {
		return nil, fmt.Errorf("bad commit `%s`: %s", opts.Commit, err)
	}

	hasSubmodules, err := HasSubmodulesInCommit(commit)
	if err != nil {
		return nil, err
	}

	checksum := &ChecksumDescriptor{
		NoMatchPaths: make([]string, 0),
		Hash:         sha256.New(),
	}

	err = true_git.WithWorkTree(gitDir, workTreeCacheDir, opts.Commit, true_git.WithWorkTreeOptions{HasSubmodules: hasSubmodules}, func(worktreeDir string) error {
		repositoryWithPreparedWorktree, err := true_git.GitOpenWithCustomWorktreeDir(gitDir, worktreeDir)
		if err != nil {
			return err
		}

		pathMatcher := path_matcher.NewGitMappingPathMatcher(
			opts.BasePath,
			opts.IncludePaths,
			opts.ExcludePaths,
			false,
		)

		var mainLsTreeResult *ls_tree.Result
		processMsg := fmt.Sprintf("ls-tree (%s)", pathMatcher.String())
		if err := logboek.Debug.LogProcess(
			processMsg,
			logboek.LevelLogProcessOptions{},
			func() error {
				mainLsTreeResult, err = ls_tree.LsTree(repositoryWithPreparedWorktree, opts.Commit, pathMatcher, true)
				return err
			},
		); err != nil {
			return err
		}

		for _, path := range opts.Paths {
			var pathLsTreeResult *ls_tree.Result
			pathMatcher := path_matcher.NewSimplePathMatcher(
				opts.BasePath,
				[]string{path},
				false,
			)

			processMsg := fmt.Sprintf("ls-tree (%s)", pathMatcher.String())
			logboek.Debug.LogProcessStart(processMsg, logboek.LevelLogProcessStartOptions{})
			pathLsTreeResult, err = mainLsTreeResult.LsTree(pathMatcher)
			if err != nil {
				logboek.Debug.LogProcessFail(logboek.LevelLogProcessFailOptions{})
				return err
			}
			logboek.Debug.LogProcessEnd(logboek.LevelLogProcessEndOptions{})

			var pathChecksum string
			if !pathLsTreeResult.IsEmpty() {
				blockMsg := fmt.Sprintf("ls-tree result checksum (%s)", pathMatcher.String())
				_ = logboek.Debug.LogBlock(blockMsg, logboek.LevelLogBlockOptions{}, func() error {
					pathChecksum = pathLsTreeResult.Checksum()
					logboek.Debug.LogLn()
					logboek.Debug.LogLn(pathChecksum)

					return nil
				})
			}

			if pathChecksum != "" {
				checksum.Hash.Write([]byte(pathChecksum))
			} else {
				checksum.NoMatchPaths = append(checksum.NoMatchPaths, path)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return checksum, nil
}
