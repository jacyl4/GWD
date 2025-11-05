package server

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	sampleZipPath   = "/opt/de_GWD/.repo/sample.zip"
	tmpSampleDir    = "/tmp/sample"
	webRootDir      = "/var/www/html"
	sptFilePath     = "/var/www/html/spt"
	requiredSptSize = 102400 // KB
)

// EnsureSampleInstalled replicates the shell logic:
// 1) validate the zip via `unzip -tq` for "No errors detected in compressed data"
// 2) if valid, extract to /tmp and copy contents to web root, then cleanup
// 3) if invalid, remove the zip and return an error
// 4) ensure /var/www/html/spt is at least 100MB; create with dd if smaller/nonexistent
func EnsureSampleInstalled() error {
	valid, err := verifyZipNoErrors(sampleZipPath)
	if err != nil {
		return err
	}
	if valid {
		// Clean previous temp dir if any
		_ = os.RemoveAll(tmpSampleDir)

		if err := unzipQuiet(sampleZipPath, "/tmp"); err != nil {
			return fmt.Errorf("failed to unzip %s: %w", sampleZipPath, err)
		}

		// Copy extracted sample directory contents into web root
		if err := copyRecursive(filepath.Join(tmpSampleDir, "."), webRootDir); err != nil {
			_ = os.RemoveAll(tmpSampleDir)
			return err
		}

		// Cleanup temp dir
		_ = os.RemoveAll(tmpSampleDir)
	} else {
		_ = os.Remove(sampleZipPath)
		return fmt.Errorf("sample zip download failed")
	}

	// Ensure spt file has size >= 102400 KB (100MB)
	if err := ensureSptSize(); err != nil {
		return err
	}

	return nil
}

func verifyZipNoErrors(zipPath string) (bool, error) {
	cmd := exec.Command("unzip", "-tq", zipPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// unzip -tq returns non-zero on invalid; treat as not valid without wrapping as hard error
		return false, nil
	}
	return bytes.Contains(stdout.Bytes(), []byte("No errors detected in compressed data")), nil
}

func unzipQuiet(zipPath, destDir string) error {
	cmd := exec.Command("unzip", "-q", zipPath, "-d", destDir)
	// Silence output like the original script
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func copyRecursive(srcDir, dstDir string) error {
	// Use system cp -rf to match behavior (preserves modes/links similar to shell script intent)
	cmd := exec.Command("cp", "-rf", srcDir+string(os.PathSeparator), dstDir+string(os.PathSeparator))
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy from %s to %s: %w", srcDir, dstDir, err)
	}
	return nil
}

func ensureSptSize() error {
	// If file doesn't exist or is smaller than required, create with dd like the script
	var sizeKB int64
	if fi, err := os.Stat(sptFilePath); err == nil {
		sizeKB = fi.Size() / 1024
	} else if os.IsNotExist(err) {
		sizeKB = 0
	} else {
		return err
	}

	if sizeKB < requiredSptSize {
		// dd if=/dev/zero of=/var/www/html/spt bs=1k count=100k status=progress
		cmd := exec.Command("dd", "if=/dev/zero", "of="+sptFilePath, "bs=1k", "count=100k", "status=progress")
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create spt file with dd: %w", err)
		}
	}
	return nil
}
