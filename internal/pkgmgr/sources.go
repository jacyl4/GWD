package dpkg

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	apperrors "GWD/internal/errors"
)

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
		return dpkgError("dpkg.rewriteSourcesList.read", "failed to read sources.list entry", err, apperrors.Metadata{
			"path": path,
		})
	}

	updated, changed, err := rewriteSourcesContent(original, current, next)
	if err != nil {
		return dpkgError("dpkg.rewriteSourcesList.transform", "failed to update suites in sources list", err, apperrors.Metadata{
			"path": path,
		})
	}

	if !changed {
		return nil
	}

	if err := createBackup(path, original); err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return dpkgError("dpkg.rewriteSourcesList.stat", "failed to stat sources.list entry", err, apperrors.Metadata{
			"path": path,
		})
	}

	if err := os.WriteFile(path, updated, info.Mode().Perm()); err != nil {
		return dpkgError("dpkg.rewriteSourcesList.write", "failed to write updated sources list", err, apperrors.Metadata{
			"path": path,
		})
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

	if !containsCurrentRelease(line, current) {
		return line, false
	}

	bodyPart, commentPart := splitLineAndComment(line)
	if bodyPart == "" {
		return line, false
	}

	fields := strings.Fields(bodyPart)
	if len(fields) < 3 {
		return line, false
	}

	if fields[0] != "deb" && fields[0] != "deb-src" {
		return line, false
	}

	suiteIdx := suiteIndex(fields)
	if suiteIdx < 0 || suiteIdx >= len(fields) {
		return line, false
	}

	newSuite, suiteChanged := rewriteSuite(fields[suiteIdx], current, next)
	fields[suiteIdx] = newSuite

	compsChanged := false
	if needsFirmware {
		fields, compsChanged = ensureFirmwareComponent(fields, suiteIdx)
	}

	if !suiteChanged && !compsChanged {
		return line, false
	}

	newLine := strings.Join(fields, " ")
	if commentPart != "" {
		newLine += "  " + commentPart
	}

	return newLine, true
}

func containsCurrentRelease(line string, current *DebianRelease) bool {
	if current == nil {
		return false
	}

	if strings.Contains(line, current.Codename) {
		return true
	}
	if current.SecuritySuite != "" && strings.Contains(line, current.SecuritySuite) {
		return true
	}
	if current.UpdatesSuite != "" && strings.Contains(line, current.UpdatesSuite) {
		return true
	}
	return false
}

func splitLineAndComment(line string) (string, string) {
	if idx := strings.Index(line, "#"); idx >= 0 {
		body := strings.TrimSpace(line[:idx])
		comment := strings.TrimSpace(line[idx:])
		return body, comment
	}
	return strings.TrimSpace(line), ""
}

func suiteIndex(fields []string) int {
	if len(fields) < 3 {
		return -1
	}
	if strings.HasPrefix(fields[1], "[") {
		if len(fields) < 4 {
			return -1
		}
		return 3
	}
	return 2
}

func ensureFirmwareComponent(fields []string, suiteIdx int) ([]string, bool) {
	compsStart := suiteIdx + 1
	if compsStart >= len(fields) {
		return fields, false
	}

	components := append([]string(nil), fields[compsStart:]...)
	if stringSliceContains(components, "non-free") && !stringSliceContains(components, "non-free-firmware") {
		components = append(components, "non-free-firmware")
		updated := append([]string(nil), fields[:compsStart]...)
		updated = append(updated, components...)
		return updated, true
	}

	return fields, false
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
		return dpkgError("dpkg.createBackup", "failed to create backup", err, apperrors.Metadata{
			"path":        path,
			"backup_path": backupPath,
		})
	}

	return nil
}
