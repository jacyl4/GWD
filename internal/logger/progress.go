package logger

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// ProgressLogger renders a spinner-style progress indicator.
type ProgressLogger struct {
	mu       sync.Mutex
	output   io.Writer
	spinner  []string
	index    int
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewProgressLogger creates a progress logger writing to the provided output.
func NewProgressLogger(output io.Writer) *ProgressLogger {
	if output == nil {
		output = io.Discard
	}

	return &ProgressLogger{
		output:  output,
		spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		stopCh:  make(chan struct{}),
	}
}

// Start begins rendering the progress spinner with the specified message.
func (p *ProgressLogger) Start(message string) {
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.mu.Lock()
				frame := p.spinner[p.index%len(p.spinner)]
				p.index++
				fmt.Fprintf(p.output, "\r%s %s", frame, message)
				p.mu.Unlock()
			}
		}
	}()
}

// Stop terminates the spinner and prints the final message.
func (p *ProgressLogger) Stop(message string) {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	fmt.Fprintf(p.output, "\r✓ %s\n", message)
}
