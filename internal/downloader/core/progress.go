package core

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ProgressReporter receives download progress updates.
type ProgressReporter interface {
	OnStart(fileName string, totalSize int64)
	OnProgress(fileName string, current, total int64, speed float64)
	OnComplete(fileName string, totalSize int64, elapsed time.Duration)
}

// NoopProgressReporter discards all progress events.
type NoopProgressReporter struct{}

func (n *NoopProgressReporter) OnStart(string, int64)                    {}
func (n *NoopProgressReporter) OnProgress(string, int64, int64, float64) {}
func (n *NoopProgressReporter) OnComplete(string, int64, time.Duration)  {}

// ConsoleProgressReporter renders progress updates to a writer (defaults to stdout).
type ConsoleProgressReporter struct {
	writer     io.Writer
	lastUpdate time.Time
}

// NewConsoleProgressReporter constructs a ConsoleProgressReporter.
func NewConsoleProgressReporter(w io.Writer) *ConsoleProgressReporter {
	if w == nil {
		w = os.Stdout
	}
	return &ConsoleProgressReporter{
		writer:     w,
		lastUpdate: time.Now(),
	}
}

func (c *ConsoleProgressReporter) OnStart(fileName string, totalSize int64) {
	fmt.Fprintf(c.writer, "  %s: starting download (%.2f MB total)\n", fileName, float64(totalSize)/1024/1024)
	c.lastUpdate = time.Now()
}

func (c *ConsoleProgressReporter) OnProgress(fileName string, current, total int64, speed float64) {
	now := time.Now()
	if now.Sub(c.lastUpdate) < 200*time.Millisecond {
		return
	}
	c.lastUpdate = now

	if total > 0 {
		percentage := float64(current) / float64(total) * 100
		barWidth := 30
		filledWidth := int(float64(barWidth) * percentage / 100)
		if filledWidth > barWidth {
			filledWidth = barWidth
		}
		bar := strings.Repeat("=", filledWidth)
		if filledWidth < barWidth {
			bar += ">"
			bar += strings.Repeat(" ", barWidth-filledWidth-1)
		}

		fmt.Fprintf(c.writer, "\r  %s: [%s] %.1f%% (%.2f/%.2f MB) %.2f MB/s",
			fileName,
			bar,
			percentage,
			float64(current)/1024/1024,
			float64(total)/1024/1024,
			speed,
		)
	} else {
		fmt.Fprintf(c.writer, "\r  %s: %.2f MB downloaded", fileName, float64(current)/1024/1024)
	}
}

func (c *ConsoleProgressReporter) OnComplete(fileName string, totalSize int64, elapsed time.Duration) {
	speed := float64(totalSize) / elapsed.Seconds() / 1024 / 1024
	if elapsed.Seconds() <= 0 {
		speed = 0
	}
	fmt.Fprintf(c.writer, "\r  %s: [%s] 100.0%% (%.2f/%.2f MB) %.2f MB/s\n",
		fileName,
		strings.Repeat("=", 30),
		float64(totalSize)/1024/1024,
		float64(totalSize)/1024/1024,
		speed,
	)
}

// ProgressReader wraps a reader to emit progress updates.
type ProgressReader struct {
	reader    io.Reader
	total     int64
	current   int64
	reporter  ProgressReporter
	fileName  string
	startTime time.Time
}

// NewProgressReader constructs a progress tracking reader.
func NewProgressReader(reader io.Reader, total int64, reporter ProgressReporter, fileName string) *ProgressReader {
	if reporter == nil {
		reporter = &NoopProgressReporter{}
	}

	pr := &ProgressReader{
		reader:    reader,
		total:     total,
		reporter:  reporter,
		fileName:  fileName,
		startTime: time.Now(),
	}

	reporter.OnStart(fileName, total)

	return pr
}

// Read implements io.Reader and relays progress.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.current += int64(n)
		elapsed := time.Since(pr.startTime).Seconds()
		if elapsed <= 0 {
			elapsed = 0.001
		}
		speed := float64(pr.current) / elapsed / 1024 / 1024
		pr.reporter.OnProgress(pr.fileName, pr.current, pr.total, speed)
	}
	return n, err
}

// Finish notifies the reporter that the download has completed.
func (pr *ProgressReader) Finish() {
	elapsed := time.Since(pr.startTime)
	pr.reporter.OnProgress(pr.fileName, pr.total, pr.total, 0)
	pr.reporter.OnComplete(pr.fileName, pr.total, elapsed)
}
