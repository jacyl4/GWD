package dpkg

import (
	"os"
	"os/exec"
	"strings"

	apperrors "GWD/internal/errors"
)

// UpgradeDebianTo13 upgrades Debian 9-12 systems stepwise to Debian 13 (trixie).
func UpgradeDebianTo13() error {
	info, err := detectDebianReleaseInfo()
	if err != nil {
		return dpkgError("dpkg.upgrade.detectRelease", "failed to detect Debian release information", err, nil)
	}

	if info.ID != "debian" {
		return dpkgError("dpkg.upgrade.detectRelease", "unsupported distribution", nil, apperrors.Metadata{
			"distribution": info.ID,
		})
	}

	if err := ensureAptConfiguration(); err != nil {
		return dpkgError("dpkg.upgrade.ensureAptConfiguration", "failed to apply apt configuration", err, nil)
	}

	idx, err := releaseIndex(info)
	if err != nil {
		return err
	}

	if idx == len(debianUpgradePath)-1 {
		return nil
	}

	for ; idx < len(debianUpgradePath)-1; idx++ {
		current := debianUpgradePath[idx]
		next := debianUpgradePath[idx+1]

		if err := ensureFullyUpgraded(current); err != nil {
			return dpkgError("dpkg.upgrade.ensureFullyUpgraded", "failed to prepare release for upgrade", err, apperrors.Metadata{
				"codename": current.Codename,
			})
		}

		if err := updateAptSources(current.Codename, next.Codename); err != nil {
			return dpkgError("dpkg.upgrade.updateAptSources", "failed to update apt sources", err, apperrors.Metadata{
				"from": current.Codename,
				"to":   next.Codename,
			})
		}

		if err := runCommand("apt-get", "update", "--allow-releaseinfo-change"); err != nil {
			return dpkgError("dpkg.upgrade.aptUpdate", "apt-get update failed after switching release", err, apperrors.Metadata{
				"target": next.Codename,
			})
		}

		if err := runCommand("apt-get", "-y", "full-upgrade"); err != nil {
			return dpkgError("dpkg.upgrade.fullUpgrade", "apt-get full-upgrade failed", err, apperrors.Metadata{
				"target": next.Codename,
			})
		}

		if err := runCommand("apt-get", "-y", "autoremove"); err != nil {
			return dpkgError("dpkg.upgrade.autoremove", "apt-get autoremove failed", err, nil)
		}
	}

	return nil
}

func ensureFullyUpgraded(release DebianRelease) error {
	if err := runCommand("apt-get", "update"); err != nil {
		return dpkgError("dpkg.ensureFullyUpgraded", "apt-get update failed", err, apperrors.Metadata{
			"codename": release.Codename,
		})
	}

	if err := runCommand("apt-get", "-y", "full-upgrade"); err != nil {
		return dpkgError("dpkg.ensureFullyUpgraded", "apt-get full-upgrade failed", err, apperrors.Metadata{
			"codename": release.Codename,
		})
	}

	return nil
}

func ensureAptConfiguration() error {
	aptConfigs := map[string]string{
		"/etc/apt/apt.conf.d/01InstallLess": `APT::Get::Assume-Yes "true";
APT::Install-Recommends "false";
APT::Install-Suggests "false";`,
		"/etc/apt/apt.conf.d/71debconf": `Dpkg::Options {
   "--force-confdef";
   "--force-confold";
};`,
	}

	for file, content := range aptConfigs {
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			return dpkgError("dpkg.ensureAptConfiguration", "failed to write apt configuration file", err, apperrors.Metadata{
				"path": file,
			})
		}
	}

	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	if err := cmd.Run(); err != nil {
		return dpkgError("dpkg.runCommand", "command execution failed", err, apperrors.Metadata{
			"command": name,
			"args":    strings.Join(args, " "),
		})
	}

	return nil
}
