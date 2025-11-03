package deployer

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	binaryDir     = "/usr/local/bin"
	systemdDir    = "/etc/systemd/system"
	binaryMode    = 0o755
	systemdMode   = 0o644
	systemdDropIn = ".d"
)

// deployBinary copies a binary from the repository into the supplied target path and makes it executable.
func deployBinary(repoDir, binaryName, targetPath string) error {
	source := filepath.Join(repoDir, binaryName)
	target := targetPath

	if err := copyFile(source, target); err != nil {
		return errors.Wrapf(err, "copying binary %s to %s", source, target)
	}

	if err := os.Chmod(target, binaryMode); err != nil {
		return errors.Wrapf(err, "setting permissions on %s", target)
	}

	return nil
}

// writeSystemdUnit writes a systemd unit file into the systemd directory.
func writeSystemdUnit(unitName, content string) error {
	path := filepath.Join(systemdDir, unitName)
	if err := writeSystemdFile(path, content); err != nil {
		return errors.Wrapf(err, "writing systemd unit %s", unitName)
	}
	return nil
}

// writeSystemdDropIn writes a systemd drop-in file for a given unit.
func writeSystemdDropIn(unitName, dropInName, content string) error {
	dir := filepath.Join(systemdDir, unitName+systemdDropIn)
	path := filepath.Join(dir, dropInName)

	if err := writeSystemdFile(path, content); err != nil {
		return errors.Wrapf(err, "writing systemd drop-in %s for %s", dropInName, unitName)
	}

	return nil
}

// copyFile copies a file from source to target, creating parent directories as needed.
func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return errors.Wrapf(err, "opening source file %s", source)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return errors.Wrapf(err, "ensuring directory for %s", target)
	}

	out, err := os.Create(target)
	if err != nil {
		return errors.Wrapf(err, "creating target file %s", target)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return errors.Wrapf(err, "copying data from %s to %s", source, target)
	}

	if err := out.Close(); err != nil {
		return errors.Wrapf(err, "closing target file %s", target)
	}

	return nil
}

func writeSystemdFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrapf(err, "creating directory for %s", path)
	}

	if err := os.WriteFile(path, []byte(content), systemdMode); err != nil {
		return errors.Wrapf(err, "writing file %s", path)
	}

	return nil
}
