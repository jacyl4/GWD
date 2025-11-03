package dpkg

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// DebianRelease captures the metadata required to describe a Debian suite.
type DebianRelease struct {
	Version       string
	Codename      string
	SecuritySuite string
	UpdatesSuite  string
}

var debianUpgradePath = []DebianRelease{
	{Version: "9", Codename: "stretch", SecuritySuite: "stretch/updates", UpdatesSuite: "stretch-updates"},
	{Version: "10", Codename: "buster", SecuritySuite: "buster/updates", UpdatesSuite: "buster-updates"},
	{Version: "11", Codename: "bullseye", SecuritySuite: "bullseye-security", UpdatesSuite: "bullseye-updates"},
	{Version: "12", Codename: "bookworm", SecuritySuite: "bookworm-security", UpdatesSuite: "bookworm-updates"},
	{Version: "13", Codename: "trixie", SecuritySuite: "trixie-security", UpdatesSuite: "trixie-updates"},
}

// UpgradeDebianTo13 upgrades Debian 9-12 systems stepwise to Debian 13 (trixie).
func UpgradeDebianTo13() error {
	info, err := detectDebianReleaseInfo()
	if err != nil {
		return errors.Wrap(err, "failed to detect Debian release information")
	}

	if info.ID != "debian" {
		return errors.Errorf("unsupported distribution: %s", info.ID)
	}

	if err := ensureAptConfiguration(); err != nil {
		return errors.Wrap(err, "failed to apply apt configuration")
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
			return errors.Wrapf(err, "failed to write apt configuration file: %s", file)
		}
	}

	return nil
}

func releaseForCodename(codename string) (*DebianRelease, error) {
	for i := range debianUpgradePath {
		if debianUpgradePath[i].Codename == codename {
			return &debianUpgradePath[i], nil
		}
	}

	return nil, errors.Errorf("unknown Debian codename: %s", codename)
}

func releaseRequiresNonFreeFirmware(release *DebianRelease) bool {
	if release == nil {
		return false
	}

	version, err := strconv.Atoi(release.Version)
	if err != nil {
		return false
	}

	return version >= 12
}

type debianReleaseInfo struct {
	ID       string
	Version  string
	Codename string
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
	currentRelease, err := releaseForCodename(currentCodename)
	if err != nil {
		return err
	}

	nextRelease, err := releaseForCodename(nextCodename)
	if err != nil {
		return err
	}

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
		if err := rewriteSourcesList(path, currentRelease, nextRelease); err != nil {
			return err
		}
	}

	return nil
}

func rewriteSourcesList(path string, current, next *DebianRelease) error {
	original, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", path)
	}

	updated, changed, err := rewriteSourcesContent(original, current, next)
	if err != nil {
		return errors.Wrapf(err, "failed to update suites in %s", path)
	}

	if !changed {
		return nil
	}

	if err := createBackup(path, original); err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrapf(err, "failed to stat %s", path)
	}

	if err := os.WriteFile(path, updated, info.Mode().Perm()); err != nil {
		return errors.Wrapf(err, "failed to write %s", path)
	}

	return nil
}

func rewriteSourcesContent(content []byte, current, next *DebianRelease) ([]byte, bool, error) {
	needsFirmware := releaseRequiresNonFreeFirmware(next)
	lines := strings.Split(string(content), "\n")
	changed := false

	for idx, line := range lines {
		updatedLine, lineChanged := transformAptSourceLine(line, current, next, needsFirmware)
		if lineChanged {
			lines[idx] = updatedLine
			changed = true
		}
	}

	if !changed {
		return content, false, nil
	}

	joined := strings.Join(lines, "\n")
	if bytes.HasSuffix(content, []byte("\n")) && !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}

	return []byte(joined), true, nil
}

func transformAptSourceLine(line string, current, next *DebianRelease, needsFirmware bool) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line, false
	}

	if !strings.Contains(line, current.Codename) &&
		(current.SecuritySuite == "" || !strings.Contains(line, current.SecuritySuite)) &&
		(current.UpdatesSuite == "" || !strings.Contains(line, current.UpdatesSuite)) {
		return line, false
	}

	commentIdx := strings.Index(line, "#")
	var body, comment string
	if commentIdx >= 0 {
		body = line[:commentIdx]
		comment = line[commentIdx:]
	} else {
		body = line
	}

	leadingLen := len(body) - len(strings.TrimLeft(body, " \t"))
	leading := body[:leadingLen]
	bodyWithoutLeading := body[leadingLen:]
	bodyTrimmedRight := strings.TrimRight(bodyWithoutLeading, " \t")
	trailing := bodyWithoutLeading[len(bodyTrimmedRight):]
	core := strings.TrimSpace(bodyWithoutLeading)
	if core == "" {
		return line, false
	}

	fields := strings.Fields(core)
	if len(fields) < 3 {
		return line, false
	}

	if fields[0] != "deb" && fields[0] != "deb-src" {
		return line, false
	}

	suiteIdx := 2
	if strings.HasPrefix(fields[1], "[") {
		if len(fields) < 4 {
			return line, false
		}
		suiteIdx = 3
	}

	suite := fields[suiteIdx]
	newSuite, suiteChanged := rewriteSuite(suite, current, next)
	fields[suiteIdx] = newSuite

	compsChanged := false
	if needsFirmware {
		compsStart := suiteIdx + 1
		if compsStart < len(fields) {
			components := append([]string(nil), fields[compsStart:]...)
			if stringSliceContains(components, "non-free") && !stringSliceContains(components, "non-free-firmware") {
				components = append(components, "non-free-firmware")
				fields = append(append([]string(nil), fields[:compsStart]...), components...)
				compsChanged = true
			}
		}
	}

	if !suiteChanged && !compsChanged {
		return line, false
	}

	newBody := leading + strings.Join(fields, " ")
	if trailing != "" {
		newBody += trailing
	}

	if comment != "" {
		newBody += comment
	}

	return newBody, true
}

func rewriteSuite(suite string, current, next *DebianRelease) (string, bool) {
	if suite == current.Codename {
		if suite == next.Codename {
			return suite, false
		}
		return next.Codename, true
	}

	if current.UpdatesSuite != "" && suite == current.UpdatesSuite {
		if suite == next.UpdatesSuite {
			return suite, false
		}
		return next.UpdatesSuite, true
	}

	if current.SecuritySuite != "" && suite == current.SecuritySuite {
		if suite == next.SecuritySuite {
			return suite, false
		}
		return next.SecuritySuite, true
	}

	if !strings.Contains(suite, current.Codename) {
		return suite, false
	}

	replaced := strings.ReplaceAll(suite, current.Codename, next.Codename)
	if replaced == suite {
		return suite, false
	}

	return replaced, true
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func createBackup(path string, data []byte) error {
	timestamp := time.Now().Format("20060102T150405")
	backupPath := fmt.Sprintf("%s.%s.bak", path, timestamp)

	if err := os.WriteFile(backupPath, data, fs.FileMode(0o644)); err != nil {
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
