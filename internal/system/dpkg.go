package system

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

// DpkgManager wraps Debian package management tasks (install/upgrade/cleanup).
type DpkgManager struct {
	logger *logger.ColoredLogger
}

// NewDpkgManager creates a new package manager instance.
func NewDpkgManager(log *logger.ColoredLogger) *DpkgManager {
	return &DpkgManager{
		logger: log,
	}
}

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
	"dnsutils",
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
	"sqlite3",
	"unbound",
	"libmimalloc2.0",
}

// ConditionalPackages are packages whose installation depends on system conditions.
var ConditionalPackages = map[string]func() bool{
	"irqbalance": func() bool {
		return getNumCPUs() > 1
	},
}

// PackagesToRemove are conflicting software packages that need to be uninstalled.
var PackagesToRemove = []string{
	"ipset",
	"haveged",
	"subversion",
	"os-prober",
	"systemd-timesyncd",
}

// InstallDependencies installs system dependency packages.
func (m *DpkgManager) InstallDependencies() error {
	m.logger.Progress("Checking and installing system dependency packages")

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

	m.logger.ProgressDone("System dependency package check completed")
	return nil
}

func (m *DpkgManager) removeConflictingPackages() error {
	for _, pkg := range PackagesToRemove {
		if installed, err := m.isPackageInstalled(pkg); err != nil {
			return errors.Wrapf(err, "Failed to check installation status of package %s", pkg)
		} else if installed {
			m.logger.Info("Uninstalling conflicting package: %s", pkg)
			if err := m.removePackage(pkg); err != nil {
				return errors.Wrapf(err, "Failed to uninstall package %s", pkg)
			}
		}
	}
	return nil
}

func (m *DpkgManager) getMissingPackages() ([]string, error) {
	var missing []string

	for _, pkg := range RequiredPackages {
		installed, err := m.isPackageInstalled(pkg)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to check installation status of package %s", pkg)
		}
		if !installed {
			missing = append(missing, pkg)
		}
	}

	for pkg, condition := range ConditionalPackages {
		if condition() {
			installed, err := m.isPackageInstalled(pkg)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed to check installation status of package %s", pkg)
			}
			if !installed {
				missing = append(missing, pkg)
			}
		}
	}

	return missing, nil
}

func (m *DpkgManager) isPackageInstalled(packageName string) (bool, error) {
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("dpkg -l | awk '{print$2}' | grep '^%s$'", packageName))

	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (m *DpkgManager) installPackages(packages []string) error {
	if len(packages) == 0 {
		return nil
	}

	m.logger.Info("Installing missing packages: %s", strings.Join(packages, " "))

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

func (m *DpkgManager) removePackage(packageName string) error {
	cmd := exec.Command("apt", "remove", "--purge", "-y", packageName)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "Failed to uninstall package %s", packageName)
	}
	return nil
}

func (m *DpkgManager) updatePackageIndex() error {
	m.logger.Info("Updating package index...")
	cmd := exec.Command("apt", "update")
	return cmd.Run()
}

// UpgradeSystem upgrades the system to the latest state.
func (m *DpkgManager) UpgradeSystem() error {
	m.logger.Progress("Upgrading system packages...")

	upgradeCommands := []struct {
		name string
		cmd  []string
	}{
		{"Update package index", []string{"apt", "update", "--fix-missing"}},
		{"Upgrade installed packages", []string{"apt", "upgrade", "--allow-downgrades", "-y"}},
		{"Full system upgrade", []string{"apt", "full-upgrade", "-y"}},
		{"Clean up unused packages", []string{"apt", "autoremove", "--purge", "-y"}},
		{"Clean package cache", []string{"apt", "clean", "-y"}},
		{"Auto clean", []string{"apt", "autoclean", "-y"}},
	}

	for _, step := range upgradeCommands {
		m.logger.Info("Executing: %s", step.name)
		cmd := exec.Command(step.cmd[0], step.cmd[1:]...)
		if err := cmd.Run(); err != nil {
			return errors.Wrapf(err, "%s failed", step.name)
		}
	}

	m.logger.ProgressDone("System upgrade completed")
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

func (m *DpkgManager) SetupAptConfiguration() error {
	m.logger.Info("Configuring APT package manager...")

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
		if err := writeFile(file, content, 0644); err != nil {
			return errors.Wrapf(err, "Failed to create configuration file: %s", file)
		}
	}

	return nil
}

func writeFile(filename, content string, perm os.FileMode) error {
	return os.WriteFile(filename, []byte(content), perm)
}
