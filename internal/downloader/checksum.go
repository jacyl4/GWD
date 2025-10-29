package downloader

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

// Validator validates downloaded files against expected metrics.
type Validator struct {
	logger *logger.ColoredLogger
}

// NewValidator creates a new Validator instance.
func NewValidator(log *logger.ColoredLogger) *Validator {
	return &Validator{
		logger: log,
	}
}

// ValidateFile compares the file hash with the expected hash string.
func (v *Validator) ValidateFile(filePath, expectedHash string) (bool, error) {
	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))

	actualHash, err := v.calculateSHA256(filePath)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to calculate file hash: %s", filePath)
	}

	match := actualHash == expectedHash

	if match {
		v.logger.Debug("File validation successful: %s", filePath)
	} else {
		v.logger.Warn("File validation failed: %s", filePath)
		v.logger.Debug("Expected hash: %s", expectedHash)
		v.logger.Debug("Actual hash: %s", actualHash)
	}

	return match, nil
}

func (v *Validator) calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to open file: %s", filePath)
	}
	defer file.Close()

	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		return "", errors.Wrapf(err, "Failed to read file content: %s", filePath)
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	return hash, nil
}

// ValidateFileSize ensures file meets minimum size expectation.
func (v *Validator) ValidateFileSize(filePath string, minSize int64) (bool, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return false, errors.Wrapf(err, "Failed to get file information: %s", filePath)
	}

	size := stat.Size()
	valid := size >= minSize

	if !valid {
		v.logger.Warn("File size validation failed: %s (size: %d, minimum required: %d)",
			filePath, size, minSize)
	}

	return valid, nil
}

// VerifyDownload validates both size and hash for the downloaded file.
func (v *Validator) VerifyDownload(filePath, expectedHash string, minSize int64) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return errors.Errorf("Downloaded file does not exist: %s", filePath)
	}

	if minSize > 0 {
		valid, err := v.ValidateFileSize(filePath, minSize)
		if err != nil {
			return errors.Wrap(err, "File size validation failed")
		}
		if !valid {
			return errors.Errorf("File size does not meet requirements: %s", filePath)
		}
	}

	valid, err := v.ValidateFile(filePath, expectedHash)
	if err != nil {
		return errors.Wrap(err, "Hash validation failed")
	}
	if !valid {
		return errors.Errorf("File hash validation failed: %s", filePath)
	}

	return nil
}
