package core

import (
	"context"
	stdErrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	apperrors "GWD/internal/errors"
)

const copyBufferSize = 32 * 1024

// HTTPClient represents the subset of http.Client methods required by the repository.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Repository orchestrates downloads for a given configuration.
type Repository struct {
	cfg      *DownloadConfig
	logger   Logger
	client   HTTPClient
	fs       FileSystem
	reporter ProgressReporter
}

// RepositoryOption customises Repository construction.
type RepositoryOption func(*Repository)

// WithHTTPClient overrides the HTTP client used for downloads.
func WithHTTPClient(client HTTPClient) RepositoryOption {
	return func(r *Repository) {
		r.client = client
	}
}

// WithFileSystem overrides the filesystem implementation.
func WithFileSystem(fs FileSystem) RepositoryOption {
	return func(r *Repository) {
		r.fs = fs
	}
}

// WithProgressReporter overrides the progress reporter implementation.
func WithProgressReporter(reporter ProgressReporter) RepositoryOption {
	return func(r *Repository) {
		r.reporter = reporter
	}
}

// NewRepository constructs a Repository using the provided configuration, logger and options.
func NewRepository(cfg *DownloadConfig, log Logger, opts ...RepositoryOption) (*Repository, error) {
	if cfg == nil {
		return nil, apperrors.New(apperrors.ErrCategoryConfig, apperrors.CodeConfigGeneric, "download configuration must not be nil", nil).
			WithModule("downloader.core").
			WithOperation("NewRepository")
	}
	if log == nil {
		return nil, apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "logger must not be nil", nil).
			WithModule("downloader.core").
			WithOperation("NewRepository")
	}

	copyCfg := *cfg
	if strings.TrimSpace(copyCfg.BaseURL) == "" {
		copyCfg.BaseURL = defaultArchiveBaseURL
	}
	copyCfg.BaseURL = strings.TrimRight(copyCfg.BaseURL, "/")

	copyCfg.Branch = strings.TrimSpace(copyCfg.Branch)
	if copyCfg.Branch == "" {
		copyCfg.Branch = "main"
	}

	if copyCfg.Timeout == 0 {
		copyCfg.Timeout = 300 * time.Second
	}
	if copyCfg.MaxRetries <= 0 {
		copyCfg.MaxRetries = 3
	}

	repo := &Repository{
		cfg:      &copyCfg,
		logger:   log,
		client:   defaultHTTPClient(copyCfg.Timeout),
		fs:       &OSFileSystem{},
		reporter: NewConsoleProgressReporter(nil),
	}

	for _, opt := range opts {
		opt(repo)
	}

	if repo.reporter == nil {
		repo.reporter = &NoopProgressReporter{}
	}
	if repo.fs == nil {
		repo.fs = &OSFileSystem{}
	}
	if repo.client == nil {
		repo.client = defaultHTTPClient(copyCfg.Timeout)
	}

	return repo, nil
}

// Download iterates downloads for the provided targets with a background context.
func (r *Repository) Download(targets []Target) error {
	return r.DownloadWithContext(context.Background(), targets)
}

// DownloadWithContext iterates downloads for the provided targets using the supplied context.
func (r *Repository) DownloadWithContext(ctx context.Context, targets []Target) error {
	for _, target := range targets {
		if err := r.downloadIfNeeded(ctx, target); err != nil {
			if appErr, ok := apperrors.As(err); ok {
				if appErr.Module == "" {
					appErr.WithModule("downloader.core")
				}
				if appErr.Operation == "" {
					appErr.WithOperation("DownloadWithContext")
				}
				appErr.WithField("target", target.Name)
				return appErr
			}
			return apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, "failed to download file", err).
				WithModule("downloader.core").
				WithOperation("DownloadWithContext").
				WithField("target", target.Name)
		}
	}
	return nil
}

// BuildTargets creates download targets for the provided architecture stored under repoDir.
func (r *Repository) BuildTargets(repoDir, arch string) ([]Target, error) {
	targets := make([]Target, 0, len(r.cfg.Components))

	for _, component := range r.cfg.Components {
		archivePath, ok := component.PathForArch(arch)
		if !ok {
			continue
		}

		displayName := component.DisplayName
		if displayName == "" {
			displayName = component.Name
		}

		target, err := r.newTarget(repoDir, displayName, archivePath, component.Name, component.MinSize, component.Executable, component.SupportResume)
		if err != nil {
			if appErr, ok := apperrors.As(err); ok {
				if appErr.Module == "" {
					appErr.WithModule("downloader.core")
				}
				if appErr.Operation == "" {
					appErr.WithOperation("BuildTargets")
				}
				appErr.WithField("component", component.Name)
				return nil, appErr
			}
			return nil, apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, "failed to prepare download target", err).
				WithModule("downloader.core").
				WithOperation("BuildTargets").
				WithField("component", component.Name)
		}

		targets = append(targets, target)
	}

	return targets, nil
}

func (r *Repository) newTarget(repoDir, displayName, archivePath, localName string, minSize int64, executable, supportResume bool) (Target, error) {
	hashURL := r.archiveURL(fmt.Sprintf("%s.sha256sum", archivePath))
	hash, err := r.fetchHash(context.Background(), hashURL)
	if err != nil {
		return Target{}, err
	}

	localPath := filepath.Join(repoDir, localName)

	return Target{
		Name:          displayName,
		URL:           r.archiveURL(archivePath),
		ExpectedHash:  hash,
		LocalPath:     localPath,
		TempPath:      localPath,
		MinSize:       minSize,
		Executable:    executable,
		SupportResume: supportResume,
	}, nil
}

func (r *Repository) downloadIfNeeded(ctx context.Context, target Target) error {
	needed, err := r.needsDownload(target.LocalPath, target.ExpectedHash)
	if err != nil {
		return err
	}

	if !needed {
		r.logger.Debug("%s is already the latest version, skipping download", target.Name)
		return nil
	}

	if err := r.prepareDownload(target); err != nil {
		return err
	}

	tempPath := target.TempPath
	if tempPath == "" {
		tempPath = target.LocalPath
	}

	if err := r.downloadFile(ctx, target.URL, tempPath, target.Name, target.SupportResume); err != nil {
		_ = r.fs.Remove(tempPath)
		return err
	}

	if err := r.finalizeDownload(tempPath, target); err != nil {
		_ = r.fs.Remove(tempPath)
		return err
	}

	r.logger.Success("%s downloaded successfully", target.Name)
	return nil
}

func (r *Repository) needsDownload(localPath, expectedHash string) (bool, error) {
	if _, err := r.fs.Stat(localPath); err != nil {
		if stdErrors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return true, apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to inspect local file", err).
			WithModule("downloader.core").
			WithOperation("needsDownload").
			WithField("path", localPath)
	}

	if err := ValidateChecksum(r.fs, localPath, expectedHash); err != nil {
		r.logger.Warn("Failed to validate local file %s: %v", localPath, err)
		return true, nil
	}

	return false, nil
}

func (r *Repository) prepareDownload(target Target) error {
	if err := r.fs.MkdirAll(filepath.Dir(target.LocalPath), 0o755); err != nil {
		return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to create directory", err).
			WithModule("downloader.core").
			WithOperation("prepareDownload").
			WithField("path", filepath.Dir(target.LocalPath))
	}

	if target.TempPath != "" {
		if err := r.fs.Remove(target.TempPath); err != nil && !stdErrors.Is(err, os.ErrNotExist) {
			return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to remove temporary file", err).
				WithModule("downloader.core").
				WithOperation("prepareDownload").
				WithField("temp_path", target.TempPath)
		}
	}

	return nil
}

func (r *Repository) finalizeDownload(tempPath string, target Target) error {
	if err := ValidateFileSize(r.fs, tempPath, target.MinSize); err != nil {
		if appErr, ok := apperrors.As(err); ok {
			return appErr.
				WithModule("downloader.core").
				WithOperation("finalizeDownload").
				WithField("target", target.Name)
		}
		return apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, "downloaded file size validation failed", err).
			WithModule("downloader.core").
			WithOperation("finalizeDownload").
			WithField("target", target.Name)
	}

	if err := ValidateChecksum(r.fs, tempPath, target.ExpectedHash); err != nil {
		if appErr, ok := apperrors.As(err); ok {
			return appErr.
				WithModule("downloader.core").
				WithOperation("finalizeDownload").
				WithField("target", target.Name)
		}
		return apperrors.New(apperrors.ErrCategoryDependency, apperrors.CodeDependencyGeneric, "downloaded file checksum validation failed", err).
			WithModule("downloader.core").
			WithOperation("finalizeDownload").
			WithField("target", target.Name)
	}

	if tempPath != target.LocalPath {
		if err := r.fs.Rename(tempPath, target.LocalPath); err != nil {
			return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to move downloaded file", err).
				WithModule("downloader.core").
				WithOperation("finalizeDownload").
				WithFields(apperrors.Metadata{
					"source": tempPath,
					"target": target.LocalPath,
				})
		}
	}

	if target.Executable {
		if err := r.fs.Chmod(target.LocalPath, 0o755); err != nil {
			return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to set execute permissions", err).
				WithModule("downloader.core").
				WithOperation("finalizeDownload").
				WithField("path", target.LocalPath)
		}
	}

	return nil
}

func (r *Repository) downloadFile(ctx context.Context, url, localPath, name string, supportResume bool) error {
	retries := r.cfg.MaxRetries
	if retries <= 0 {
		retries = 1
	}

	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		if attempt > 1 {
			r.logger.Info("Retrying download (attempt %d/%d): %s", attempt, retries, url)
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		if err := r.doDownload(ctx, url, localPath, name, supportResume); err != nil {
			lastErr = err
			r.logger.Warn("Download attempt %d failed: %v", attempt, err)
			continue
		}

		return nil
	}

	if lastErr != nil {
		if appErr, ok := apperrors.As(lastErr); ok {
			if appErr.Module == "" {
				appErr.WithModule("downloader.core")
			}
			if appErr.Operation == "" {
				appErr.WithOperation("downloadFile")
			}
			appErr.WithField("url", url)
			return appErr
		}
		return apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "download failed after retries", lastErr).
			WithModule("downloader.core").
			WithOperation("downloadFile").
			WithField("url", url)
	}

	return apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "download failed: exceeded retry limit", nil).
		WithModule("downloader.core").
		WithOperation("downloadFile").
		WithField("url", url)
}

func (r *Repository) doDownload(ctx context.Context, url, localPath, name string, supportResume bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "failed to create download request", err).
			WithModule("downloader.core").
			WithOperation("doDownload").
			WithField("url", url)
	}
	req.Header.Set("User-Agent", "GWD/1.0 (Go downloader)")

	resp, err := r.client.Do(req)
	if err != nil {
		return apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "download request failed", err).
			WithModule("downloader.core").
			WithOperation("doDownload").
			WithField("url", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "download failed with unexpected status", nil).
			WithModule("downloader.core").
			WithOperation("doDownload").
			WithFields(apperrors.Metadata{
				"url":    url,
				"status": resp.StatusCode,
			})
	}

	file, err := r.fs.Create(localPath)
	if err != nil {
		return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to create local file", err).
			WithModule("downloader.core").
			WithOperation("doDownload").
			WithField("path", localPath)
	}
	defer file.Close()

	totalSize := resp.ContentLength
	progressReader := NewProgressReader(resp.Body, totalSize, r.reporter, name)

	if supportResume {
		// Future enhancement: add resume support by seeking to current size and setting Range headers.
	}

	buf := make([]byte, copyBufferSize)
	if _, err := io.CopyBuffer(file, progressReader, buf); err != nil {
		return apperrors.New(apperrors.ErrCategorySystem, apperrors.CodeSystemGeneric, "failed to write file to disk", err).
			WithModule("downloader.core").
			WithOperation("doDownload").
			WithField("path", localPath)
	}

	progressReader.Finish()

	return nil
}

func (r *Repository) fetchHash(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "failed to create hash request", err).
			WithModule("downloader.core").
			WithOperation("fetchHash").
			WithField("url", url)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "failed to request hash file", err).
			WithModule("downloader.core").
			WithOperation("fetchHash").
			WithField("url", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "failed to fetch hash file", nil).
			WithModule("downloader.core").
			WithOperation("fetchHash").
			WithFields(apperrors.Metadata{
				"url":    url,
				"status": resp.StatusCode,
			})
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "failed to read hash file content", err).
			WithModule("downloader.core").
			WithOperation("fetchHash").
			WithField("url", url)
	}

	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", apperrors.NewRecoverable(apperrors.ErrCategoryNetwork, apperrors.CodeNetworkGeneric, "invalid hash file format", nil).
			WithModule("downloader.core").
			WithOperation("fetchHash").
			WithField("url", url)
	}

	return strings.TrimSpace(fields[0]), nil
}

func (r *Repository) archiveURL(parts ...string) string {
	segments := []string{r.cfg.Branch, "archive"}
	segments = append(segments, parts...)
	joined := path.Join(segments...)
	joined = strings.TrimPrefix(joined, "/")
	return fmt.Sprintf("%s/%s", r.cfg.BaseURL, joined)
}

func defaultHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
