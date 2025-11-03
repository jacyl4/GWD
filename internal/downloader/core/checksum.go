package core

import (
	"crypto/sha256"
	stdErrors "errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// CalculateSHA256 returns the SHA256 checksum for the provided reader.
func CalculateSHA256(r io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return "", errors.Wrap(err, "failed to read data for checksum")
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// CalculateFileChecksum opens the supplied path via the provided filesystem and returns its SHA256 hash.
func CalculateFileChecksum(fs FileSystem, filePath string) (string, error) {
	file, err := fs.Open(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open file: %s", filePath)
	}
	defer file.Close()

	return CalculateSHA256(file)
}

// ValidateChecksum ensures that the file at filePath matches the expected hash value.
func ValidateChecksum(fs FileSystem, filePath, expectedHash string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedHash))
	if expected == "" {
		return errors.New("expected checksum is empty")
	}

	actual, err := CalculateFileChecksum(fs, filePath)
	if err != nil {
		return err
	}

	if actual != expected {
		return errors.Errorf("checksum mismatch for %s: expected %s, got %s", filePath, expected, actual)
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
			return errors.Errorf("file does not exist: %s", filePath)
		}
		return errors.Wrapf(err, "failed to stat file: %s", filePath)
	}

	size := info.Size()
	if size < minSize {
		return errors.Errorf("file size %d bytes is less than minimum %d bytes (%s)", size, minSize, filePath)
	}

	return nil
}
