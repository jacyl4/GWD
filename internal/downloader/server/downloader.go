package server

import (
	"context"
	"os"
	"strings"

	_ "embed"

	"GWD/internal/downloader/core"
	"GWD/internal/system"

	"github.com/pkg/errors"
)

//go:embed server-extra.yaml
var serverExtraConfig []byte

// Downloader orchestrates server-side asset downloads.
type Downloader struct {
	repo *core.Repository
	cfg  *system.SystemConfig
	log  core.Logger
}

// New creates a server downloader bound to the provided config and logger.
func New(cfg *system.SystemConfig, log core.Logger) (*Downloader, error) {
	baseCfg, err := core.BaseConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load base download configuration")
	}

	extraCfg, err := core.ParseConfig(serverExtraConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse server download configuration")
	}

	mergedCfg, err := core.MergeConfigs(baseCfg, extraCfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to merge download configurations")
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
		return errors.Wrapf(err, "failed to create repository directory: %s", repoDir)
	}

	targets, err := d.repo.BuildTargets(repoDir, d.cfg.Architecture)
	if err != nil {
		return errors.Wrap(err, "failed to prepare download targets")
	}

	if err := d.repo.DownloadWithContext(context.Background(), targets); err != nil {
		return err
	}

	d.log.ProgressDone("Checking repository files")
	return nil
}
