package server

import (
	"os"
	"path"

	"GWD/internal/downloader/core"
	"GWD/internal/system"

	"github.com/pkg/errors"
)

var binaryPaths = map[string]map[string][]string{
	"amd64": {
		"doh":      {"doh", "doh_s"},
		"nginx":    {"nginx", "nginx"},
		"smartdns": {"smartdns", "smartdns-x86_64"},
		"tcsss":    {"tcsss", "tcsss"},
		"vtrui":    {"vtrui", "vtrui"},
	},
	"arm64": {
		"doh":      {"doh", "doh_s_arm64"},
		"nginx":    {"nginx", "nginx_arm64"},
		"smartdns": {"smartdns", "smartdns-aarch64"},
		"tcsss":    {"tcsss", "tcsss-arm64"},
		"vtrui":    {"vtrui", "vtrui_arm64"},
	},
}

// Downloader orchestrates server-side asset downloads.
type Downloader struct {
	repo *core.Repository
	cfg  *system.SystemConfig
	log  core.Logger
}

// New creates a server downloader bound to the provided config and logger.
func New(cfg *system.SystemConfig, log core.Logger) *Downloader {
	return &Downloader{
		repo: core.NewRepository(cfg, log),
		cfg:  cfg,
		log:  log,
	}
}

// DownloadAll installs the server-side assets required for GWD.
func (d *Downloader) DownloadAll() error {
	d.log.Progress("Checking repository files")

	repoDir := d.cfg.GetRepoDir()
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return errors.Wrapf(err, "Failed to create repository directory: %s", repoDir)
	}

	targets, err := d.buildTargets()
	if err != nil {
		return errors.Wrap(err, "Failed to prepare download targets")
	}

	if err := d.repo.Download(targets); err != nil {
		return err
	}

	d.log.ProgressDone("Checking repository files")
	return nil
}

func (d *Downloader) buildTargets() ([]core.Target, error) {
	repoDir := d.cfg.GetRepoDir()

	type binarySpec struct {
		name       string
		component  string
		localName  string
		minSize    int64
		executable bool
	}

	binaries := []binarySpec{
		{name: "DoH Server", component: "doh", localName: "doh-server", minSize: 1024 * 1024, executable: true},
		{name: "Nginx", component: "nginx", localName: "nginx", minSize: 1024 * 1024 * 2, executable: true},
		{name: "SmartDNS", component: "smartdns", localName: "smartdns", minSize: 512 * 1024, executable: true},
		{name: "TCSSS", component: "tcsss", localName: "tcsss", minSize: 512 * 1024, executable: true},
		{name: "vtrui", component: "vtrui", localName: "vtrui", minSize: 512 * 1024, executable: true},
	}

	targets := make([]core.Target, 0, len(binaries)+2)

	for _, spec := range binaries {
		archivePath := d.binaryPath(spec.component)
		target, err := d.repo.NewTarget(repoDir, spec.name, archivePath, spec.localName, spec.minSize, spec.executable)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}

	staticSpecs := []struct {
		name      string
		archive   string
		localName string
		minSize   int64
	}{
		{name: "Nginx Configuration", archive: path.Join("nginx", "nginxConf.zip"), localName: "nginxConf.zip", minSize: 1024 * 5},
		{name: "Sample Configuration", archive: "sample.zip", localName: "sample.zip", minSize: 1024 * 5},
	}

	for _, spec := range staticSpecs {
		target, err := d.repo.NewTarget(repoDir, spec.name, spec.archive, spec.localName, spec.minSize, false)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}

	return targets, nil
}

func (d *Downloader) binaryPath(component string) string {
	if archPaths, ok := binaryPaths[d.cfg.Architecture]; ok {
		if segments, ok := archPaths[component]; ok {
			return path.Join(segments...)
		}
	}

	if segments, ok := binaryPaths["amd64"][component]; ok {
		return path.Join(segments...)
	}

	return component
}
