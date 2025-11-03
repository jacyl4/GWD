package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// ColoredLogger renders log messages using colours when supported by the output writer.
type ColoredLogger struct {
	*StandardLogger
	colors   map[Level]*color.Color
	progress struct {
		sync.Mutex
		active  *ProgressLogger
		message string
	}
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

// Success logs a successful operation using the info level.
func (l *ColoredLogger) Success(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.StandardLogger.Info("✓ %s", message)
}

// Progress starts an animated progress indicator for the supplied operation.
func (l *ColoredLogger) Progress(operation string) {
	l.progress.Lock()
	defer l.progress.Unlock()

	if l.progress.active != nil {
		l.progress.active.Stop(l.progress.message)
	}

	writer := l.output
	if writer == nil {
		writer = os.Stdout
	}

	progress := NewProgressLogger(writer)
	progress.Start(operation)
	l.progress.active = progress
	l.progress.message = operation
}

// ProgressDone stops the active progress indicator.
func (l *ColoredLogger) ProgressDone(operation string) {
	l.progress.Lock()
	defer l.progress.Unlock()

	if l.progress.active == nil {
		l.StandardLogger.Info("%s completed", operation)
		return
	}

	l.progress.active.Stop(operation)
	l.progress.active = nil
	l.progress.message = ""
}

// White prints plain text without level prefixes.
func (l *ColoredLogger) White(format string, args ...interface{}) {
	writer := l.output
	if writer == nil {
		writer = os.Stdout
	}
	fmt.Fprintf(writer, format+"\n", args...)
}

// ColoredFormatter renders log entries with coloured levels when enabled.
type ColoredFormatter struct {
	timestampFormat string
	colors          map[Level]*color.Color
	enableColors    bool
}

// Format converts the Entry into a coloured textual representation.
func (f *ColoredFormatter) Format(entry *Entry) ([]byte, error) {
	var buf bytes.Buffer

	timestampFormat := f.timestampFormat
	if timestampFormat == "" {
		timestampFormat = time.RFC3339
	}

	buf.WriteString(entry.Time.Format(timestampFormat))
	buf.WriteString(" ")

	level := entry.Level.String()
	if f.enableColors {
		if c := f.colors[entry.Level]; c != nil {
			level = c.Sprint(level)
		}
	}

	buf.WriteString("[")
	buf.WriteString(level)
	buf.WriteString("] ")

	buf.WriteString(entry.Message)

	for _, field := range entry.Fields {
		buf.WriteString(" ")
		fieldText := fmt.Sprintf("%s=%v", field.Key, field.Value)
		if f.enableColors {
			buf.WriteString(color.New(color.Faint).Sprint(fieldText))
		} else {
			buf.WriteString(fieldText)
		}
	}

	if entry.Caller != nil {
		buf.WriteString(" ")
		buf.WriteString("caller=")
		buf.WriteString(fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line))
	}

	buf.WriteString("\n")
	return buf.Bytes(), nil
}

func supportsColor(w io.Writer) bool {
	if file, ok := w.(*os.File); ok {
		return term.IsTerminal(int(file.Fd()))
	}
	return false
}
