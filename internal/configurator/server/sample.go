package server

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"

	apperrors "GWD/internal/errors"
)

const (
	sampleZipPath        = "/opt/de_GWD/.repo/sample.zip"
	webRootDir           = "/var/www/html"
	sptFilePath          = "/var/www/html/spt"
	requiredSptSizeBytes = 102400 * 1024 // 100 MB
)

// EnsureSampleInstalled validates and extracts the sample archive and ensures the SPT file size.
func EnsureSampleInstalled() error {
	if err := validateSampleZip(sampleZipPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newConfiguratorError(
				"configurator.EnsureSampleInstalled",
				"sample archive not found",
				err,
				apperrors.Metadata{"path": sampleZipPath},
			)
		}
		_ = os.Remove(sampleZipPath)
		return newConfiguratorError(
			"configurator.EnsureSampleInstalled",
			"sample archive validation failed",
			err,
			apperrors.Metadata{"path": sampleZipPath},
		)
	}

	tempDir, err := os.MkdirTemp("", "sample-extract-*")
	if err != nil {
		return newConfiguratorError(
			"configurator.EnsureSampleInstalled",
			"failed to create temporary directory for sample extraction",
			err,
			nil,
		)
	}
	defer os.RemoveAll(tempDir)

	if err := extractZipArchive(sampleZipPath, tempDir); err != nil {
		return newConfiguratorError(
			"configurator.EnsureSampleInstalled",
			"failed to extract sample archive",
			err,
			apperrors.Metadata{"zip": sampleZipPath, "destination": tempDir},
		)
	}

	sourceRoot := filepath.Join(tempDir, "sample")
	if _, err := os.Stat(sourceRoot); err != nil {
		if os.IsNotExist(err) {
			sourceRoot = tempDir
		} else {
			return newConfiguratorError(
				"configurator.EnsureSampleInstalled",
				"failed to inspect extracted sample directory",
				err,
				nil,
			)
		}
	}

	if err := copyDirectoryContents(sourceRoot, webRootDir); err != nil {
		return newConfiguratorError(
			"configurator.EnsureSampleInstalled",
			"failed to copy sample contents",
			err,
			apperrors.Metadata{"source": sourceRoot, "destination": webRootDir},
		)
	}

	if err := ensureSptFileSize(); err != nil {
		return err
	}

	return nil
}

func validateSampleZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}

	return nil
}

func extractZipArchive(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if err := extractZipEntry(f, dest); err != nil {
			return err
		}
	}

	return nil
}

func extractZipEntry(f *zip.File, dest string) error {
	targetPath := filepath.Join(dest, f.Name)

	// Prevent ZipSlip
	if !filepath.IsAbs(dest) {
		dest, _ = filepath.Abs(dest)
	}
	absoluteTarget, _ := filepath.Abs(targetPath)
	if len(absoluteTarget) < len(dest) || absoluteTarget[:len(dest)] != dest {
		return errors.New("zip entry escapes destination directory")
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, f.Mode())
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, rc); err != nil {
		return err
	}

	return nil
}

func copyDirectoryContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if err := os.RemoveAll(dstPath); err != nil {
			return err
		}

		if entry.IsDir() {
			if err := copyDirectoryContents(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()

	info, err := input.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, input)
	return err
}

func ensureSptFileSize() error {
	info, err := os.Stat(sptFilePath)
	switch {
	case err == nil && info.Size() >= requiredSptSizeBytes:
		return nil
	case err != nil && !os.IsNotExist(err):
		return newConfiguratorError(
			"configurator.ensureSptFileSize",
			"failed to stat spt file",
			err,
			apperrors.Metadata{"path": sptFilePath},
		)
	}

	if err := os.MkdirAll(filepath.Dir(sptFilePath), 0755); err != nil {
		return newConfiguratorError(
			"configurator.ensureSptFileSize",
			"failed to create directory for spt file",
			err,
			apperrors.Metadata{"path": filepath.Dir(sptFilePath)},
		)
	}

	f, err := os.OpenFile(sptFilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return newConfiguratorError(
			"configurator.ensureSptFileSize",
			"failed to open spt file",
			err,
			apperrors.Metadata{"path": sptFilePath},
		)
	}
	defer f.Close()

	if err := f.Truncate(requiredSptSizeBytes); err != nil {
		return newConfiguratorError(
			"configurator.ensureSptFileSize",
			"failed to resize spt file",
			err,
			apperrors.Metadata{"path": sptFilePath, "size_bytes": requiredSptSizeBytes},
		)
	}

	return nil
}
