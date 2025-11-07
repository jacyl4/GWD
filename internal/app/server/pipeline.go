package server

import (
	apperrors "GWD/internal/errors"
	"GWD/internal/logger"
	ui "GWD/internal/ui/server"
)

// InstallStep describes a single installation phase.
type InstallStep struct {
	Name      string
	Operation string
	Category  apperrors.ErrorCategory
	Fn        func() error
}

// StepErrorHandler handles step failures.
type StepErrorHandler func(step InstallStep, err error) error

// Pipeline executes installation steps sequentially.
type Pipeline struct {
	steps   []InstallStep
	console *ui.Console
	logger  logger.Logger
	onError StepErrorHandler
}

// NewPipeline constructs a new pipeline.
func NewPipeline(console *ui.Console, log logger.Logger, steps []InstallStep, handler StepErrorHandler) *Pipeline {
	return &Pipeline{
		steps:   steps,
		console: console,
		logger:  log,
		onError: handler,
	}
}

// Execute runs through all configured steps.
func (p *Pipeline) Execute() error {
	for _, step := range p.steps {
		if p.logger != nil {
			p.logger.Debug("Executing step: %s", step.Name)
		}
		p.console.StartProgress(step.Name)
		if err := step.Fn(); err != nil {
			if p.onError != nil {
				return p.onError(step, err)
			}
			return err
		}

		p.console.StopProgress(step.Name)
	}

	return nil
}
