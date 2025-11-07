package menu

import (
	"fmt"

	"GWD/internal/logger"
	"GWD/internal/system"
	ui "GWD/internal/ui/server"
)

// Menu coordinates interactive workflow for the server deployment UI.
type Menu struct {
	config         *system.Config
	console        *ui.Console
	logger         logger.Logger
	printer        *ui.Printer
	sysProbe       SystemProbe
	installHandler func(*DomainInfo) error
}

// NewMenu creates a new menu manager instance.
func NewMenu(cfg *system.Config, console *ui.Console) *Menu {
	var log logger.Logger = logger.NewStandardLogger()
	if console != nil && console.Logger() != nil {
		log = console.Logger()
	}

	probe := newExecSystemProbe(cfg)

	return &Menu{
		config:   cfg,
		console:  console,
		logger:   log,
		printer:  ui.NewPrinter(),
		sysProbe: probe,
	}
}

// SetInstallHandler registers the handler that executes the full installation workflow.
func (m *Menu) SetInstallHandler(handler func(*DomainInfo) error) {
	m.installHandler = handler
}

// ShowMainMenu displays the interactive menu until the user quits.
func (m *Menu) ShowMainMenu() error {
	for {
		m.clearScreen()
		status := m.collectSystemStatus()
		m.displaySystemStatus(status)

		m.printer.PrintBanner()
		options := m.buildMenuOptions()

		selected, err := m.promptUserSelection(options)
		if err != nil {
			if err.Error() == "^C" {
				m.logger.Info("User cancelled operation")
				return nil
			}
			return fmt.Errorf("failed to process user input: %w", err)
		}

		if err := options[selected].Handler(); err != nil {
			m.logger.Error("Operation failed: %v", err)
			m.waitForUserInput("\nPress Enter to continue...")
		}
	}
}

func (m *Menu) buildMenuOptions() []MenuOption {
	options := []MenuOption{
		{
			Label:       "1. Install GWD",
			Description: "Fresh installation of GWD reverse proxy system",
			Handler:     m.handleInstallGWD,
			Color:       "green",
			Enabled:     true,
		},
	}

	// Placeholder for future non-container specific options.
	return options
}

func (m *Menu) clearScreen() {
	fmt.Print("\033[H\033[2J")
}
