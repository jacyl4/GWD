package dpkg

import (
	"runtime"
	"strings"

	apperrors "GWD/internal/errors"
)

// Manager orchestrates package installation and cleanup via dpkg/apt.
type Manager struct {
	exec Executor
}

// NewManager constructs a Manager with the provided executor (defaults to SystemExecutor).
func NewManager(exec Executor) *Manager {
	if exec == nil {
		exec = SystemExecutor{}
	}
	return &Manager{exec: exec}
}

// RequiredPackages defines the list of baseline software packages GWD expects.
var RequiredPackages = []string{
	"sudo",
	"wget",
	"curl",
	"git",
	"locales",
	"psmisc",
	"idn2",
	"dns-root-data",
	"netcat-openbsd",
	"bind9-dnsutils",
	"net-tools",
	"resolvconf",
	"unbound",
	"nftables",
	"ca-certificates",
	"apt-transport-https",
	"gnupg2",
	"unzip",
	"zstd",
	"jq",
	"bc",
	"moreutils",
	"rng-tools-debian",
	"chrony",
	"socat",
	"screen",
	"ethtool",
	"qrencode",
	"libmimalloc3",
}

// PackagesToRemove lists conflicting packages that should be purged.
var PackagesToRemove = []string{
	"haveged",
	"subversion",
	"os-prober",
	"systemd-timesyncd",
}

// ConditionalPackages identifies extra packages that depend on runtime characteristics.
var ConditionalPackages = map[string]bool{
	"irqbalance": runtime.NumCPU() > 1,
}

// InstallDependencies installs required and conditional packages after removing conflicts.
func (m *Manager) InstallDependencies() error {
	if err := m.removeConflictingPackages(); err != nil {
		return dpkgError("dpkg.removeConflicts", "failed to uninstall conflicting packages", err, nil)
	}

	packagesToInstall, err := m.getMissingPackages()
	if err != nil {
		return dpkgError("dpkg.getMissingPackages", "failed to determine missing packages", err, nil)
	}

	if len(packagesToInstall) == 0 {
		return nil
	}

	if err := m.installPackages(packagesToInstall); err != nil {
		return dpkgError("dpkg.installPackages", "failed to install packages", err, apperrors.Metadata{
			"packages": strings.Join(packagesToInstall, ","),
		})
	}

	return nil
}

// UpgradeSystem performs an apt full-upgrade.
func (m *Manager) UpgradeSystem() error {
	steps := [][]string{
		{"apt", "update", "--fix-missing"},
		{"apt", "full-upgrade", "-y"},
	}
	for _, step := range steps {
		if err := m.exec.Run(step[0], step[1:]...); err != nil {
			cmd := strings.Join(append([]string{step[0]}, step[1:]...), " ")
			return dpkgError("dpkg.upgradeSystem", "package upgrade command failed", err, apperrors.Metadata{
				"command": cmd,
			})
		}
	}
	return nil
}

func (m *Manager) getMissingPackages() ([]string, error) {
	installed, err := m.installedPackageSet()
	if err != nil {
		return nil, dpkgError("dpkg.installedPackageSet", "failed to list installed packages", err, nil)
	}

	var missing []string

	for _, pkg := range RequiredPackages {
		if _, exists := installed[pkg]; !exists {
			missing = append(missing, pkg)
		}
	}

	for pkg, shouldInstall := range ConditionalPackages {
		if !shouldInstall {
			continue
		}
		if _, exists := installed[pkg]; !exists {
			missing = append(missing, pkg)
		}
	}

	return missing, nil
}

func (m *Manager) installedPackageSet() (map[string]struct{}, error) {
	output, err := m.exec.Output("dpkg-query", "-W", "-f=${binary:Package}\n")
	if err != nil {
		return nil, dpkgError("dpkg.installedPackageSet", "dpkg-query failed", err, apperrors.Metadata{
			"command": "dpkg-query -W",
		})
	}

	result := make(map[string]struct{})
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		result[pkg] = struct{}{}
		if idx := strings.Index(pkg, ":"); idx > 0 {
			result[pkg[:idx]] = struct{}{}
		}
	}
	return result, nil
}

func (m *Manager) installPackages(packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	if err := m.updatePackageIndex(); err != nil {
		return dpkgError("dpkg.updatePackageIndex", "failed to update package index", err, nil)
	}

	args := append([]string{"install", "-y"}, packages...)
	if err := m.exec.Run("apt", args...); err != nil {
		return dpkgError("dpkg.installPackages", "failed to install packages via apt", err, apperrors.Metadata{
			"packages": strings.Join(packages, ","),
		})
	}
	return nil
}

func (m *Manager) updatePackageIndex() error {
	if err := m.exec.Run("apt", "update"); err != nil {
		return dpkgError("dpkg.updatePackageIndex", "apt update failed", err, nil)
	}
	return nil
}

func (m *Manager) removeConflictingPackages() error {
	installed, err := m.installedPackageSet()
	if err != nil {
		return dpkgError("dpkg.installedPackageSet", "failed to list installed packages", err, nil)
	}

	for _, pkg := range PackagesToRemove {
		if _, exists := installed[pkg]; exists {
			if err := m.removePackage(pkg); err != nil {
				return dpkgError("dpkg.removePackage", "failed to uninstall package", err, apperrors.Metadata{
					"package": pkg,
				})
			}
		}
	}
	return nil
}

func (m *Manager) removePackage(name string) error {
	if err := m.exec.Run("apt", "remove", "--purge", "-y", name); err != nil {
		return dpkgError("dpkg.removePackage", "apt remove failed", err, apperrors.Metadata{
			"package": name,
		})
	}
	return nil
}

func dpkgError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	return apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, message, err).
		WithModule("pkgmgr.dpkg").
		WithOperation(operation).
		WithFields(metadata)
}
