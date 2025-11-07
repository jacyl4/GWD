package system

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	apperrors "GWD/internal/errors"
)

const (
	swapFilePath     = "/swapfile"
	maxSwapSizeBytes = 4 * 1024 * 1024 * 1024 // 4 GiB
	writeBlockSize   = 1024 * 1024            // 1 MiB
	fstabPath        = "/etc/fstab"
	fstabBackupPath  = "/etc/fstab.backup"
)

// EnsureSwapConfigured guarantees the host has an active swap file configured.
// The operation is idempotent, returning immediately when swap is already enabled.
func EnsureSwapConfigured() error {
	if err := checkRequiredCommands(); err != nil {
		return newSystemError("system.EnsureSwapConfigured", "required swap commands missing", err, nil)
	}

	enabled, err := hasSwap()
	if err != nil {
		return newSystemError("system.EnsureSwapConfigured", "failed to inspect current swap state", err, nil)
	}
	if enabled {
		return nil
	}

	size, err := calculateSwapSize()
	if err != nil {
		return newSystemError("system.EnsureSwapConfigured", "failed to determine swap size", err, nil)
	}

	fsType, err := getFilesystemType(swapFilePath)
	if err != nil {
		return newSystemError("system.EnsureSwapConfigured", "failed to detect filesystem type", err, nil)
	}

	if err := createSwapFile(size, fsType); err != nil {
		return newSystemError(
			"system.EnsureSwapConfigured",
			"failed to create swap file",
			err,
			apperrors.Metadata{
				"size":    size,
				"fs_type": fsType,
				"path":    swapFilePath,
			},
		)
	}

	if err := formatSwap(); err != nil {
		_ = os.Remove(swapFilePath)
		return newSystemError("system.EnsureSwapConfigured", "failed to format swap", err, nil)
	}

	if err := enableSwap(); err != nil {
		_ = os.Remove(swapFilePath)
		return newSystemError("system.EnsureSwapConfigured", "failed to enable swap", err, nil)
	}

	if err := addToFstab(); err != nil {
		return newSystemError("system.EnsureSwapConfigured", "failed to persist swap configuration", err, nil)
	}

	return nil
}

// VerifySwap performs sanity checks to confirm swap is configured as expected.
func VerifySwap() error {
	enabled, err := hasSwap()
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("swap is not enabled")
	}

	if _, err := os.Stat(swapFilePath); err != nil {
		return fmt.Errorf("swap file missing: %w", err)
	}

	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", fstabPath, err)
	}
	if !strings.Contains(string(data), swapFilePath) {
		return fmt.Errorf("swap entry not found in %s", fstabPath)
	}

	return nil
}

func hasSwap() (bool, error) {
	data, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return false, err
	}

	lines := bytes.Split(data, []byte{'\n'})
	for i := 1; i < len(lines); i++ {
		if len(strings.TrimSpace(string(lines[i]))) > 0 {
			return true, nil
		}
	}

	return false, nil
}

func calculateSwapSize() (int64, error) {
	memBytes, err := getMemorySize()
	if err != nil {
		return 0, err
	}
	if memBytes <= 0 {
		return 0, errors.New("unexpected memory size detected")
	}
	if memBytes > maxSwapSizeBytes {
		return maxSwapSizeBytes, nil
	}
	return memBytes, nil
}

func getMemorySize() (int64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected MemTotal format: %q", line)
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		return kb * 1024, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return 0, errors.New("MemTotal not found in /proc/meminfo")
}

func getFilesystemType(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absClean := filepath.Clean(absolute)

	mounts, err := readMounts()
	if err != nil {
		return "", err
	}

	bestLen := -1
	var fsType string
	for _, mount := range mounts {
		mp := filepath.Clean(mount.MountPoint)
		if !pathWithinMount(absClean, mp) {
			continue
		}
		if len(mp) > bestLen {
			bestLen = len(mp)
			fsType = mount.FSType
		}
	}

	if fsType == "" {
		return "", fmt.Errorf("filesystem type not found for %s", path)
	}
	return fsType, nil
}

func pathWithinMount(path, mount string) bool {
	if mount == "/" {
		return true
	}
	if !strings.HasPrefix(path, mount) {
		return false
	}
	if len(path) == len(mount) {
		return true
	}
	return strings.HasPrefix(path[len(mount):], "/")
}

type mountEntry struct {
	MountPoint string
	FSType     string
}

func readMounts() ([]mountEntry, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}

	var entries []mountEntry
	replacer := strings.NewReplacer(
		"\\040", " ",
		"\\011", "\t",
		"\\012", "\n",
		"\\134", "\\",
	)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, mountEntry{
			MountPoint: replacer.Replace(fields[1]),
			FSType:     fields[2],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func createSwapFile(size int64, fsType string) error {
	if size <= 0 {
		return errors.New("swap size must be positive")
	}

	if _, err := os.Stat(swapFilePath); err == nil {
		return fmt.Errorf("swap file already exists at %s", swapFilePath)
	}

	switch fsType {
	case "ext4", "ext3", "ext2":
		return createSwapFileExt(size)
	case "xfs":
		return createSwapFileGeneric(size)
	case "btrfs":
		return createSwapFileBtrfs(size)
	default:
		return createSwapFileGeneric(size)
	}
}

func createSwapFileExt(size int64) error {
	if err := runFallocate(size); err == nil {
		return os.Chmod(swapFilePath, 0o600)
	}
	return createSwapFileGeneric(size)
}

func runFallocate(size int64) error {
	if _, err := exec.LookPath("fallocate"); err != nil {
		return err
	}
	return exec.Command("fallocate", "-l", fmt.Sprintf("%d", size), swapFilePath).Run()
}

func createSwapFileBtrfs(size int64) error {
	if _, err := exec.LookPath("chattr"); err != nil {
		return fmt.Errorf("chattr command not available: %w", err)
	}

	file, err := os.OpenFile(swapFilePath, os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	file.Close()

	if err := os.Chmod(swapFilePath, 0o600); err != nil {
		os.Remove(swapFilePath)
		return err
	}

	if err := exec.Command("chattr", "+C", swapFilePath).Run(); err != nil {
		os.Remove(swapFilePath)
		return fmt.Errorf("failed to disable COW for swapfile: %w", err)
	}

	if err := writeZerosToFile(size, false); err != nil {
		os.Remove(swapFilePath)
		return err
	}

	return nil
}

func createSwapFileGeneric(size int64) error {
	if err := writeZerosToFile(size, true); err != nil {
		return err
	}
	return os.Chmod(swapFilePath, 0o600)
}

func writeZerosToFile(size int64, create bool) error {
	if size <= 0 {
		return errors.New("swap size must be positive")
	}

	flags := os.O_WRONLY
	if create {
		flags |= os.O_CREATE | os.O_TRUNC
	} else {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(swapFilePath, flags, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, writeBlockSize)
	var written int64
	for written < size {
		remaining := size - written
		if remaining > int64(len(buf)) {
			remaining = int64(len(buf))
		}
		if _, err := file.Write(buf[:remaining]); err != nil {
			return err
		}
		written += remaining
	}

	return file.Sync()
}

func formatSwap() error {
	output, err := exec.Command("mkswap", swapFilePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkswap failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func enableSwap() error {
	output, err := exec.Command("swapon", swapFilePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("swapon failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func addToFstab() error {
	entry := fmt.Sprintf("%s none swap sw 0 0\n", swapFilePath)

	data, err := os.ReadFile(fstabPath)
	if err != nil {
		return err
	}

	content := string(data)
	if strings.Contains(content, swapFilePath) {
		return nil
	}

	if err := os.WriteFile(fstabBackupPath, data, 0o644); err != nil {
		return err
	}

	var builder strings.Builder
	builder.WriteString(content)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString(entry)

	tmpPath := fstabPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(builder.String()), 0o644); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, fstabPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

func checkRequiredCommands() error {
	required := []string{"mkswap", "swapon"}
	for _, cmd := range required {
		if _, err := exec.LookPath(cmd); err != nil {
			return fmt.Errorf("required command not found: %s", cmd)
		}
	}
	return nil
}
