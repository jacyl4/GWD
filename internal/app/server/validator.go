package server

import (
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	apperrors "GWD/internal/errors"
	"GWD/internal/logger"
	"GWD/internal/system"
)

type validation struct {
	name      string
	operation string
	category  apperrors.ErrorCategory
	fn        func() error
}

// EnvironmentValidator encapsulates host validation logic.
type EnvironmentValidator struct {
	config *system.Config
	logger logger.Logger
}

// NewEnvironmentValidator constructs a validator instance.
func NewEnvironmentValidator(cfg *system.Config, log logger.Logger) *EnvironmentValidator {
	return &EnvironmentValidator{
		config: cfg,
		logger: log,
	}
}

// Validate executes core environment checks.
func (v *EnvironmentValidator) Validate() error {
	return v.runValidations([]validation{
		{"Operating System", "validator.validateOperatingSystem", apperrors.ErrCategoryValidation, v.validateOperatingSystem},
		{"Architecture", "validator.validateArchitecture", apperrors.ErrCategoryValidation, v.validateArchitecture},
		{"Network", "validator.validateNetwork", apperrors.ErrCategoryNetwork, v.validateNetworkConnectivity},
		{"Disk Space", "validator.validateDiskSpace", apperrors.ErrCategorySystem, v.validateDiskSpace},
	})
}

func (v *EnvironmentValidator) runValidations(checks []validation) error {
	for _, check := range checks {
		if err := check.fn(); err != nil {
			if appErr, ok := apperrors.As(err); ok {
				return appErr
			}
			return v.wrapError(check.category, check.operation, check.name+" validation failed", err, nil)
		}
	}
	return nil
}

func (v *EnvironmentValidator) validateOperatingSystem() error {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return v.wrapError(
			apperrors.ErrCategorySystem,
			"validator.validateOperatingSystem",
			"failed to read system information",
			err,
			apperrors.Metadata{"path": "/etc/os-release"},
		)
	}

	osInfo := string(content)
	if !strings.Contains(osInfo, "ID=debian") && !strings.Contains(osInfo, "ID_LIKE=debian") {
		return v.wrapError(
			apperrors.ErrCategoryValidation,
			"validator.validateOperatingSystem",
			"unsupported operating system",
			nil,
			apperrors.Metadata{"os_release": osInfo},
		)
	}

	for _, line := range strings.Split(osInfo, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			systemName := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			v.logger.Debug("Detected system: %s", systemName)
			break
		}
	}

	return nil
}

func (v *EnvironmentValidator) validateArchitecture() error {
	switch v.config.Architecture {
	case "amd64", "arm64":
		return nil
	default:
		return v.wrapError(
			apperrors.ErrCategoryValidation,
			"validator.validateArchitecture",
			"unsupported system architecture",
			nil,
			apperrors.Metadata{"architecture": v.config.Architecture},
		)
	}
}

func (v *EnvironmentValidator) validateNetworkConnectivity() error {
	testURLs := []string{
		"https://cloudflare.com",
		"https://google.com",
	}

	for _, url := range testURLs {
		if err := v.testHTTPConnection(url); err != nil {
			return v.wrapError(
				apperrors.ErrCategoryNetwork,
				"validator.validateNetwork",
				"network connectivity test failed",
				err,
				apperrors.Metadata{"url": url},
			)
		}
	}

	return nil
}

func (v *EnvironmentValidator) testHTTPConnection(url string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return v.wrapError(
			apperrors.ErrCategoryNetwork,
			"validator.testHTTPConnection",
			"failed to establish HTTP connection",
			err,
			apperrors.Metadata{"url": url},
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return v.wrapError(
			apperrors.ErrCategoryNetwork,
			"validator.testHTTPConnection",
			"received unsuccessful HTTP status",
			nil,
			apperrors.Metadata{
				"url":         url,
				"status_code": resp.StatusCode,
			},
		)
	}

	return nil
}

func (v *EnvironmentValidator) validateDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return v.wrapError(
			apperrors.ErrCategorySystem,
			"validator.validateDiskSpace",
			"failed to get disk space information",
			err,
			apperrors.Metadata{"path": "/"},
		)
	}

	available := stat.Bavail * uint64(stat.Bsize)
	availableMB := available / (1024 * 1024)

	const minSpaceMB = 1024
	if availableMB < minSpaceMB {
		return v.wrapError(
			apperrors.ErrCategorySystem,
			"validator.validateDiskSpace",
			"insufficient disk space",
			nil,
			apperrors.Metadata{
				"required_mb":  minSpaceMB,
				"available_mb": availableMB,
			},
		)
	}

	v.logger.Debug("Disk available space: %d MB", availableMB)
	return nil
}

func (v *EnvironmentValidator) wrapError(category apperrors.ErrorCategory, operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	if err == nil {
		return apperrors.New(category, errorCodeForCategory(category), message, nil).
			WithModule("environment-validator").
			WithOperation(operation).
			WithFields(metadata)
	}

	if appErr, ok := apperrors.As(err); ok {
		if appErr.Module == "" {
			appErr.WithModule("environment-validator")
		}
		if operation != "" && appErr.Operation == "" {
			appErr.WithOperation(operation)
		}
		if metadata != nil {
			appErr.WithFields(metadata)
		}
		if appErr.Message == "" {
			appErr.Message = message
		}
		return appErr
	}

	return apperrors.New(category, errorCodeForCategory(category), message, err).
		WithModule("environment-validator").
		WithOperation(operation).
		WithFields(metadata)
}
