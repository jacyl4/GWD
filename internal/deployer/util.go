package deployer

import (
	"io"
	"os"
	"path/filepath"
)

// copyFile copies a file atomically by writing to a temp file first.
func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}

	tmpFile := target + ".tmp"
	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpFile)
		return err
	}

	if err := out.Close(); err != nil {
		os.Remove(tmpFile)
		return err
	}

	if err := os.Rename(tmpFile, target); err != nil {
		os.Remove(tmpFile)
		return err
	}

	return nil
}
