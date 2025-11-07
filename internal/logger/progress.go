package logger

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Progress describes progress indicators that can be started and stopped.
type Progress interface {
	Start(operation string)
	Stop(operation string)
}

// SpinnerProgress renders a spinner-style progress indicator.
type SpinnerProgress struct {
	mu       sync.Mutex
	output   io.Writer
	spinner  []string
	index    int
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewSpinnerProgress creates a progress spinner writing to the provided output.
func NewSpinnerProgress(output io.Writer) *SpinnerProgress {
	if output == nil {
		output = io.Discard
	}

	return &SpinnerProgress{
		output:  output,
		spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		stopCh:  make(chan struct{}),
	}
}

// Start begins rendering the progress spinner with the specified message.
func (p *SpinnerProgress) Start(message string) {
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
func (p *SpinnerProgress) Stop(message string) {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	fmt.Fprintf(p.output, "\r✓ %s\n", message)
}
