package ui

import (
	"fmt"
	"io"
	"os"

	"GWD/internal/logger"
)

// Console coordinates logger output, progress indicators, and plain text UI writes.
type Console struct {
	logger   logger.Logger
	progress logger.Progress
	output   io.Writer
}

// ConsoleOption configures Console construction.
type ConsoleOption func(*Console)

// WithProgress supplies a custom progress implementation.
func WithProgress(p logger.Progress) ConsoleOption {
	return func(c *Console) {
		c.progress = p
	}
}

// WithOutput overrides the writer used for non-log text output.
func WithOutput(w io.Writer) ConsoleOption {
	return func(c *Console) {
		if w != nil {
			c.output = w
		}
	}
}

// NewConsole builds a Console bound to the provided logger.
func NewConsole(log logger.Logger, opts ...ConsoleOption) *Console {
	c := &Console{
		logger: log,
		output: os.Stdout,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	if c.output == nil {
		c.output = os.Stdout
	}
	if c.progress == nil {
		c.progress = logger.NewSpinnerProgress(c.output)
	}

	return c
}

// Logger exposes the underlying logger.
func (c *Console) Logger() logger.Logger {
	return c.logger
}

// Progress exposes the configured progress indicator.
func (c *Console) Progress() logger.Progress {
	return c.progress
}

// Success logs a success message with a consistent prefix.
func (c *Console) Success(format string, args ...interface{}) {
	if c.logger == nil {
		return
	}
	c.logger.Info("✓ "+format, args...)
}

// StartProgress starts the underlying progress indicator.
func (c *Console) StartProgress(operation string) {
	if c.progress == nil {
		return
	}
	c.progress.Start(operation)
}

// StopProgress stops the underlying progress indicator.
func (c *Console) StopProgress(operation string) {
	if c.progress == nil {
		return
	}
	c.progress.Stop(operation)
}

// WriteLine outputs formatted text without involving the logger.
func (c *Console) WriteLine(format string, args ...interface{}) {
	if c.output == nil {
		return
	}
	fmt.Fprintf(c.output, format+"\n", args...)
}
