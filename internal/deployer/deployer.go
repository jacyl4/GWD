package deployer

import (
	"os"
	"path/filepath"

	apperrors "GWD/internal/errors"
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
			return newDeployerError("deployer.GenericDeployer.Install", "failed to create config directory", err, apperrors.Metadata{
				"directory": dir,
			})
		}
	}

	if err := deployBinary(g.repoDir, g.config.BinaryName, g.config.BinaryPath); err != nil {
		return newDeployerError("deployer.GenericDeployer.Install", "failed to deploy binary", err, apperrors.Metadata{
			"binary": g.config.BinaryName,
			"target": g.config.BinaryPath,
		})
	}

	serviceContent, err := renderTemplate(g.config.Service.Source, g.config.Service.Data)
	if err != nil {
		return newDeployerError("deployer.GenericDeployer.Install", "failed to render systemd service template", err, apperrors.Metadata{
			"template": g.config.Service.Source,
		})
	}

	if err := writeSystemdUnit(g.config.ServiceUnit, serviceContent); err != nil {
		return newDeployerError("deployer.GenericDeployer.Install", "failed to write systemd unit", err, apperrors.Metadata{
			"unit": g.config.ServiceUnit,
		})
	}

	for dropInName, dropInTemplate := range g.config.DropIns {
		content, err := renderTemplate(dropInTemplate.Source, dropInTemplate.Data)
		if err != nil {
			return newDeployerError("deployer.GenericDeployer.Install", "failed to render systemd drop-in", err, apperrors.Metadata{
				"drop_in":  dropInName,
				"template": dropInTemplate.Source,
			})
		}

		if err := writeSystemdDropIn(g.config.ServiceUnit, dropInName, content); err != nil {
			return newDeployerError("deployer.GenericDeployer.Install", "failed to write systemd drop-in", err, apperrors.Metadata{
				"drop_in": dropInName,
				"unit":    g.config.ServiceUnit,
			})
		}
	}

	if g.config.PostInstall != nil {
		if err := g.config.PostInstall(); err != nil {
			return newDeployerError("deployer.GenericDeployer.Install", "post-install hook failed", err, nil)
		}
	}

	return nil
}

// Validate verifies that the deployed resources exist on disk.
func (g *GenericDeployer) Validate() error {
	if _, err := os.Stat(g.config.BinaryPath); err != nil {
		return newDeployerError("deployer.GenericDeployer.Validate", "binary not found", err, apperrors.Metadata{
			"path": g.config.BinaryPath,
		})
	}

	unitPath := filepath.Join(systemdDir, g.config.ServiceUnit)
	if _, err := os.Stat(unitPath); err != nil {
		return newDeployerError("deployer.GenericDeployer.Validate", "systemd unit not found", err, apperrors.Metadata{
			"path": unitPath,
		})
	}

	for _, dir := range g.config.ConfigDirs {
		if _, err := os.Stat(dir); err != nil {
			return newDeployerError("deployer.GenericDeployer.Validate", "config directory not found", err, apperrors.Metadata{
				"directory": dir,
			})
		}
	}

	return nil
}

func newDeployerError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	appErr := apperrors.DeploymentError(apperrors.CodeDeploymentGeneric, message, err).
		WithModule("deployer").
		WithOperation(operation)
	if metadata != nil {
		appErr.WithFields(metadata)
	}
	return appErr
}
