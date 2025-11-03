package logger

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockLogger records log entries in memory for assertions in tests.
type MockLogger struct {
	mu      sync.Mutex
	entries []MockEntry
	level   Level
}

// MockEntry stores a single log emission.
type MockEntry struct {
	Level   Level
	Message string
	Fields  []Field
}

// NewMockLogger creates a MockLogger with the lowest log level.
func NewMockLogger() *MockLogger {
	return &MockLogger{
		level: LevelDebug,
	}
}

// Debug satisfies the Logger interface.
func (m *MockLogger) Debug(format string, args ...interface{}) {
	m.log(LevelDebug, format, args...)
}

// Info satisfies the Logger interface.
func (m *MockLogger) Info(format string, args ...interface{}) {
	m.log(LevelInfo, format, args...)
}

// Warn satisfies the Logger interface.
func (m *MockLogger) Warn(format string, args ...interface{}) {
	m.log(LevelWarn, format, args...)
}

// Error satisfies the Logger interface.
func (m *MockLogger) Error(format string, args ...interface{}) {
	m.log(LevelError, format, args...)
}

// DebugContext satisfies the Logger interface.
func (m *MockLogger) DebugContext(ctx context.Context, msg string, fields ...Field) {
	m.logContext(LevelDebug, msg, fields...)
}

// InfoContext satisfies the Logger interface.
func (m *MockLogger) InfoContext(ctx context.Context, msg string, fields ...Field) {
	m.logContext(LevelInfo, msg, fields...)
}

// WarnContext satisfies the Logger interface.
func (m *MockLogger) WarnContext(ctx context.Context, msg string, fields ...Field) {
	m.logContext(LevelWarn, msg, fields...)
}

// ErrorContext satisfies the Logger interface.
func (m *MockLogger) ErrorContext(ctx context.Context, msg string, fields ...Field) {
	m.logContext(LevelError, msg, fields...)
}

// With returns the same mock logger to capture subsequent entries.
func (m *MockLogger) With(fields ...Field) Logger {
	return m
}

// SetLevel adjusts the minimum log level stored.
func (m *MockLogger) SetLevel(level Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.level = level
}

// GetLevel returns the minimum level stored.
func (m *MockLogger) GetLevel() Level {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.level
}

func (m *MockLogger) log(level Level, format string, args ...interface{}) {
	if level < m.GetLevel() {
		return
	}

	message := fmt.Sprintf(format, args...)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = append(m.entries, MockEntry{
		Level:   level,
		Message: message,
	})
}

func (m *MockLogger) logContext(level Level, msg string, fields ...Field) {
	if level < m.GetLevel() {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = append(m.entries, MockEntry{
		Level:   level,
		Message: msg,
		Fields:  append([]Field{}, fields...),
	})
}

// GetEntries returns a copy of all stored entries.
func (m *MockLogger) GetEntries() []MockEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockEntry(nil), m.entries...)
}

// HasEntry reports whether an entry with the provided level contains the substring.
func (m *MockLogger) HasEntry(level Level, substring string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range m.entries {
		if entry.Level == level && strings.Contains(entry.Message, substring) {
			return true
		}
	}
	return false
}

// CountEntries counts entries recorded with the supplied level.
func (m *MockLogger) CountEntries(level Level) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, entry := range m.entries {
		if entry.Level == level {
			count++
		}
	}
	return count
}

// Reset clears all stored entries.
func (m *MockLogger) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = nil
}
