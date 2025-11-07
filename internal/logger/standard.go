package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// StandardLogger provides a baseline logger implementation backed by a single writer.
type StandardLogger struct {
	mu           sync.Mutex
	level        Level
	output       io.Writer
	formatter    Formatter
	fields       []Field
	reportCaller bool
}

// NewStandardLogger constructs a StandardLogger instance configured by the provided options.
func NewStandardLogger(options ...Option) *StandardLogger {
	log := &StandardLogger{
		level:     LevelInfo,
		output:    os.Stdout,
		formatter: &TextFormatter{TimestampFormat: time.RFC3339},
	}

	for _, opt := range options {
		if opt != nil {
			opt(log)
		}
	}

	if log.output == nil {
		log.output = os.Stdout
	}
	if log.formatter == nil {
		log.formatter = &TextFormatter{TimestampFormat: time.RFC3339}
	}

	return log
}

// Option configures a StandardLogger during construction.
type Option func(*StandardLogger)

// WithLevel sets the minimum Level that will be emitted by the logger.
func WithLevel(level Level) Option {
	return func(l *StandardLogger) {
		l.level = level
	}
}

// WithOutput redirects log output to the provided writer.
func WithOutput(w io.Writer) Option {
	return func(l *StandardLogger) {
		l.output = w
		if tf, ok := l.formatter.(*TextFormatter); ok {
			tf.Output = w
		}
	}
}

// WithFormatter overrides the formatter used to render log entries.
func WithFormatter(formatter Formatter) Option {
	return func(l *StandardLogger) {
		l.formatter = formatter
	}
}

// WithFields registers default fields for all subsequent log entries.
func WithFields(fields ...Field) Option {
	return func(l *StandardLogger) {
		l.fields = append(l.fields, fields...)
	}
}

// WithCaller enables caller reporting for each log entry.
func WithCaller() Option {
	return func(l *StandardLogger) {
		l.reportCaller = true
	}
}

// Debug emits a debug level log entry.
func (l *StandardLogger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info emits an info level log entry.
func (l *StandardLogger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn emits a warn level log entry.
func (l *StandardLogger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error emits an error level log entry.
func (l *StandardLogger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// DebugContext emits a debug level structured log entry.
func (l *StandardLogger) DebugContext(ctx context.Context, msg string, fields ...Field) {
	l.logContext(ctx, LevelDebug, msg, fields...)
}

// InfoContext emits an info level structured log entry.
func (l *StandardLogger) InfoContext(ctx context.Context, msg string, fields ...Field) {
	l.logContext(ctx, LevelInfo, msg, fields...)
}

// WarnContext emits a warn level structured log entry.
func (l *StandardLogger) WarnContext(ctx context.Context, msg string, fields ...Field) {
	l.logContext(ctx, LevelWarn, msg, fields...)
}

// ErrorContext emits an error level structured log entry.
func (l *StandardLogger) ErrorContext(ctx context.Context, msg string, fields ...Field) {
	l.logContext(ctx, LevelError, msg, fields...)
}

// With derives a new logger enriched with the provided fields.
func (l *StandardLogger) With(fields ...Field) Logger {
	l.mu.Lock()
	baseFields := append([]Field{}, l.fields...)
	level := l.level
	output := l.output
	formatter := l.formatter
	reportCaller := l.reportCaller
	l.mu.Unlock()

	child := &StandardLogger{
		level:        level,
		output:       output,
		formatter:    formatter,
		reportCaller: reportCaller,
		fields:       append(baseFields, fields...),
	}
	return child
}

// SetLevel adjusts the minimum log level emitted.
func (l *StandardLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current minimum log level.
func (l *StandardLogger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

func (l *StandardLogger) log(level Level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	message := fmt.Sprintf(format, args...)

	entry := &Entry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
		Fields:  append([]Field{}, l.fields...),
	}

	if l.reportCaller {
		entry.Caller = l.getCaller()
	}

	l.write(entry)
}

func (l *StandardLogger) logContext(_ context.Context, level Level, msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	allFields := append([]Field{}, l.fields...)
	allFields = append(allFields, fields...)

	entry := &Entry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
		Fields:  allFields,
	}

	if l.reportCaller {
		entry.Caller = l.getCaller()
	}

	l.write(entry)
}

func (l *StandardLogger) write(entry *Entry) {
	if l.formatter == nil {
		fmt.Fprintf(os.Stderr, "logger formatter is not configured\n")
		return
	}

	bytes, err := l.formatter.Format(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to format log entry: %v\n", err)
		return
	}

	if l.output == nil {
		l.output = os.Stdout
	}

	if _, err := l.output.Write(bytes); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log entry: %v\n", err)
	}
}

func (l *StandardLogger) getCaller() *Caller {
	// runtime.Caller skip levels: this function, logContext/log -> caller -> user
	pc, file, line, ok := runtime.Caller(3)
	if !ok {
		return nil
	}

	call := &Caller{
		File: file,
		Line: line,
	}

	if fn := runtime.FuncForPC(pc); fn != nil {
		call.Function = fn.Name()
	}

	return call
}
