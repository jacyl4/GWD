package server

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	resolvconfConfigDir        = "/etc/resolvconf/resolv.conf.d"
	resolvconfHeadFile         = "/etc/resolvconf/resolv.conf.d/head"
	resolvconfOriginalFile     = "/etc/resolvconf/resolv.conf.d/original"
	resolvconfBaseFile         = "/etc/resolvconf/resolv.conf.d/base"
	resolvconfTailFile         = "/etc/resolvconf/resolv.conf.d/tail"
	resolvconfRunResolvConf    = "/etc/resolvconf/run/resolv.conf"
	runResolvconfResolvConf    = "/run/resolvconf/resolv.conf"
	resolvconfInterfaceDir     = "/run/resolvconf/interface"
	etcResolvConfPath          = "/etc/resolv.conf"
	systemInterfacesConfigFile = "/etc/network/interfaces"

	resolvconfHeadContent = "nameserver 1.1.1.1\nnameserver 8.8.8.8\n"
)

// EnsureResolvconfConfig prepares the system resolvconf configuration to use the local resolver.
func EnsureResolvconfConfig() error {
	if err := removeDirectoryContents(resolvconfConfigDir); err != nil {
		return errors.Wrapf(err, "failed to clean %s", resolvconfConfigDir)
	}

	filesToTouch := []string{
		resolvconfOriginalFile,
		resolvconfBaseFile,
		resolvconfTailFile,
	}

	for _, file := range filesToTouch {
		if err := ensureEmptyFile(file); err != nil {
			return errors.Wrapf(err, "failed to prepare %s", file)
		}
	}

	if err := os.RemoveAll(etcResolvConfPath); err != nil {
		return errors.Wrapf(err, "failed to remove %s", etcResolvConfPath)
	}

	if err := os.RemoveAll(resolvconfInterfaceDir); err != nil {
		return errors.Wrapf(err, "failed to remove %s", resolvconfInterfaceDir)
	}

	if err := os.WriteFile(resolvconfHeadFile, []byte(resolvconfHeadContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s", resolvconfHeadFile)
	}

	if err := ensureResolvConfSymlink(); err != nil {
		return errors.Wrap(err, "failed to configure /etc/resolv.conf symlink")
	}

	if err := removeDNSServerLines(systemInterfacesConfigFile); err != nil {
		return errors.Wrapf(err, "failed to update %s", systemInterfacesConfigFile)
	}

	if err := runResolvconfUpdate(); err != nil {
		return errors.Wrap(err, "failed to apply resolvconf settings")
	}

	return nil
}

func removeDirectoryContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to read directory %s", dir)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to remove %s", path)
		}
	}

	return nil
}

func ensureEmptyFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for %s", path)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to truncate %s", path)
	}

	return file.Close()
}

func ensureResolvConfSymlink() error {
	if exists, err := fileExists(resolvconfRunResolvConf); err != nil {
		return err
	} else if exists {
		return replaceSymlink(resolvconfRunResolvConf, etcResolvConfPath)
	}

	if exists, err := fileExists(runResolvconfResolvConf); err != nil {
		return err
	} else if exists {
		return replaceSymlink(runResolvconfResolvConf, etcResolvConfPath)
	}

	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, errors.Wrapf(err, "failed to stat %s", path)
}

func replaceSymlink(target, link string) error {
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to remove existing %s", link)
	}

	if err := os.Symlink(target, link); err != nil {
		return errors.Wrapf(err, "failed to create symlink %s -> %s", link, target)
	}

	return nil
}

func removeDNSServerLines(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to read %s", path)
	}

	lines := bytes.Split(data, []byte{'\n'})
	hasTrailingNewline := len(data) > 0 && data[len(data)-1] == '\n'
	filtered := lines[:0]
	removed := false

	for i, line := range lines {
		if hasTrailingNewline && i == len(lines)-1 && len(line) == 0 {
			filtered = append(filtered, line)
			continue
		}

		if bytes.Contains(line, []byte("dns-nameservers ")) {
			removed = true
			continue
		}

		filtered = append(filtered, line)
	}

	if !removed {
		return nil
	}

	output := bytes.Join(filtered, []byte{'\n'})
	// Ensure trailing newline is preserved when original file had one
	if hasTrailingNewline && (len(output) == 0 || output[len(output)-1] != '\n') {
		output = append(output, '\n')
	}

	if err := os.WriteFile(path, output, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s", path)
	}

	return nil
}

func runResolvconfUpdate() error {
	cmd := exec.Command("resolvconf", "-u")
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "resolvconf -u failed: %s", string(output))
	}
	return nil
}
