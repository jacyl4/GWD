package system

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// DebianRelease captures the numeric version and codename we need for an upgrade step.
type DebianRelease struct {
	Version  string
	Codename string
}

var debianUpgradePath = []DebianRelease{
	{Version: "9", Codename: "stretch"},
	{Version: "10", Codename: "buster"},
	{Version: "11", Codename: "bullseye"},
	{Version: "12", Codename: "bookworm"},
	{Version: "13", Codename: "trixie"},
}

type debianReleaseInfo struct {
	ID       string
	Version  string
	Codename string
}

// UpgradeDebianTo13 upgrades any supported Debian release (9-12) sequentially to Debian 13 (trixie).
func UpgradeDebianTo13() error {
	info, err := detectDebianReleaseInfo()
	if err != nil {
		return errors.Wrap(err, "failed to detect Debian release information")
	}

	if info.ID != "debian" {
		return errors.Errorf("unsupported distribution: %s", info.ID)
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
			return errors.Wrapf(err, "failed to prepare %s for upgrade", current.Codename)
		}

		if err := updateAptSources(current.Codename, next.Codename); err != nil {
			return errors.Wrapf(err, "failed to update apt sources from %s to %s", current.Codename, next.Codename)
		}

		if err := runCommand("apt-get", "update", "--allow-releaseinfo-change"); err != nil {
			return errors.Wrapf(err, "apt-get update failed after switching to %s", next.Codename)
		}

		if err := runCommand("apt-get", "-y", "full-upgrade"); err != nil {
			return errors.Wrapf(err, "apt-get full-upgrade failed while upgrading to %s", next.Codename)
		}

		if err := runCommand("apt-get", "-y", "autoremove"); err != nil {
			return errors.Wrap(err, "apt-get autoremove failed")
		}
	}

	return nil
}

func ensureFullyUpgraded(release DebianRelease) error {
	if err := runCommand("apt-get", "update"); err != nil {
		return errors.Wrapf(err, "apt-get update failed on %s", release.Codename)
	}

	if err := runCommand("apt-get", "-y", "full-upgrade"); err != nil {
		return errors.Wrapf(err, "apt-get full-upgrade failed on %s", release.Codename)
	}

	return nil
}

func detectDebianReleaseInfo() (*debianReleaseInfo, error) {
	info := &debianReleaseInfo{}

	file, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, errors.Wrap(err, "unable to open /etc/os-release")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := strings.Trim(parts[1], `"`)

		switch key {
		case "ID":
			info.ID = value
		case "VERSION_ID":
			info.Version = normalizeDebianVersion(value)
		case "VERSION_CODENAME":
			info.Codename = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "failed to parse /etc/os-release")
	}

	if info.Version == "" {
		if data, readErr := os.ReadFile("/etc/debian_version"); readErr == nil {
			info.Version = normalizeDebianVersion(strings.TrimSpace(string(data)))
		}
	}

	if info.Codename == "" {
		for _, rel := range debianUpgradePath {
			if info.Version == rel.Version {
				info.Codename = rel.Codename
				break
			}
		}
	}

	if info.Version == "" || info.Codename == "" {
		return nil, errors.Errorf("could not determine Debian version (%s) codename (%s)", info.Version, info.Codename)
	}

	return info, nil
}

func releaseIndex(info *debianReleaseInfo) (int, error) {
	for idx, rel := range debianUpgradePath {
		if info.Version == rel.Version || info.Codename == rel.Codename {
			return idx, nil
		}
	}

	return -1, errors.Errorf("unsupported Debian version: %s (%s)", info.Version, info.Codename)
}

func normalizeDebianVersion(version string) string {
	if version == "" {
		return version
	}

	if strings.Contains(version, ".") {
		return strings.SplitN(version, ".", 2)[0]
	}

	return version
}

func updateAptSources(currentCodename, nextCodename string) error {
	sources := []string{}

	if _, err := os.Stat("/etc/apt/sources.list"); err == nil {
		sources = append(sources, "/etc/apt/sources.list")
	}

	if entries, err := os.ReadDir("/etc/apt/sources.list.d"); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			sources = append(sources, filepath.Join("/etc/apt/sources.list.d", entry.Name()))
		}
	}

	for _, path := range sources {
		if err := replaceCodenameInFile(path, currentCodename, nextCodename); err != nil {
			return err
		}
	}

	return nil
}

func replaceCodenameInFile(path, currentCodename, nextCodename string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", path)
	}

	if !strings.Contains(string(content), currentCodename) {
		return nil
	}

	if err := createBackup(path, content); err != nil {
		return err
	}

	updated := strings.ReplaceAll(string(content), currentCodename, nextCodename)

	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "failed to stat %s", path)
	}

	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return errors.Wrapf(err, "failed to write %s", path)
	}

	return nil
}

func createBackup(path string, data []byte) error {
	timestamp := time.Now().Format("20060102T150405")
	backupPath := fmt.Sprintf("%s.%s.bak", path, timestamp)

	if err := os.WriteFile(backupPath, data, fs.FileMode(0644)); err != nil {
		return errors.Wrapf(err, "failed to create backup for %s", path)
	}

	return nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "command %s %s failed", name, strings.Join(args, " "))
	}

	return nil
}
