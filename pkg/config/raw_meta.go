package config

import (
	"fmt"
	"os"

	"github.com/werf/werf/pkg/slug"
)

type rawMeta struct {
	ConfigVersion   *int               `yaml:"configVersion,omitempty"`
	Project         *string            `yaml:"project,omitempty"`
	DeployTemplates rawDeployTemplates `yaml:"deploy,omitempty"`

	doc *doc `yaml:"-"` // parent

	UnsupportedAttributes map[string]interface{} `yaml:",inline"`
}

func (c *rawMeta) UnmarshalYAML(unmarshal func(interface{}) error) error {
	parentStack.Push(c)
	type plain rawMeta
	err := unmarshal((*plain)(c))
	parentStack.Pop()
	if err != nil {
		return err
	}

	if err := checkOverflow(c.UnsupportedAttributes, nil, c.doc); err != nil {
		return err
	}

	if c.ConfigVersion == nil || *c.ConfigVersion != 1 {
		return newDetailedConfigError("'configVersion: 1' field required!", nil, c.doc)
	}

	if c.Project == nil || *c.Project == "" {
		return newDetailedConfigError("'project' field cannot be empty!", nil, c.doc)
	}

	if err := slug.ValidateProject(*c.Project); err != nil {
		return newDetailedConfigError(fmt.Sprintf("bad project name '%s' specified in config: %s", *c.Project, err), nil, c.doc)
	}

	return nil
}

func (c *rawMeta) toMeta() *Meta {
	meta := &Meta{}

	if c.ConfigVersion != nil {
		meta.ConfigVersion = *c.ConfigVersion
	}

	if c.Project != nil {
		werfProjectName := os.Getenv("WERF_PROJECT_NAME")
		if werfProjectName != "" {
			meta.Project = werfProjectName
		} else {
			meta.Project = *c.Project
		}
	}

	meta.DeployTemplates = c.DeployTemplates.toDeployTemplates()

	return meta
}
