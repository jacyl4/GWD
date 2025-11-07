package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// ColoredLogger renders log messages using colours when supported by the output writer.
type ColoredLogger struct {
	*StandardLogger
	colors map[Level]*color.Color
}

// NewColoredLogger returns a logger configured for colourful terminal output when possible.
func NewColoredLogger(options ...Option) *ColoredLogger {
	std := NewStandardLogger(options...)

	writer := std.output
	if writer == nil {
		writer = os.Stdout
	}

	useColor := supportsColor(writer) && os.Getenv("NO_COLOR") == ""

	colors := map[Level]*color.Color{
		LevelDebug: color.New(color.FgCyan),
		LevelInfo:  color.New(color.FgBlue),
		LevelWarn:  color.New(color.FgYellow),
		LevelError: color.New(color.FgRed),
	}

	std.formatter = &ColoredFormatter{
		timestampFormat: "15:04:05",
		colors:          colors,
		enableColors:    useColor,
	}

	return &ColoredLogger{
		StandardLogger: std,
		colors:         colors,
	}
}

// ColoredFormatter renders log entries with coloured levels when enabled.
type ColoredFormatter struct {
	timestampFormat string
	colors          map[Level]*color.Color
	enableColors    bool
}

// Format converts the Entry into a coloured textual representation.
func (f *ColoredFormatter) Format(entry *Entry) ([]byte, error) {
	timestampFormat := f.timestampFormat
	if timestampFormat == "" {
		timestampFormat = time.RFC3339
	}

	timestamp := entry.Time.Format(timestampFormat)

	level := entry.Level.String()
	if f.enableColors {
		if c := f.colors[entry.Level]; c != nil {
			level = c.Sprint(level)
		}
	}

	faint := color.New(color.Faint)
	fieldFormatter := func(field Field) string {
		fieldText := fmt.Sprintf("%s=%v", field.Key, field.Value)
		if f.enableColors {
			return faint.Sprint(fieldText)
		}
		return fieldText
	}

	return formatEntry(entry, timestamp, level, fieldFormatter), nil
}

func supportsColor(w io.Writer) bool {
	if file, ok := w.(*os.File); ok {
		return term.IsTerminal(int(file.Fd()))
	}
	return false
}
