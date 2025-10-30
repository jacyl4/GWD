package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"GWD/internal/system"

	"github.com/pkg/errors"
)

const archiveBaseURL = "https://raw.githubusercontent.com/jacyl4/GWD/main/archive"

// Logger abstracts the logging methods used by the downloader package.
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Success(format string, args ...interface{})
	Progress(operation string)
	ProgressDone(operation string)
}

// Repository is the repository download manager
// Manages the entire lifecycle of downloading files from a GitHub repository
type Repository struct {
	config    *system.SystemConfig
	logger    Logger
	validator *Validator
	client    *http.Client
}

// DownloadTarget defines a download target
// Describes the download information for a single file, including URL, checksum, and save location
type DownloadTarget struct {
	// Name of the file, used for logging
	Name string

	// URL for download, usually pointing to a raw file on GitHub
	URL string

	// ExpectedHash is the expected SHA256 hash for integrity verification
	ExpectedHash string

	// LocalPath is the local save path
	LocalPath string

	// TempPath is the temporary download path, moved to LocalPath after download
	TempPath string

	// MinSize is the minimum file size in bytes, for basic validity checks
	MinSize int64

	// Executable indicates if the file is executable, sets execute permissions if true
	Executable bool
}

// NewRepository creates a new repository manager instance
func NewRepository(cfg *system.SystemConfig, log Logger) *Repository {
	// Configure HTTP client for optimized download performance and reliability
	client := &http.Client{
		Timeout: 300 * time.Second, // 5-minute timeout for large file downloads
		Transport: &http.Transport{
			MaxIdleConns:       10,               // Maximum idle connections
			IdleConnTimeout:    90 * time.Second, // Idle connection timeout
			DisableCompression: true,             // Disable compression to save CPU
		},
	}

	return &Repository{
		config:    cfg,
		logger:    log,
		validator: NewValidator(log),
		client:    client,
	}
}

// DownloadAll downloads all GWD components
// This is the Go implementation of the original bash script's repoDL function
// Includes the complete process of fetching hashes, checking local files, downloading updates, etc.
func (r *Repository) DownloadAll() error {
	r.logger.Progress("Checking repository files")

	// Ensure repository directory exists
	repoDir := r.config.GetRepoDir()
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return errors.Wrapf(err, "Failed to create repository directory: %s", repoDir)
	}

	// First, fetch SHA256 hashes for all files
	// These hashes are stored in the GitHub repository for verifying downloaded file integrity
	hashes, err := r.fetchHashValues()
	if err != nil {
		return errors.Wrap(err, "Failed to fetch file hash values")
	}

	// Build the list of download targets
	targets := r.buildDownloadTargets(hashes)

	// Check and download files one by one
	for _, target := range targets {
		if err := r.downloadIfNeeded(target); err != nil {
			return errors.Wrapf(err, "Failed to download file: %s", target.Name)
		}
	}

	r.logger.ProgressDone("Repository")
	return nil
}

// fetchHashValues fetches SHA256 hash values for all files from GitHub
// These hash files are stored in the repository in the format "hash filename"
func (r *Repository) fetchHashValues() (map[string]string, error) {
	hashes := make(map[string]string)

	// Define all files for which hash values need to be fetched
	// Each .sha256sum file contains the hash value for the corresponding component
	hashFiles := map[string]string{
		"doh":       fmt.Sprintf("doh/%s.sha256sum", r.dohFileName()),
		"nginx":     fmt.Sprintf("nginx/%s.sha256sum", r.nginxFileName()),
		"nginxConf": "nginx/nginxConf.zip.sha256sum",
		"sample":    "sample.zip.sha256sum",
		"smartdns":  fmt.Sprintf("smartdns/%s.sha256sum", r.smartdnsFileName()),
		"tcsss":     fmt.Sprintf("tcsss/%s.sha256sum", r.tcsssFileName()),
		"vtrui":     fmt.Sprintf("vtrui/%s.sha256sum", r.vtruiFileName()),
	}

	// Concurrently fetch all hash values to improve download efficiency
	for name, hashFile := range hashFiles {
		url := fmt.Sprintf("%s/%s", archiveBaseURL, hashFile)
		hash, err := r.fetchHash(url)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to fetch hash for %s", name)
		}

		hashes[name] = hash
	}

	return hashes, nil
}

// fetchHash fetches a single hash value from a URL
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

	// Hash file format is usually "hash filename", we only need the hash part
	hash := strings.Fields(string(body))[0]
	return strings.TrimSpace(hash), nil
}

// buildDownloadTargets builds the list of download targets
// Creates information for all files that need to be downloaded based on system architecture and configuration
func (r *Repository) buildDownloadTargets(hashes map[string]string) []DownloadTarget {
	repoDir := r.config.GetRepoDir()

	return []DownloadTarget{
		{
			Name:         "DoH Server",
			URL:          r.archiveURL("doh", r.dohFileName()),
			ExpectedHash: hashes["doh"],
			LocalPath:    filepath.Join(repoDir, "doh-server"),
			TempPath:     r.config.GetTempFilePath("doh-server.tmp"),
			MinSize:      1024 * 1024, // 1MB, minimum reasonable size for DoH server binary
			Executable:   true,
		},
		{
			Name:         "Nginx",
			URL:          r.archiveURL("nginx", r.nginxFileName()),
			ExpectedHash: hashes["nginx"],
			LocalPath:    filepath.Join(repoDir, "nginx"),
			TempPath:     r.config.GetTempFilePath("nginx.tmp"),
			MinSize:      1024 * 1024 * 2, // 2MB, minimum reasonable size for Nginx binary
			Executable:   true,
		},
		{
			Name:         "SmartDNS",
			URL:          r.archiveURL("smartdns", r.smartdnsFileName()),
			ExpectedHash: hashes["smartdns"],
			LocalPath:    filepath.Join(repoDir, "smartdns"),
			TempPath:     r.config.GetTempFilePath("smartdns.tmp"),
			MinSize:      512 * 1024, // 512KB, minimum reasonable size for SmartDNS binary
			Executable:   true,
		},
		{
			Name:         "TCSSS",
			URL:          r.archiveURL("tcsss", r.tcsssFileName()),
			ExpectedHash: hashes["tcsss"],
			LocalPath:    filepath.Join(repoDir, "tcsss"),
			TempPath:     r.config.GetTempFilePath("tcsss.tmp"),
			MinSize:      512 * 1024, // 512KB, minimum reasonable size for tcsss binary
			Executable:   true,
		},
		{
			Name:         "vtrui",
			URL:          r.archiveURL("vtrui", r.vtruiFileName()),
			ExpectedHash: hashes["vtrui"],
			LocalPath:    filepath.Join(repoDir, "vtrui"),
			TempPath:     r.config.GetTempFilePath("vtrui.tmp"),
			MinSize:      512 * 1024, // 512KB, minimum reasonable size for vtrui binary
			Executable:   true,
		},
		{
			Name:         "Nginx Configuration",
			URL:          r.archiveURL("nginx", "nginxConf.zip"),
			ExpectedHash: hashes["nginxConf"],
			LocalPath:    filepath.Join(repoDir, "nginxConf.zip"),
			TempPath:     r.config.GetTempFilePath("nginxConf.zip.tmp"),
			MinSize:      1024 * 10, // 10KB, minimum reasonable size for configuration file
			Executable:   false,
		},
		{
			Name:         "Sample Configuration",
			URL:          r.archiveURL("sample.zip"),
			ExpectedHash: hashes["sample"],
			LocalPath:    filepath.Join(repoDir, "sample.zip"),
			TempPath:     r.config.GetTempFilePath("sample.zip.tmp"),
			MinSize:      1024 * 5, // 5KB, minimum reasonable size for sample configuration
			Executable:   false,
		},
	}
}

// downloadIfNeeded checks if a file needs to be downloaded, and downloads it if necessary
// This implements an incremental update mechanism, only downloading changed files
func (r *Repository) downloadIfNeeded(target DownloadTarget) error {
	// Check if local file exists and hash is correct
	needsDownload := true
	if _, err := os.Stat(target.LocalPath); err == nil {
		// File exists, check hash
		valid, err := r.validator.ValidateFile(target.LocalPath, target.ExpectedHash)
		if err != nil {
			r.logger.Warn("Failed to validate local file %s: %v", target.Name, err)
		} else if valid {
			// Hash matches, skip download
			needsDownload = false
		}
	}

	if !needsDownload {
		r.logger.Debug("%s is already the latest version, skipping download", target.Name)
		return nil
	}

	r.logger.Info("Downloading %s...", target.Name)

	// Clean up temporary file
	os.Remove(target.TempPath)

	// Perform download
	if err := r.downloadFile(target.URL, target.TempPath); err != nil {
		return errors.Wrapf(err, "Download failed: %s", target.Name)
	}

	// Validate downloaded file
	if err := r.validator.VerifyDownload(target.TempPath, target.ExpectedHash, target.MinSize); err != nil {
		// Delete corrupted file
		os.Remove(target.TempPath)
		return errors.Wrapf(err, "File validation failed: %s", target.Name)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(filepath.Dir(target.LocalPath), 0755); err != nil {
		return errors.Wrapf(err, "Failed to create directory: %s", filepath.Dir(target.LocalPath))
	}

	// Move file to final location
	if err := os.Rename(target.TempPath, target.LocalPath); err != nil {
		return errors.Wrapf(err, "Failed to move file: %s -> %s", target.TempPath, target.LocalPath)
	}

	// Set executable permissions (if required)
	if target.Executable {
		if err := os.Chmod(target.LocalPath, 0755); err != nil {
			return errors.Wrapf(err, "Failed to set execute permissions: %s", target.LocalPath)
		}
	}

	r.logger.Success("%s downloaded successfully", target.Name)
	return nil
}

// downloadFile downloads a single file
// Implements file download with progress display and retry mechanism
func (r *Repository) downloadFile(url, localPath string) error {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			r.logger.Info("Retrying download (attempt %d/%d): %s", attempt, maxRetries, url)
			time.Sleep(time.Duration(attempt) * time.Second) // Incremental delay
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

// doDownload performs the actual file download operation
func (r *Repository) doDownload(url, localPath string) error {
	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return errors.Wrap(err, "Failed to create download request")
	}

	// Set User-Agent to simulate wget behavior
	req.Header.Set("User-Agent", "GWD/1.0 (Go downloader)")

	// Send request
	resp, err := r.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "Download request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("Download failed, HTTP status code: %d", resp.StatusCode)
	}

	// Create temporary file
	file, err := os.Create(localPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to create temporary file: %s", localPath)
	}
	defer file.Close()

	// Stream file content
	// Use io.Copy for efficient data transfer
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return errors.Wrap(err, "Failed to write file")
	}

	return nil
}

// GetDownloadProgress retrieves download progress information
// Used to provide user feedback during long downloads
type DownloadProgress struct {
	CurrentFile string // Current file being downloaded
	TotalFiles  int    // Total number of files
	CurrentNum  int    // Current file number
	BytesTotal  int64  // Total bytes
	BytesDone   int64  // Bytes downloaded
}

func (r *Repository) archiveURL(parts ...string) string {
	joined := path.Join(parts...)
	joined = strings.TrimPrefix(joined, "/")
	return fmt.Sprintf("%s/%s", archiveBaseURL, joined)
}

func (r *Repository) dohFileName() string {
	if r.config.Architecture == "arm64" {
		return "doh_s_arm64"
	}
	return "doh_s"
}

func (r *Repository) nginxFileName() string {
	if r.config.Architecture == "arm64" {
		return "nginx_arm64"
	}
	return "nginx"
}

func (r *Repository) smartdnsFileName() string {
	if r.config.Architecture == "arm64" {
		return "smartdns-aarch64"
	}
	return "smartdns-x86_64"
}

func (r *Repository) tcsssFileName() string {
	if r.config.Architecture == "arm64" {
		return "tcsss-arm64"
	}
	return "tcsss"
}

func (r *Repository) vtruiFileName() string {
	if r.config.Architecture == "arm64" {
		return "vtrui_arm64"
	}
	return "vtrui"
}

// downloadFileWithProgress downloads a file with progress display
func (r *Repository) downloadFileWithProgress(url, localPath string, progressCallback func(progress DownloadProgress)) error {
	// This method can implement more complex progress display features in the future
	// The current version uses a simple start/completion status display
	return r.downloadFile(url, localPath)
}
