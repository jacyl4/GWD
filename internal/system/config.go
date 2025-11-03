package system

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/pkg/errors"
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
		return nil, errors.Wrap(err, "architecture detection failed")
	}
	cfg.Architecture = arch

	virtType, err := detectVirtualization()
	if err != nil {
		return nil, errors.Wrap(err, "virtualization detection failed")
	}
	cfg.VirtType = virtType

	return cfg, nil
}

// Validate ensures working directories exist and required commands are present.
func (c *Config) Validate() error {
	if err := os.MkdirAll(c.WorkingDir, 0o755); err != nil {
		return errors.Wrapf(err, "failed to create working directory %s", c.WorkingDir)
	}

	if err := os.MkdirAll(c.TmpDir, 0o755); err != nil {
		return errors.Wrapf(err, "failed to create temporary directory %s", c.TmpDir)
	}

	requiredCommands := []string{"apt", "wget", "curl", "systemctl"}
	for _, cmd := range requiredCommands {
		if _, err := exec.LookPath(cmd); err != nil {
			return errors.Errorf("missing required system command: %s", cmd)
		}
	}

	return nil
}

// GetRepoDir returns the local repository directory used by installers.
func (c *Config) GetRepoDir() string {
	return fmt.Sprintf("%s/.repo", c.WorkingDir)
}

// GetLogDir returns the directory where runtime logs are stored.
func (c *Config) GetLogDir() string {
	return fmt.Sprintf("%s/logs", c.WorkingDir)
}

// IsSupportedArchitecture reports whether the detected architecture is supported.
func (c *Config) IsSupportedArchitecture() bool {
	return c.Architecture == "amd64" || c.Architecture == "arm64"
}

// IsContainer reports whether the environment is containerized.
func (c *Config) IsContainer() bool {
	return c.VirtType == "container"
}

// GetTempFilePath returns an absolute path inside the configured temporary directory.
func (c *Config) GetTempFilePath(name string) string {
	return fmt.Sprintf("%s/%s", c.TmpDir, name)
}

// GetTempDirPath returns an absolute directory path inside the configured temp directory.
func (c *Config) GetTempDirPath(name string) string {
	return fmt.Sprintf("%s/%s", c.TmpDir, name)
}

// SystemConfig is kept for backward compatibility with existing call sites.
type SystemConfig = Config

// LoadSystemConfig preserves the previous constructor name.
func LoadSystemConfig() (*SystemConfig, error) {
	return LoadConfig()
}
