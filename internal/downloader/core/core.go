package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"GWD/internal/system"

	"github.com/pkg/errors"
)

const archiveRepoBaseURL = "https://raw.githubusercontent.com/jacyl4/GWD"

// Logger abstracts the logging methods used by the downloader packages.
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Success(format string, args ...interface{})
	Progress(operation string)
	ProgressDone(operation string)
}

// Repository manages the download workflow for a given configuration/logger pair.
type Repository struct {
	config    *system.SystemConfig
	logger    Logger
	validator *Validator
	client    *http.Client
}

// Target describes a single downloadable asset.
type Target struct {
	Name         string
	URL          string
	ExpectedHash string
	LocalPath    string
	TempPath     string
	MinSize      int64
	Executable   bool
}

// ProgressReader wraps an io.Reader to track download progress.
type ProgressReader struct {
	reader     io.Reader
	total      int64
	current    int64
	logger     Logger
	fileName   string
	startTime  time.Time
	lastUpdate time.Time
}

// NewProgressReader creates a new progress tracking reader.
func NewProgressReader(reader io.Reader, total int64, logger Logger, fileName string) *ProgressReader {
	now := time.Now()
	return &ProgressReader{
		reader:     reader,
		total:      total,
		current:    0,
		logger:     logger,
		fileName:   fileName,
		startTime:  now,
		lastUpdate: now,
	}
}

// Finish displays the final 100% progress bar.
func (pr *ProgressReader) Finish() {
	if pr.total <= 0 {
		fmt.Println()
		return
	}
	
	current := atomic.LoadInt64(&pr.current)
	elapsed := time.Since(pr.startTime).Seconds()
	if elapsed < 0.001 {
		elapsed = 0.001
	}
	speed := float64(current) / elapsed / 1024 / 1024
	
	// Create full progress bar
	barWidth := 30
	bar := strings.Repeat("=", barWidth)
	
	fmt.Printf("\r  %s: [%s] 100.0%% (%.2f/%.2f MB) %.2f MB/s\n",
		pr.fileName,
		bar,
		float64(current)/1024/1024,
		float64(pr.total)/1024/1024,
		speed,
	)
}

// Read implements io.Reader and updates progress.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&pr.current, int64(n))
		pr.updateProgress()
	}
	return n, err
}

// updateProgress prints the download progress bar.
func (pr *ProgressReader) updateProgress() {
	now := time.Now()
	// Update progress every 200ms to make it smoother
	if now.Sub(pr.lastUpdate) < 200*time.Millisecond {
		return
	}
	pr.lastUpdate = now

	current := atomic.LoadInt64(&pr.current)
	
	if pr.total > 0 {
		percentage := float64(current) / float64(pr.total) * 100
		elapsed := now.Sub(pr.startTime).Seconds()
		speed := float64(current) / elapsed / 1024 / 1024 // MB/s
		
		// Create visual progress bar
		barWidth := 30
		filledWidth := int(float64(barWidth) * percentage / 100)
		bar := strings.Repeat("=", filledWidth)
		if filledWidth < barWidth {
			bar += ">"
			bar += strings.Repeat(" ", barWidth-filledWidth-1)
		}
		
		// Use \r to overwrite the same line
		fmt.Printf("\r  %s: [%s] %.1f%% (%.2f/%.2f MB) %.2f MB/s",
			pr.fileName,
			bar,
			percentage,
			float64(current)/1024/1024,
			float64(pr.total)/1024/1024,
			speed,
		)
	} else {
		// Unknown size, just show downloaded amount
		fmt.Printf("\r  %s: %.2f MB downloaded",
			pr.fileName,
			float64(current)/1024/1024,
		)
	}
}

// NewRepository constructs a Repository with sensible HTTP defaults.
func NewRepository(cfg *system.SystemConfig, log Logger) *Repository {
	client := &http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: true,
		},
	}

	return &Repository{
		config:    cfg,
		logger:    log,
		validator: NewValidator(log),
		client:    client,
	}
}

// Download iterates downloads for the provided targets, performing validation for each.
func (r *Repository) Download(targets []Target) error {
	for _, target := range targets {
		if err := r.downloadIfNeeded(target); err != nil {
			return errors.Wrapf(err, "Failed to download file: %s", target.Name)
		}
	}
	return nil
}

// NewTarget prepares a Target definition, fetching checksum information eagerly.
func (r *Repository) NewTarget(repoDir, name, archivePath, localName string, minSize int64, executable bool) (Target, error) {
	hashURL := r.archiveURL(fmt.Sprintf("%s.sha256sum", archivePath))
	hash, err := r.fetchHash(hashURL)
	if err != nil {
		return Target{}, errors.Wrapf(err, "Failed to fetch hash for %s", name)
	}

	localPath := filepath.Join(repoDir, localName)

	return Target{
		Name:         name,
		URL:          r.archiveURL(archivePath),
		ExpectedHash: hash,
		LocalPath:    localPath,
		TempPath:     localPath,
		MinSize:      minSize,
		Executable:   executable,
	}, nil
}

func (r *Repository) downloadIfNeeded(target Target) error {
	needsDownload := true
	if _, err := os.Stat(target.LocalPath); err == nil {
		valid, err := r.validator.ValidateFile(target.LocalPath, target.ExpectedHash)
		if err != nil {
			r.logger.Warn("Failed to validate local file %s: %v", target.Name, err)
		} else if valid {
			needsDownload = false
		}
	}

	if !needsDownload {
		r.logger.Debug("%s is already the latest version, skipping download", target.Name)
		return nil
	}

	tempPath := target.TempPath
	if tempPath == "" {
		tempPath = target.LocalPath
	}

	if err := os.MkdirAll(filepath.Dir(target.LocalPath), 0755); err != nil {
		return errors.Wrapf(err, "Failed to create directory: %s", filepath.Dir(target.LocalPath))
	}

	os.Remove(tempPath)

	if err := r.downloadFile(target.URL, tempPath); err != nil {
		return errors.Wrapf(err, "Download failed: %s", target.Name)
	}

	if err := r.validator.VerifyDownload(tempPath, target.ExpectedHash, target.MinSize); err != nil {
		os.Remove(tempPath)
		return errors.Wrapf(err, "File validation failed: %s", target.Name)
	}

	if tempPath != target.LocalPath {
		if err := os.Rename(tempPath, target.LocalPath); err != nil {
			return errors.Wrapf(err, "Failed to move file: %s -> %s", tempPath, target.LocalPath)
		}
	}

	if target.Executable {
		if err := os.Chmod(target.LocalPath, 0755); err != nil {
			return errors.Wrapf(err, "Failed to set execute permissions: %s", target.LocalPath)
		}
	}

	r.logger.Success("%s downloaded successfully", target.Name)
	return nil
}

func (r *Repository) downloadFile(url, localPath string) error {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			r.logger.Info("Retrying download (attempt %d/%d): %s", attempt, maxRetries, url)
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		if err := r.doDownload(url, localPath); err != nil {
			if attempt == maxRetries {
				return err
			}
			r.logger.Warn("Download failed, retrying: %v", err)
			continue
		}

		return nil
	}

	return errors.New("Download retry limit exceeded")
}

func (r *Repository) doDownload(url, localPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create download request")
	}

	req.Header.Set("User-Agent", "GWD/1.0 (Go downloader)")

	resp, err := r.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "Download request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("Download failed, HTTP status code: %d", resp.StatusCode)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to create temporary file: %s", localPath)
	}
	defer file.Close()

	// Get file size from Content-Length header
	totalSize := resp.ContentLength
	
	// Extract filename from path
	fileName := filepath.Base(localPath)
	
	// Wrap response body with progress reader
	progressReader := NewProgressReader(resp.Body, totalSize, r.logger, fileName)
	
	if _, err = io.Copy(file, progressReader); err != nil {
		return errors.Wrap(err, "Failed to write file")
	}
	
	// Display final 100% progress bar
	progressReader.Finish()

	return nil
}

func (r *Repository) fetchHash(url string) (string, error) {
	resp, err := r.client.Get(url)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to request hash file: %s", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Failed to fetch hash file, status code: %d, URL: %s",
			resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "Failed to read hash file content")
	}

	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", errors.Errorf("Invalid hash file format: %s", url)
	}
	return strings.TrimSpace(fields[0]), nil
}

func (r *Repository) archiveURL(parts ...string) string {
	segments := []string{r.configBranch(), "archive"}
	segments = append(segments, parts...)
	joined := path.Join(segments...)
	joined = strings.TrimPrefix(joined, "/")
	return fmt.Sprintf("%s/%s", archiveRepoBaseURL, joined)
}

func (r *Repository) configBranch() string {
	if branch := strings.TrimSpace(r.config.Branch); branch != "" {
		return branch
	}
	return "main"
}
