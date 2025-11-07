package system

import (
	"os/exec"
	"runtime"
	"strings"

	apperrors "GWD/internal/errors"
)

const (
	VirtTypePhysical  = "physical"
	VirtTypeContainer = "container"
	VirtTypeVM        = "vm"
	VirtTypeUnknown   = "unknown"
)

var containerTypes = []string{
	"docker", "podman",
	"lxc", "systemd-nspawn",
	"lxc-libvirt", "openvz", "proot", "pouch",
}

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

func DetectVirtualization() string {
	output, err := exec.Command("systemd-detect-virt").Output()
	if err != nil {
		return VirtTypeUnknown
	}

	virt := strings.TrimSpace(string(output))
	if virt == "none" {
		return VirtTypePhysical
	}

	for _, containerType := range containerTypes {
		if virt == containerType {
			return VirtTypeContainer
		}
	}

	return VirtTypeVM
}
