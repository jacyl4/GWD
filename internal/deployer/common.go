package deployer

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"

	apperrors "GWD/internal/errors"
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
		return newDeployerError("deployer.deployBinary", "failed to copy binary", err, apperrors.Metadata{
			"source": source,
			"target": target,
		})
	}

	if err := os.Chmod(target, binaryMode); err != nil {
		return newDeployerError("deployer.deployBinary", "failed to set permissions", err, apperrors.Metadata{
			"path": target,
		})
	}

	return nil
}

// writeSystemdUnit writes a systemd unit file into the systemd directory.
func writeSystemdUnit(unitName, content string) error {
	path := filepath.Join(systemdDir, unitName)
	if err := writeSystemdFile(path, content); err != nil {
		return newDeployerError("deployer.writeSystemdUnit", "failed to write systemd unit", err, apperrors.Metadata{
			"unit": unitName,
		})
	}
	return nil
}

// writeSystemdDropIn writes a systemd drop-in file for a given unit.
func writeSystemdDropIn(unitName, dropInName, content string) error {
	dir := filepath.Join(systemdDir, unitName+systemdDropIn)
	path := filepath.Join(dir, dropInName)

	if err := writeSystemdFile(path, content); err != nil {
		return newDeployerError("deployer.writeSystemdDropIn", "failed to write systemd drop-in", err, apperrors.Metadata{
			"drop_in": dropInName,
			"unit":    unitName,
		})
	}

	return nil
}

// copyFile copies a file from source to target, creating parent directories as needed.
func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return newDeployerError("deployer.copyFile", "failed to open source file", err, apperrors.Metadata{
			"source": source,
		})
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return newDeployerError("deployer.copyFile", "failed to ensure directory", err, apperrors.Metadata{
			"target": target,
		})
	}

	out, err := os.Create(target)
	if err != nil {
		return newDeployerError("deployer.copyFile", "failed to create target file", err, apperrors.Metadata{
			"target": target,
		})
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return newDeployerError("deployer.copyFile", "failed to copy file data", err, apperrors.Metadata{
			"source": source,
			"target": target,
		})
	}

	if err := out.Close(); err != nil {
		return newDeployerError("deployer.copyFile", "failed to close target file", err, apperrors.Metadata{
			"target": target,
		})
	}

	return nil
}

func writeSystemdFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return newDeployerError("deployer.writeSystemdFile", "failed to create directory", err, apperrors.Metadata{
			"path": path,
		})
	}

	if err := os.WriteFile(path, []byte(content), systemdMode); err != nil {
		return newDeployerError("deployer.writeSystemdFile", "failed to write file", err, apperrors.Metadata{
			"path": path,
		})
	}

	return nil
}

// systemctlRestart restarts a systemd service.
func systemctlRestart(serviceName string) error {
	cmd := exec.Command("systemctl", "restart", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return newDeployerError("deployer.systemctlRestart", "failed to restart service", err, apperrors.Metadata{
			"service": serviceName,
			"output":  string(output),
		})
	}
	return nil
}

// systemctlEnable enables a systemd service.
func systemctlEnable(serviceName string) error {
	cmd := exec.Command("systemctl", "enable", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return newDeployerError("deployer.systemctlEnable", "failed to enable service", err, apperrors.Metadata{
			"service": serviceName,
			"output":  string(output),
		})
	}
	return nil
}

// copyDirectory recursively copies all files from source directory to target directory.
func copyDirectory(source, target string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return newDeployerError("deployer.copyDirectory", "failed to read source directory", err, apperrors.Metadata{
			"source": source,
		})
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return newDeployerError("deployer.copyDirectory", "failed to create target directory", err, apperrors.Metadata{
			"target": target,
		})
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		targetPath := filepath.Join(target, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(sourcePath, targetPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(sourcePath, targetPath); err != nil {
				return err
			}
		}
	}

	return nil
}
