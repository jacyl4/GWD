package errors

import "time"

// Option customises AppError instances created by constructors.
type Option func(*AppError)

// WithRecoverable toggles the Recoverable flag during construction.
func WithRecoverable(recoverable bool) Option {
	return func(err *AppError) {
		err.Recoverable = recoverable
	}
}

// WithMetadata merges metadata during construction.
func WithMetadata(metadata Metadata) Option {
	return func(err *AppError) {
		if len(metadata) == 0 {
			return
		}
		if err.Metadata == nil {
			err.Metadata = make(Metadata, len(metadata))
		}
		for k, v := range metadata {
			err.Metadata[k] = v
		}
	}
}

// New creates a generic AppError with the supplied metadata.
func New(category ErrorCategory, code, message string, err error, opts ...Option) *AppError {
	appErr := &AppError{
		Code:      code,
		Category:  category,
		Message:   message,
		Err:       err,
		Timestamp: time.Now(),
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(appErr)
	}

	return appErr
}

// NewRecoverable creates an AppError marked as recoverable.
func NewRecoverable(category ErrorCategory, code, message string, err error, opts ...Option) *AppError {
	return New(category, code, message, err, append([]Option{WithRecoverable(true)}, opts...)...)
}
