package logger

import "context"

// Logger describes the core behaviour required by logging implementations.
type Logger interface {
	// Basic logging helpers.
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})

	// Structured logging helpers with contextual fields.
	DebugContext(ctx context.Context, msg string, fields ...Field)
	InfoContext(ctx context.Context, msg string, fields ...Field)
	WarnContext(ctx context.Context, msg string, fields ...Field)
	ErrorContext(ctx context.Context, msg string, fields ...Field)

	// With returns a derived logger enriched with constant fields.
	With(fields ...Field) Logger

	// Level control.
	SetLevel(level Level)
	GetLevel() Level
}

// Level represents the severity of a log entry.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String renders the textual representation of a Level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Field carries additional contextual information for a log entry.
type Field struct {
	Key   string
	Value interface{}
}

// String records a string field.
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Int records an integer field.
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Error records an error field.
func Error(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

// Any records an arbitrary value.
func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}
