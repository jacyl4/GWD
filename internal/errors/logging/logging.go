package logging

import (
	"context"
	"time"

	apperrors "GWD/internal/errors"
	"GWD/internal/logger"
)

var reservedMetadataKeys = map[string]struct{}{
	"error_code":     {},
	"error_category": {},
	"error_message":  {},
	"operation":      {},
	"module":         {},
	"recoverable":    {},
	"error_time":     {},
	"error":          {},
}

// Error logs msg with structured fields derived from the supplied AppError.
func Error(ctx context.Context, log logger.Logger, msg string, appErr *apperrors.AppError) {
	if log == nil {
		return
	}
	if appErr == nil {
		log.ErrorContext(ctx, msg)
		return
	}

	log.ErrorContext(ctx, msg, Fields(appErr)...)
}

// Fields converts an AppError into a slice of logger.Field for structured logging.
func Fields(appErr *apperrors.AppError) []logger.Field {
	if appErr == nil {
		return nil
	}

	fields := make([]logger.Field, 0, len(appErr.Metadata)+8)

	if appErr.Code != "" {
		fields = append(fields, logger.String("error_code", appErr.Code))
	}
	if appErr.Category != "" {
		fields = append(fields, logger.String("error_category", string(appErr.Category)))
	}
	if appErr.Message != "" {
		fields = append(fields, logger.String("error_message", appErr.Message))
	}
	if appErr.Operation != "" {
		fields = append(fields, logger.String("operation", appErr.Operation))
	}
	if appErr.Module != "" {
		fields = append(fields, logger.String("module", appErr.Module))
	}
	if appErr.Err != nil {
		fields = append(fields, logger.Error(appErr.Err))
	}

	fields = append(fields, logger.String("error_time", appErr.TimestampOrNow().Format(time.RFC3339Nano)))
	fields = append(fields, logger.Any("recoverable", appErr.Recoverable))

	for k, v := range appErr.Metadata {
		if _, reserved := reservedMetadataKeys[k]; reserved {
			continue
		}
		fields = append(fields, logger.Any(k, v))
	}

	return fields
}
