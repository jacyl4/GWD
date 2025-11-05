package server

import (
	apperrors "GWD/internal/errors"
)

func newConfiguratorError(operation, message string, err error, metadata apperrors.Metadata) *apperrors.AppError {
	appErr := apperrors.ConfigError(apperrors.CodeConfigGeneric, message, err).
		WithModule("configurator").
		WithOperation(operation)
	if metadata != nil {
		appErr.WithFields(metadata)
	}
	return appErr
}
