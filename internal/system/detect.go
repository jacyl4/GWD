package system

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

func detectArchitecture() (string, error) {
	output, err := exec.Command("dpkg", "--print-architecture").Output()
	if err != nil {
		switch runtime.GOARCH {
		case "amd64":
			return "amd64", nil
		case "arm64":
			return "arm64", nil
		default:
			return "", errors.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	}

	arch := strings.TrimSpace(string(output))
	switch arch {
	case "amd64", "arm64":
		return arch, nil
	default:
		return "", errors.Errorf("unsupported architecture: %s", arch)
	}
}

func detectVirtualization() (string, error) {
	output, err := exec.Command("systemd-detect-virt").Output()
	if err != nil {
		return "physical", nil
	}

	virt := strings.TrimSpace(string(output))
	containerTypes := []string{
		"openvz", "lxc", "lxc-libvirt", "systemd-nspawn",
		"docker", "podman", "proot", "pouch",
	}

	for _, containerType := range containerTypes {
		if virt == containerType {
			return "container", nil
		}
	}

	return "vm", nil
}
