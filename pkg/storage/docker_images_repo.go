package storage

import (
	"fmt"

	"github.com/flant/logboek"

	"github.com/werf/werf/pkg/container_runtime"
	"github.com/werf/werf/pkg/docker_registry"
	"github.com/werf/werf/pkg/image"
)

type DockerImagesRepo struct {
	docker_registry.DockerRegistry
	*imagesRepoManager // FIXME rename images repo manager to something
	projectName        string
}

type DockerImagesRepoOptions struct {
	docker_registry.DockerRegistryOptions
	Implementation string
}

func NewDockerImagesRepo(projectName, imagesRepoAddress, imagesRepoMode string, options DockerImagesRepoOptions) (ImagesRepo, error) {
	resolvedImplementation, err := docker_registry.ResolveImplementation(imagesRepoAddress, options.Implementation)
	if err != nil {
		return nil, err
	}
	logboek.Info.LogLn("Using images repo docker registry implementation:", resolvedImplementation)

	dockerRegistry, err := docker_registry.NewDockerRegistry(imagesRepoAddress, resolvedImplementation, options.DockerRegistryOptions)
	if err != nil {
		return nil, err
	}

	resolvedImagesRepoMode, err := dockerRegistry.ResolveRepoMode(imagesRepoAddress, imagesRepoMode)
	if err != nil {
		return nil, err
	}
	logboek.Info.LogLn("Using images repo mode:", resolvedImagesRepoMode)

	imagesRepoManager, err := newImagesRepoManager(imagesRepoAddress, resolvedImagesRepoMode)
	if err != nil {
		return nil, err
	}

	imagesRepo := &DockerImagesRepo{
		projectName:       projectName,
		imagesRepoManager: imagesRepoManager,
		DockerRegistry:    dockerRegistry,
	}

	return imagesRepo, nil
}

func (repo *DockerImagesRepo) CreateImageRepo(imageName string) error {
	return repo.DockerRegistry.CreateRepo(repo.ImageRepositoryName(imageName))
}

func (repo *DockerImagesRepo) DeleteImageRepo(imageName string) error {
	return repo.DockerRegistry.DeleteRepo(repo.ImageRepositoryName(imageName))
}

func (repo *DockerImagesRepo) GetRepoImage(imageName, tag string) (*image.Info, error) {
	return repo.DockerRegistry.GetRepoImage(repo.ImageRepositoryNameWithTag(imageName, tag))
}

func (repo *DockerImagesRepo) GetRepoImages(imageNames []string) (map[string][]*image.Info, error) {
	return repo.SelectRepoImages(imageNames, nil)
}

func (repo *DockerImagesRepo) SelectRepoImages(imageNames []string, f func(string, *image.Info, error) (bool, error)) (map[string][]*image.Info, error) {
	if repo.imagesRepoManager.IsMonorepo() {
		return repo.getRepoImagesFromMonorepo(imageNames, f)
	} else {
		return repo.getRepoImagesFromMultirepo(imageNames, f)
	}
}

func (repo *DockerImagesRepo) DeleteRepoImage(_ DeleteImageOptions, repoImageList ...*image.Info) error {
	return repo.DockerRegistry.DeleteRepoImage(repoImageList...)
}

func (repo *DockerImagesRepo) GetAllImageRepoTags(imageName string) ([]string, error) {
	imageRepoName := repo.imagesRepoManager.ImageRepo(imageName)
	if existingTags, err := repo.DockerRegistry.Tags(imageRepoName); err != nil {
		return nil, fmt.Errorf("unable to get docker tags for image %q: %s", imageRepoName, err)
	} else {
		return existingTags, nil
	}
}

// FIXME: use docker-registry object
func (repo *DockerImagesRepo) PublishImage(publishImage *container_runtime.WerfImage) error {
	return publishImage.Export()
}

func (repo *DockerImagesRepo) ImageRepositoryName(imageName string) string {
	return repo.imagesRepoManager.ImageRepo(imageName)
}

func (repo *DockerImagesRepo) ImageRepositoryNameWithTag(imageName, tag string) string {
	return repo.imagesRepoManager.ImageRepoWithTag(imageName, tag)
}

func (repo *DockerImagesRepo) ImageRepositoryTag(imageName, tag string) string {
	return repo.imagesRepoManager.ImageRepoTag(imageName, tag)
}

func (repo *DockerImagesRepo) String() string {
	return repo.imagesRepoManager.ImagesRepo()
}

func (repo *DockerImagesRepo) getRepoImagesFromMonorepo(imageNames []string, f func(string, *image.Info, error) (bool, error)) (map[string][]*image.Info, error) {
	tags, err := repo.selectImages(repo.imagesRepoManager.imagesRepo, f)
	if err != nil {
		return nil, err
	}

	imageTags := map[string][]*image.Info{}

loop:
	for _, info := range tags {
		for _, imageName := range imageNames {
			metaImageName, ok := info.Labels[image.WerfImageNameLabel]
			if !ok {
				continue
			}

			if metaImageName == imageName {
				imageTags[imageName] = append(imageTags[imageName], info)
				continue loop
			}
		}
	}

	return imageTags, nil
}

func (repo *DockerImagesRepo) getRepoImagesFromMultirepo(imageNames []string, f func(string, *image.Info, error) (bool, error)) (map[string][]*image.Info, error) {
	imageTags := map[string][]*image.Info{}
	for _, imageName := range imageNames {
		tags, err := repo.selectImages(repo.imagesRepoManager.ImageRepo(imageName), f)
		if err != nil {
			return nil, err
		}

		imageTags[imageName] = tags
	}

	return imageTags, nil
}

func (repo *DockerImagesRepo) selectImages(reference string, f func(string, *image.Info, error) (bool, error)) ([]*image.Info, error) {
	return repo.DockerRegistry.SelectRepoImageList(reference, func(ref string, info *image.Info, err error) (bool, error) {
		if err != nil {
			if f != nil {
				return f(ref, info, err)
			}

			return false, err
		}

		werfImageLabel, ok := info.Labels[image.WerfImageLabel]
		if !ok || werfImageLabel != "true" {
			return false, nil
		}

		if f != nil {
			ok, err := f(ref, info, err)
			if err != nil {
				return false, err
			}

			if !ok {
				return false, nil
			}
		}

		return true, nil
	})
}
