package system

import (
	"os/exec"
	"runtime"
	"strings"

	apperrors "GWD/internal/errors"
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
			return "", newSystemError("system.detectArchitecture", "unsupported architecture", nil, apperrors.Metadata{
				"goarch": runtime.GOARCH,
			})
		}
	}

	arch := strings.TrimSpace(string(output))
	switch arch {
	case "amd64", "arm64":
		return arch, nil
	default:
		return "", newSystemError("system.detectArchitecture", "unsupported architecture", nil, apperrors.Metadata{
			"arch": arch,
		})
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
