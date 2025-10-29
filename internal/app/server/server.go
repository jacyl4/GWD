package server

import (
	"context"

	"GWD/internal/downloader"
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
	log.SetStandardLogger()

	pkgMgr := system.NewDpkgManager()
	repo := downloader.NewRepository(cfg, log)

	return &App{
		config:    cfg,
		logger:    log,
		menu:      menu.NewMenu(cfg, log),
		installer: NewInstaller(cfg, log, pkgMgr, repo),
	}
}

func (a *App) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		a.logger.Info("Received shutdown signal")
		return nil
	default:
		return a.menu.ShowMainMenu()
	}
}

// InstallGWD executes the full GWD installation process.
func (a *App) InstallGWD(domainConfig *menu.DomainInfo) error {
	return a.installer.InstallGWD(domainConfig)
}
