package errors

import (
	"errors"
	"fmt"
	"time"
)

// Metadata holds structured error attributes for diagnostics and logging.
type Metadata map[string]interface{}

// Clone returns a shallow copy of the metadata map.
// Reference values stored in the map are not deep-copied.
func (m Metadata) Clone() Metadata {
	if len(m) == 0 {
		return nil
	}
	cloned := make(Metadata, len(m))
	for k, v := range m {
		cloned[k] = v
	}
	return cloned
}

// AppError represents a structured application error with consistent metadata.
type AppError struct {
	Code        string
	Category    ErrorCategory
	Message     string
	Operation   string
	Module      string
	Err         error
	Metadata    Metadata
	Recoverable bool
	Timestamp   time.Time
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Err != nil {
		return fmt.Sprintf("[%s:%s] %s: %v", e.Category, e.Code, e.Message, e.Err)
	}

	return fmt.Sprintf("[%s:%s] %s", e.Category, e.Code, e.Message)
}

// Unwrap exposes the wrapped error to errors.Is/errors.As.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WithOperation annotates the error with the current operation name.
func (e *AppError) WithOperation(operation string) *AppError {
	e.Operation = operation
	return e
}

// WithModule annotates the error with the module name.
func (e *AppError) WithModule(module string) *AppError {
	e.Module = module
	return e
}

// WithRecoverable toggles the recoverable flag.
func (e *AppError) WithRecoverable(recoverable bool) *AppError {
	e.Recoverable = recoverable
	return e
}

// WithField appends a single metadata entry.
func (e *AppError) WithField(key string, value interface{}) *AppError {
	if e.Metadata == nil {
		e.Metadata = make(Metadata)
	}
	e.Metadata[key] = value
	return e
}

// WithFields merges the provided metadata entries.
func (e *AppError) WithFields(metadata Metadata) *AppError {
	if len(metadata) == 0 {
		return e
	}
	if e.Metadata == nil {
		e.Metadata = make(Metadata, len(metadata))
	}
	for k, v := range metadata {
		e.Metadata[k] = v
	}
	return e
}

// As unwraps standard errors to AppError when possible.
func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// Is compares the target error with AppError values.
func Is(err error, target *AppError) bool {
	if err == nil || target == nil {
		return false
	}
	return errors.Is(err, target)
}

// TimestampOrNow returns the timestamp associated with the error or now when unset.
func (e *AppError) TimestampOrNow() time.Time {
	if e == nil {
		return time.Now()
	}
	if e.Timestamp.IsZero() {
		return time.Now()
	}
	return e.Timestamp
}
