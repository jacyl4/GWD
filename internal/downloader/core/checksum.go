package core

import (
	"crypto/sha256"
	stdErrors "errors"
	"fmt"
	"io"
	"os"
	"strings"

	apperrors "GWD/internal/errors"
)

// CalculateSHA256 returns the SHA256 checksum for the provided reader.
func CalculateSHA256(r io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return "", newChecksumError("checksum.CalculateSHA256", "failed to read data for checksum", err, nil)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// CalculateFileChecksum opens the supplied path via the provided filesystem and returns its SHA256 hash.
func CalculateFileChecksum(fs FileSystem, filePath string) (string, error) {
	file, err := fs.Open(filePath)
	if err != nil {
		return "", newChecksumError("checksum.CalculateFileChecksum", "failed to open file", err, apperrors.Metadata{
			"path": filePath,
		})
	}
	defer file.Close()

	return CalculateSHA256(file)
}

// ValidateChecksum ensures that the file at filePath matches the expected hash value.
func ValidateChecksum(fs FileSystem, filePath, expectedHash string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedHash))
	if expected == "" {
		return newChecksumError("checksum.ValidateChecksum", "expected checksum is empty", nil, apperrors.Metadata{"path": filePath})
	}

	actual, err := CalculateFileChecksum(fs, filePath)
	if err != nil {
		return err
	}

	if actual != expected {
		return newChecksumError("checksum.ValidateChecksum", "checksum mismatch", nil, apperrors.Metadata{
			"path":          filePath,
			"expected_hash": expected,
			"actual_hash":   actual,
		})
	}

	return nil
}

// ValidateFileSize ensures that the file meets the specified minimum size.
func ValidateFileSize(fs FileSystem, filePath string, minSize int64) error {
	if minSize <= 0 {
		return nil
	}

	info, err := fs.Stat(filePath)
	if err != nil {
		if stdErrors.Is(err, os.ErrNotExist) {
			return newChecksumError("checksum.ValidateFileSize", "file does not exist", nil, apperrors.Metadata{"path": filePath})
		}
		return newChecksumError("checksum.ValidateFileSize", "failed to stat file", err, apperrors.Metadata{"path": filePath})
	}

	size := info.Size()
	if size < minSize {
		return newChecksumError("checksum.ValidateFileSize", "file size below required minimum", nil, apperrors.Metadata{
			"path":    filePath,
			"actual":  size,
			"minimum": minSize,
		})
	}

	return nil
}

func newChecksumError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	appErr := apperrors.DependencyError(apperrors.CodeDependencyGeneric, message, err).
		WithModule("downloader.checksum").
		WithOperation(operation)
	if metadata != nil {
		appErr.WithFields(metadata)
	}
	return appErr
}
