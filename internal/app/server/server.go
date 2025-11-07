package server

import (
	"context"

	serverdownloader "GWD/internal/downloader/server"
	"GWD/internal/logger"
	menu "GWD/internal/menu/server"
	"GWD/internal/system"
	ui "GWD/internal/ui/server"
)

type App struct {
	config    *system.Config
	console   *ui.Console
	menu      *menu.Menu
	installer *Installer
}

func NewServer(cfg *system.Config, log *logger.ColoredLogger) (*App, error) {
	if log == nil {
		log = logger.NewColoredLogger()
	}

	console := ui.NewConsole(log, nil)

	repo, err := serverdownloader.New(cfg, console)
	if err != nil {
		return nil, err
	}

	menuManager := menu.NewMenu(cfg, console)

	app := &App{
		config:  cfg,
		console: console,
		menu:    menuManager,
	}

	validator := NewEnvironmentValidator(cfg, log)

	app.installer = NewInstaller(cfg, console, repo, validator)
	app.menu.SetInstallHandler(app.InstallGWD)

	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	return a.menu.ShowMainMenu()
}

// InstallGWD executes the full GWD installation process.
func (a *App) InstallGWD(domainConfig *menu.DomainInfo) error {
	cfg, err := InstallConfigFromDomainInfo(domainConfig)
	if err != nil {
		return err
	}
	return a.installer.InstallGWD(cfg)
}
