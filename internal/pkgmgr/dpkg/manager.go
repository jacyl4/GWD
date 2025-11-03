package dpkg

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
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
var ConditionalPackages = map[string]func() bool{
	"irqbalance": func() bool {
		return getNumCPUs() > 1
	},
}

// InstallDependencies installs required and conditional packages after removing conflicts.
func (m *Manager) InstallDependencies() error {
	if err := m.removeConflictingPackages(); err != nil {
		return errors.Wrap(err, "failed to uninstall conflicting packages")
	}

	packagesToInstall, err := m.getMissingPackages()
	if err != nil {
		return errors.Wrap(err, "failed to determine missing packages")
	}

	if len(packagesToInstall) == 0 {
		return nil
	}

	if err := m.installPackages(packagesToInstall); err != nil {
		return errors.Wrap(err, "failed to install packages")
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
			return errors.Wrapf(err, "command %s %s failed", step[0], strings.Join(step[1:], " "))
		}
	}
	return nil
}

func (m *Manager) getMissingPackages() ([]string, error) {
	installed, err := m.installedPackageSet()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list installed packages")
	}

	var missing []string

	for _, pkg := range RequiredPackages {
		if _, exists := installed[pkg]; !exists {
			missing = append(missing, pkg)
		}
	}

	for pkg, condition := range ConditionalPackages {
		if condition() {
			if _, exists := installed[pkg]; !exists {
				missing = append(missing, pkg)
			}
		}
	}

	return missing, nil
}

func (m *Manager) installedPackageSet() (map[string]struct{}, error) {
	output, err := m.exec.Output("dpkg-query", "-W", "-f=${binary:Package}\n")
	if err != nil {
		return nil, errors.Wrap(err, "dpkg-query failed")
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
		return errors.Wrap(err, "failed to update package index")
	}

	args := append([]string{"install", "-y"}, packages...)
	if err := m.exec.Run("apt", args...); err != nil {
		return errors.Wrapf(err, "failed to install packages: %v", packages)
	}
	return nil
}

func (m *Manager) updatePackageIndex() error {
	return m.exec.Run("apt", "update")
}

func (m *Manager) removeConflictingPackages() error {
	installed, err := m.installedPackageSet()
	if err != nil {
		return errors.Wrap(err, "failed to list installed packages")
	}

	for _, pkg := range PackagesToRemove {
		if _, exists := installed[pkg]; exists {
			if err := m.removePackage(pkg); err != nil {
				return errors.Wrapf(err, "failed to uninstall package %s", pkg)
			}
		}
	}
	return nil
}

func (m *Manager) removePackage(name string) error {
	return m.exec.Run("apt", "remove", "--purge", "-y", name)
}

func getNumCPUs() int {
	output, err := SystemExecutor{}.Output("nproc", "--all")
	if err != nil {
		return 1
	}

	var numCPUs int
	if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &numCPUs); scanErr != nil {
		return 1
	}

	if numCPUs <= 0 {
		return 1
	}
	return numCPUs
}
