package logger

import (
	"context"
	"time"

	apperrors "GWD/internal/errors"
)

// ErrorWithAppError logs msg with fields derived from the supplied AppError.
func (l *StandardLogger) ErrorWithAppError(ctx context.Context, msg string, appErr *apperrors.AppError) {
	if l == nil {
		return
	}

	if appErr == nil {
		l.ErrorContext(ctx, msg)
		return
	}

	timestamp := appErr.TimestampOrNow().Format(time.RFC3339Nano)

	fields := []Field{}

	if appErr.Code != "" {
		fields = append(fields, String("error_code", appErr.Code))
	}
	if appErr.Category != "" {
		fields = append(fields, String("error_category", string(appErr.Category)))
	}
	if appErr.Message != "" {
		fields = append(fields, String("error_message", appErr.Message))
	}
	if appErr.Operation != "" {
		fields = append(fields, String("operation", appErr.Operation))
	}
	if appErr.Module != "" {
		fields = append(fields, String("module", appErr.Module))
	}
	if appErr.Err != nil {
		fields = append(fields, Error(appErr.Err))
	}
	if !appErr.Timestamp.IsZero() {
		fields = append(fields, String("error_time", timestamp))
	} else {
		fields = append(fields, String("error_time", timestamp))
	}
	fields = append(fields, Any("recoverable", appErr.Recoverable))

	for k, v := range appErr.Metadata {
		// Avoid overwriting reserved keys.
		switch k {
		case "error_code", "error_category", "error_message", "operation", "module", "recoverable", "error_time", "error":
			continue
		}
		fields = append(fields, Any(k, v))
	}

	l.ErrorContext(ctx, msg, fields...)
}
