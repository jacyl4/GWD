package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// Formatter converts log entries to their textual or structured representation.
type Formatter interface {
	Format(entry *Entry) ([]byte, error)
}

// Entry represents a single log record.
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
	Fields  []Field
	Caller  *Caller
}

// Caller carries caller information when caller reporting is enabled.
type Caller struct {
	File     string
	Line     int
	Function string
}

// TextFormatter renders log entries using a textual format similar to traditional log output.
type TextFormatter struct {
	TimestampFormat  string
	DisableColors    bool
	DisableTimestamp bool
	FullTimestamp    bool
	ForceColors      bool
	Output           io.Writer
}

// Format converts the Entry into a textual representation.
func (f *TextFormatter) Format(entry *Entry) ([]byte, error) {
	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = time.RFC3339
	}

	var timestamp string
	if !f.DisableTimestamp {
		if f.FullTimestamp {
			timestamp = entry.Time.Format(timestampFormat)
		} else {
			timestamp = entry.Time.Format("15:04:05")
		}
	}

	levelText := entry.Level.String()
	if f.shouldColorize() {
		levelText = f.colorize(levelText, entry.Level)
	}
	return formatEntry(entry, timestamp, levelText, nil), nil
}

func (f *TextFormatter) shouldColorize() bool {
	if f == nil {
		return false
	}
	if f.ForceColors {
		return true
	}
	if f.DisableColors {
		return false
	}

	writer := f.Output
	if writer == nil {
		writer = os.Stdout
	}
	return isTerminal(writer)
}

func (f *TextFormatter) colorize(text string, level Level) string {
	var c *color.Color
	switch level {
	case LevelDebug:
		c = color.New(color.FgCyan)
	case LevelInfo:
		c = color.New(color.FgBlue)
	case LevelWarn:
		c = color.New(color.FgYellow)
	case LevelError:
		c = color.New(color.FgRed)
	default:
		return text
	}
	return c.Sprint(text)
}

func isTerminal(w io.Writer) bool {
	if file, ok := w.(*os.File); ok {
		return term.IsTerminal(int(file.Fd()))
	}
	return false
}

// JSONFormatter renders log entries as JSON objects.
type JSONFormatter struct {
	TimestampFormat string
	PrettyPrint     bool
}

// Format converts the Entry into JSON.
func (f *JSONFormatter) Format(entry *Entry) ([]byte, error) {
	data := make(map[string]interface{})

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = time.RFC3339
	}

	data["time"] = entry.Time.Format(timestampFormat)
	data["level"] = entry.Level.String()
	data["msg"] = entry.Message

	for _, field := range entry.Fields {
		data[field.Key] = field.Value
	}

	if entry.Caller != nil {
		data["caller"] = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
	}

	var (
		bytes []byte
		err   error
	)
	if f.PrettyPrint {
		bytes, err = json.MarshalIndent(data, "", "  ")
	} else {
		bytes, err = json.Marshal(data)
	}
	if err != nil {
		return nil, err
	}

	if !f.PrettyPrint {
		bytes = append(bytes, '\n')
	} else {
		bytes = append(bytes, '\n')
	}

	return bytes, nil
}

type fieldFormatter func(Field) string

func defaultFieldFormatter(field Field) string {
	return fmt.Sprintf("%s=%v", field.Key, field.Value)
}

func formatEntry(entry *Entry, timestamp, levelText string, formatter fieldFormatter) []byte {
	if formatter == nil {
		formatter = defaultFieldFormatter
	}

	var buf bytes.Buffer

	if timestamp != "" {
		buf.WriteString(timestamp)
		buf.WriteString(" ")
	}

	buf.WriteString("[")
	buf.WriteString(levelText)
	buf.WriteString("] ")

	buf.WriteString(entry.Message)

	for _, field := range entry.Fields {
		buf.WriteString(" ")
		buf.WriteString(formatter(field))
	}

	if entry.Caller != nil {
		buf.WriteString(" ")
		buf.WriteString("caller=")
		buf.WriteString(fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line))
	}

	buf.WriteString("\n")
	return buf.Bytes()
}
