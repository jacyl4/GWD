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

// NewConsole builds a Console bound to the provided logger.
func NewConsole(log logger.Logger, output io.Writer) *Console {
	c := &Console{
		logger: log,
		output: output,
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
