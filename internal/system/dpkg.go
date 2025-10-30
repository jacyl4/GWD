package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// DpkgManager wraps Debian package management tasks (install/upgrade/cleanup).
type DpkgManager struct{}

// NewDpkgManager creates a new package manager instance.
func NewDpkgManager() *DpkgManager { return &DpkgManager{} }

// RequiredPackages defines the list of basic software packages required for GWD to run.
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

// ConditionalPackages are packages whose installation depends on system conditions.
var ConditionalPackages = map[string]func() bool{
	"irqbalance": func() bool {
		return getNumCPUs() > 1
	},
}

// InstallDependencies installs system dependency packages.
func (m *DpkgManager) InstallDependencies() error {
	if err := m.removeConflictingPackages(); err != nil {
		return errors.Wrap(err, "Failed to uninstall conflicting packages")
	}

	packagesToInstall, err := m.getMissingPackages()
	if err != nil {
		return errors.Wrap(err, "Failed to check for missing packages")
	}

	if len(packagesToInstall) > 0 {
		if err := m.installPackages(packagesToInstall); err != nil {
			return errors.Wrap(err, "Failed to install packages")
		}
	}
	return nil
}

func (m *DpkgManager) getMissingPackages() ([]string, error) {
	installed, err := m.installedPackageSet()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list installed packages")
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

func (m *DpkgManager) installedPackageSet() (map[string]struct{}, error) {
	cmd := exec.Command("dpkg-query", "-W", "-f=${binary:Package}\n")
	output, err := cmd.Output()
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

func (m *DpkgManager) installPackages(packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	if err := m.updatePackageIndex(); err != nil {
		return errors.Wrap(err, "Failed to update package index")
	}

	args := append([]string{"install", "-y"}, packages...)
	cmd := exec.Command("apt", args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "Failed to install packages: %v", packages)
	}

	return nil
}

func (m *DpkgManager) updatePackageIndex() error {
	cmd := exec.Command("apt", "update")
	return cmd.Run()
}

// UpgradeSystem upgrades the system to the latest state.
func (m *DpkgManager) UpgradeSystem() error {

	upgradeCommands := []struct {
		name string
		cmd  []string
	}{
		{"Update package index", []string{"apt", "update", "--fix-missing"}},
		{"Full system upgrade", []string{"apt", "full-upgrade", "-y"}},
	}

	for _, step := range upgradeCommands {
		cmd := exec.Command(step.cmd[0], step.cmd[1:]...)
		if err := cmd.Run(); err != nil {
			return errors.Wrapf(err, "%s failed", step.name)
		}
	}
	return nil
}

func getNumCPUs() int {
	cmd := exec.Command("nproc", "--all")
	output, err := cmd.Output()
	if err != nil {
		return 1
	}

	var numCPUs int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &numCPUs)
	if err != nil {
		return 1
	}

	return numCPUs
}
