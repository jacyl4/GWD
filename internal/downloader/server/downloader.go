package server

import (
	"context"
	"os"
	"strings"

	_ "embed"

	"GWD/internal/downloader/core"
	apperrors "GWD/internal/errors"
	"GWD/internal/logger"
	"GWD/internal/system"
)

//go:embed server-extra.yaml
var serverExtraConfig []byte

// Downloader orchestrates server-side asset downloads.
type Downloader struct {
	repo *core.Repository
	cfg  *system.SystemConfig
	log  logger.ProgressLogger
}

// New creates a server downloader bound to the provided config and logger.
func New(cfg *system.SystemConfig, log logger.ProgressLogger) (*Downloader, error) {
	baseCfg, err := core.BaseConfig()
	if err != nil {
		return nil, apperrors.DependencyError(apperrors.CodeDependencyGeneric, "failed to load base download configuration", err).
			WithModule("downloader.server").
			WithOperation("New")
	}

	extraCfg, err := core.ParseConfig(serverExtraConfig)
	if err != nil {
		return nil, apperrors.ConfigError(apperrors.CodeConfigGeneric, "failed to parse server download configuration", err).
			WithModule("downloader.server").
			WithOperation("New")
	}

	mergedCfg, err := core.MergeConfigs(baseCfg, extraCfg)
	if err != nil {
		return nil, apperrors.ConfigError(apperrors.CodeConfigGeneric, "failed to merge download configurations", err).
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
		repo: repo,
		cfg:  cfg,
		log:  log,
	}, nil
}

// DownloadAll installs the server-side assets required for GWD.
func (d *Downloader) DownloadAll() error {
	d.log.Progress("Checking repository files")

	repoDir := d.cfg.GetRepoDir()
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return apperrors.SystemError(apperrors.CodeSystemGeneric, "failed to create repository directory", err).
			WithModule("downloader.server").
			WithOperation("DownloadAll").
			WithField("repo_dir", repoDir)
	}

	targets, err := d.repo.BuildTargets(repoDir, d.cfg.Architecture)
	if err != nil {
		return apperrors.DependencyError(apperrors.CodeDependencyGeneric, "failed to prepare download targets", err).
			WithModule("downloader.server").
			WithOperation("DownloadAll")
	}

	if err := d.repo.DownloadWithContext(context.Background(), targets); err != nil {
		return err
	}

	d.log.ProgressDone("Checking repository files")
	return nil
}
