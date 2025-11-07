package system

import (
	"os"
	"os/exec"
	"path/filepath"

	apperrors "GWD/internal/errors"
)

// Config captures runtime characteristics and directories used by GWD.
type Config struct {
	Architecture string `json:"architecture"`
	VirtType     string `json:"virt_type"`
	Branch       string `json:"branch"`
	WorkingDir   string `json:"working_dir"`
	TmpDir       string `json:"tmp_dir"`
}

// LoadConfig builds a Config populated with detected system attributes.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Branch:     "main",
		WorkingDir: "/opt/GWD",
		TmpDir:     "/tmp",
	}

	arch, err := detectArchitecture()
	if err != nil {
		return nil, newSystemError("system.LoadConfig.detectArchitecture", "architecture detection failed", err, nil)
	}
	cfg.Architecture = arch

	cfg.VirtType = DetectVirtualization()

	return cfg, nil
}

// Validate ensures working directories exist and required commands are present.
func (c *Config) Validate() error {
	return c.Setup()
}

// Setup ensures required directories are present and expected commands exist.
func (c *Config) Setup() error {
	if err := c.EnsureDirectories(); err != nil {
		return err
	}
	return c.ValidateCommands()
}

// EnsureDirectories creates the working and temporary directories when absent.
func (c *Config) EnsureDirectories() error {
	if err := os.MkdirAll(c.WorkingDir, 0o755); err != nil {
		return newSystemError("system.Config.EnsureDirectories", "failed to create working directory", err, apperrors.Metadata{
			"path": c.WorkingDir,
		})
	}

	if err := os.MkdirAll(c.TmpDir, 0o755); err != nil {
		return newSystemError("system.Config.EnsureDirectories", "failed to create temporary directory", err, apperrors.Metadata{
			"path": c.TmpDir,
		})
	}

	return nil
}

// ValidateCommands verifies required system commands are available on PATH.
func (c *Config) ValidateCommands() error {
	requiredCommands := []string{"apt", "wget", "curl", "systemctl"}
	for _, cmd := range requiredCommands {
		if _, err := exec.LookPath(cmd); err != nil {
			return newSystemError("system.Config.ValidateCommands", "missing required system command", err, apperrors.Metadata{
				"command": cmd,
			})
		}
	}

	return nil
}

// GetRepoDir returns the local repository directory used by installers.
func (c *Config) GetRepoDir() string {
	return filepath.Join(c.WorkingDir, ".repo")
}

// GetLogDir returns the directory where runtime logs are stored.
func (c *Config) GetLogDir() string {
	return filepath.Join(c.WorkingDir, "logs")
}

// IsContainer reports whether the environment is containerized.
func (c *Config) IsContainer() bool {
	return c.VirtType == VirtTypeContainer
}

func newSystemError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, message, err).
		WithModule("system").
		WithOperation(operation).
		WithFields(metadata)
}
