package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	_ "embed"

	"GWD/internal/downloader/core"
	apperrors "GWD/internal/errors"
	"GWD/internal/logger"
	"GWD/internal/system"
	ui "GWD/internal/ui/server"
)

//go:embed server-extra.yaml
var serverExtraConfig []byte

// Downloader orchestrates server-side asset downloads.
type Downloader struct {
	repo    *core.Repository
	cfg     *system.Config
	console *ui.Console
	logger  logger.Logger
}

// New creates a server downloader bound to the provided config and logger.
func New(cfg *system.Config, console *ui.Console) (*Downloader, error) {
	var log logger.Logger
	if console != nil {
		log = console.Logger()
	}
	if log == nil {
		log = logger.NewStandardLogger()
	}
	if console == nil {
		console = ui.NewConsole(log, nil)
	}

	baseCfg, err := core.BaseConfig()
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, "failed to load base download configuration", err).
			WithModule("downloader.server").
			WithOperation("New")
	}

	extraCfg, err := core.ParseConfig(serverExtraConfig)
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCategoryConfig, apperrors.CodeConfigGeneric, "failed to parse server download configuration", err).
			WithModule("downloader.server").
			WithOperation("New")
	}

	mergedCfg, err := core.MergeConfigs(baseCfg, extraCfg)
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCategoryConfig, apperrors.CodeConfigGeneric, "failed to merge download configurations", err).
			WithModule("downloader.server").
			WithOperation("New")
	}

	if branch := strings.TrimSpace(cfg.Branch); branch != "" {
		mergedCfg.Branch = branch
	}

	repo, err := core.NewRepository(mergedCfg, log)
	if err != nil {
		return nil, err
	}

	return &Downloader{
		repo:    repo,
		cfg:     cfg,
		console: console,
		logger:  log,
	}, nil
}

// DownloadAll installs the server-side assets required for GWD.
func (d *Downloader) DownloadAll() error {
	d.console.StartProgress("Checking repository files")

	repoDir := d.cfg.GetRepoDir()
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to create repository directory", err).
			WithModule("downloader.server").
			WithOperation("DownloadAll").
			WithField("repo_dir", repoDir)
	}

	targets, err := d.repo.BuildTargets(repoDir, d.cfg.Architecture)
	if err != nil {
		return apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, "failed to prepare download targets", err).
			WithModule("downloader.server").
			WithOperation("DownloadAll")
	}

	templatesTcsssDir := filepath.Join(repoDir, "templates_tcsss")
	if err := os.RemoveAll(templatesTcsssDir); err != nil {
		return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to clear templates_tcsss directory", err).
			WithModule("downloader.server").
			WithOperation("DownloadAll").
			WithField("dir", templatesTcsssDir)
	}
	if err := os.MkdirAll(templatesTcsssDir, 0o755); err != nil {
		return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to recreate templates_tcsss directory", err).
			WithModule("downloader.server").
			WithOperation("DownloadAll").
			WithField("dir", templatesTcsssDir)
	}

	if err := d.repo.DownloadWithContext(context.Background(), targets); err != nil {
		return err
	}

	d.console.StopProgress("Checking repository files")
	return nil
}
