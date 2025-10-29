package system

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

type SystemConfig struct {
	Architecture string `json:"architecture"`
	VirtType string `json:"virt_type"`
	Branch string `json:"branch"`
	WorkingDir string `json:"working_dir"`
	TmpDir string `json:"tmp_dir"`
}

func LoadSystemConfig() (*SystemConfig, error) {
	cfg := &SystemConfig{
		Branch:     "main",
		WorkingDir: "/opt/GWD",
		TmpDir:     "/tmp",
	}

	arch, err := detectArchitecture()
	if err != nil {
		return nil, errors.Wrap(err, "Architecture detection failed")
	}
	cfg.Architecture = arch

	virtType, err := detectVirtualization()
	if err != nil {
		return nil, errors.Wrap(err, "Virtualization environment detection failed")
	}
	cfg.VirtType = virtType

	return cfg, nil
}

func detectArchitecture() (string, error) {
	cmd := exec.Command("dpkg", "--print-architecture")
	output, err := cmd.Output()
	if err != nil {
		switch runtime.GOARCH {
		case "amd64":
			return "amd64", nil
		case "arm64":
			return "arm64", nil
		default:
			return "", errors.Errorf("Unsupported architecture: %s", runtime.GOARCH)
		}
	}

	arch := strings.TrimSpace(string(output))
	
	switch arch {
	case "amd64", "arm64":
		return arch, nil
	default:
		return "", errors.Errorf("Unsupported architecture: %s", arch)
	}
}

func detectVirtualization() (string, error) {
	cmd := exec.Command("systemd-detect-virt")
	output, err := cmd.Output()
	if err != nil {
		return "physical", nil
	}

	virt := strings.TrimSpace(string(output))
	
	containerTypes := []string{"openvz", "lxc", "lxc-libvirt", "systemd-nspawn", 
		"docker", "podman", "proot", "pouch"}
	
	for _, containerType := range containerTypes {
		if virt == containerType {
			return "container", nil
		}
	}

	return "vm", nil
}


func (c *SystemConfig) Validate() error {
	if err := os.MkdirAll(c.WorkingDir, 0755); err != nil {
		return errors.Wrapf(err, "Failed to create working directory: %s", c.WorkingDir)
	}

	if err := os.MkdirAll(c.TmpDir, 0755); err != nil {
		return errors.Wrapf(err, "Failed to create temporary directory: %s", c.TmpDir)
	}

	requiredCommands := []string{"apt", "wget", "curl", "systemctl"}
	for _, cmd := range requiredCommands {
		if _, err := exec.LookPath(cmd); err != nil {
			return errors.Errorf("Missing required system command: %s", cmd)
		}
	}

	return nil
}

func (c *SystemConfig) GetRepoDir() string {
	return fmt.Sprintf("%s/.repo", c.WorkingDir)
}

func (c *SystemConfig) GetLogDir() string {
	return fmt.Sprintf("%s/logs", c.WorkingDir)
}

func (c *SystemConfig) IsSupportedArchitecture() bool {
	return c.Architecture == "amd64" || c.Architecture == "arm64"
}

func (c *SystemConfig) IsContainer() bool {
	return c.VirtType == "container"
}

func (c *SystemConfig) GetTempFilePath(filename string) string {
	return fmt.Sprintf("%s/%s", c.TmpDir, filename)
}

func (c *SystemConfig) GetTempDirPath(dirname string) string {
	return fmt.Sprintf("%s/%s", c.TmpDir, dirname)
}
