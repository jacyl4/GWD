package system

import (
	"os/exec"

	"github.com/pkg/errors"
)

// PackagesToRemove are conflicting software packages that need to be uninstalled.
var PackagesToRemove = []string{
	"ipset",
	"haveged",
	"subversion",
	"os-prober",
	"systemd-timesyncd",
}

func (m *DpkgManager) removeConflictingPackages() error {
	installed, err := m.installedPackageSet()
	if err != nil {
		return errors.Wrap(err, "Failed to list installed packages")
	}

	for _, pkg := range PackagesToRemove {
		if _, exists := installed[pkg]; exists {
			if err := m.removePackage(pkg); err != nil {
				return errors.Wrapf(err, "Failed to uninstall package %s", pkg)
			}
		}
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
