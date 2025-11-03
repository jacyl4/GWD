package deployer

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// Component represents a deployable system component.
type Component interface {
	// Name returns the logical name of the component.
	Name() string
	// Install deploys the component binary and configuration.
	Install() error
	// Validate checks that the component has been deployed correctly.
	Validate() error
}

// TemplateConfig describes a template file and associated data used for rendering.
type TemplateConfig struct {
	Source string
	Data   any
}

// ComponentConfig defines the resources required to deploy a component.
type ComponentConfig struct {
	Name        string
	BinaryName  string
	BinaryPath  string
	ServiceUnit string
	Service     TemplateConfig
	DropIns     map[string]TemplateConfig
	ConfigDirs  []string
	PostInstall func() error
}

// GenericDeployer provides a reusable implementation of the Component interface.
type GenericDeployer struct {
	repoDir string
	config  ComponentConfig
}

// NewGenericDeployer constructs a GenericDeployer with the provided configuration.
func NewGenericDeployer(repoDir string, config ComponentConfig) *GenericDeployer {
	if config.Name == "" {
		config.Name = config.BinaryName
	}

	return &GenericDeployer{
		repoDir: repoDir,
		config:  config,
	}
}

// Name returns the component name.
func (g *GenericDeployer) Name() string {
	return g.config.Name
}

// Install deploys the configured component resources.
func (g *GenericDeployer) Install() error {
	for _, dir := range g.config.ConfigDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errors.Wrapf(err, "creating config directory %s", dir)
		}
	}

	if err := deployBinary(g.repoDir, g.config.BinaryName, g.config.BinaryPath); err != nil {
		return errors.Wrapf(err, "deploying binary %s", g.config.BinaryName)
	}

	serviceContent, err := renderTemplate(g.config.Service.Source, g.config.Service.Data)
	if err != nil {
		return errors.Wrap(err, "rendering systemd service template")
	}

	if err := writeSystemdUnit(g.config.ServiceUnit, serviceContent); err != nil {
		return errors.Wrapf(err, "writing systemd unit %s", g.config.ServiceUnit)
	}

	for dropInName, dropInTemplate := range g.config.DropIns {
		content, err := renderTemplate(dropInTemplate.Source, dropInTemplate.Data)
		if err != nil {
			return errors.Wrapf(err, "rendering drop-in %s", dropInName)
		}

		if err := writeSystemdDropIn(g.config.ServiceUnit, dropInName, content); err != nil {
			return errors.Wrapf(err, "writing drop-in %s for %s", dropInName, g.config.ServiceUnit)
		}
	}

	if g.config.PostInstall != nil {
		if err := g.config.PostInstall(); err != nil {
			return errors.Wrap(err, "executing post-install hook")
		}
	}

	return nil
}

// Validate verifies that the deployed resources exist on disk.
func (g *GenericDeployer) Validate() error {
	if _, err := os.Stat(g.config.BinaryPath); err != nil {
		return errors.Wrapf(err, "binary not found: %s", g.config.BinaryPath)
	}

	unitPath := filepath.Join(systemdDir, g.config.ServiceUnit)
	if _, err := os.Stat(unitPath); err != nil {
		return errors.Wrapf(err, "systemd unit not found: %s", unitPath)
	}

	for _, dir := range g.config.ConfigDirs {
		if _, err := os.Stat(dir); err != nil {
			return errors.Wrapf(err, "config directory not found: %s", dir)
		}
	}

	return nil
}
