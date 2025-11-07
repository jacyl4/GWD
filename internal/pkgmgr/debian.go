package dpkg

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	apperrors "GWD/internal/errors"
)

// DebianRelease captures the metadata required to describe a Debian suite.
type DebianRelease struct {
	Version       string
	Codename      string
	SecuritySuite string
	UpdatesSuite  string
}

// debianUpgradePath defines the sequential upgrade path from Debian 9 to Debian 13.
var debianUpgradePath = []DebianRelease{
	{Version: "9", Codename: "stretch", SecuritySuite: "stretch/updates", UpdatesSuite: "stretch-updates"},
	{Version: "10", Codename: "buster", SecuritySuite: "buster/updates", UpdatesSuite: "buster-updates"},
	{Version: "11", Codename: "bullseye", SecuritySuite: "bullseye-security", UpdatesSuite: "bullseye-updates"},
	{Version: "12", Codename: "bookworm", SecuritySuite: "bookworm-security", UpdatesSuite: "bookworm-updates"},
	{Version: "13", Codename: "trixie", SecuritySuite: "trixie-security", UpdatesSuite: "trixie-updates"},
}

func releaseForCodename(codename string) (*DebianRelease, error) {
	for i := range debianUpgradePath {
		if debianUpgradePath[i].Codename == codename {
			return &debianUpgradePath[i], nil
		}
	}

	return nil, dpkgError("dpkg.releaseForCodename", "unknown Debian codename", nil, apperrors.Metadata{
		"codename": codename,
	})
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
		return nil, dpkgError("dpkg.detectDebianReleaseInfo", "unable to open /etc/os-release", err, apperrors.Metadata{
			"path": "/etc/os-release",
		})
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
		return nil, dpkgError("dpkg.detectDebianReleaseInfo", "failed to parse /etc/os-release", err, apperrors.Metadata{
			"path": "/etc/os-release",
		})
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
		return nil, dpkgError("dpkg.detectDebianReleaseInfo", "could not determine Debian version and codename", nil, apperrors.Metadata{
			"version":  info.Version,
			"codename": info.Codename,
		})
	}

	return info, nil
}

func releaseIndex(info *debianReleaseInfo) (int, error) {
	for idx, rel := range debianUpgradePath {
		if info.Version == rel.Version || info.Codename == rel.Codename {
			return idx, nil
		}
	}

	return -1, dpkgError("dpkg.releaseIndex", "unsupported Debian version", nil, apperrors.Metadata{
		"version":  info.Version,
		"codename": info.Codename,
	})
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
