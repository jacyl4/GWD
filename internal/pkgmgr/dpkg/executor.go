package dpkg

import (
	"os"
	"os/exec"
)

// Executor abstracts command execution to ease testing.
type Executor interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

// SystemExecutor executes commands using the local OS.
type SystemExecutor struct{}

func (SystemExecutor) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (SystemExecutor) Output(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.Output()
}
