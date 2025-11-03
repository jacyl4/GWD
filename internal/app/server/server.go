package server

import (
	"context"

	serverdownloader "GWD/internal/downloader/server"
	"GWD/internal/logger"
	menu "GWD/internal/menu/server"
	"GWD/internal/system"
)

type App struct {
	config    *system.SystemConfig
	logger    *logger.ColoredLogger
	menu      *menu.Menu
	installer *Installer
}

func NewServer(cfg *system.SystemConfig, log *logger.ColoredLogger) *App {
	repo := serverdownloader.New(cfg, log)

	menuManager := menu.NewMenu(cfg, log)

	app := &App{
		config: cfg,
		logger: log,
		menu:   menuManager,
	}

	app.installer = NewInstaller(cfg, log, repo)
	app.menu.SetInstallHandler(app.InstallGWD)

	return app
}

func (a *App) Run(ctx context.Context) error {
	return a.menu.ShowMainMenu()
}

// InstallGWD executes the full GWD installation process.
func (a *App) InstallGWD(domainConfig *menu.DomainInfo) error {
	return a.installer.InstallGWD(domainConfig)
}
