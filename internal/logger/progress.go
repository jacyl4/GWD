package logger

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// ProgressSpinner renders a spinner-style progress indicator.
type ProgressSpinner struct {
	mu       sync.Mutex
	output   io.Writer
	spinner  []string
	index    int
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewProgressSpinner creates a progress spinner writing to the provided output.
func NewProgressSpinner(output io.Writer) *ProgressSpinner {
	if output == nil {
		output = io.Discard
	}

	return &ProgressSpinner{
		output:  output,
		spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		stopCh:  make(chan struct{}),
	}
}

// Start begins rendering the progress spinner with the specified message.
func (p *ProgressSpinner) Start(message string) {
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
func (p *ProgressSpinner) Stop(message string) {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	fmt.Fprintf(p.output, "\r✓ %s\n", message)
}
