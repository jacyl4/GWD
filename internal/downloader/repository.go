package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"GWD/internal/logger"
	"GWD/internal/system"

	"github.com/pkg/errors"
)

// Repository is the repository download manager
// Manages the entire lifecycle of downloading files from a GitHub repository
type Repository struct {
	config    *system.SystemConfig
	logger    *logger.ColoredLogger
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
func NewRepository(cfg *system.SystemConfig, log *logger.ColoredLogger) *Repository {
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

	// Check and download the version file (special handling)
	if err := r.updateVersionFile(); err != nil {
		return errors.Wrap(err, "Failed to update version file")
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
		"GWD":        fmt.Sprintf("GWD_%s.zip.sha256sum", r.config.Architecture),
		"doh_server": fmt.Sprintf("doh/doh_s_%s.sha256sum", r.config.Architecture),
		"nginx":      fmt.Sprintf("nginx/nginx_%s.sha256sum", r.config.Architecture),
		"nginxConf":  "nginx/nginxConf.zip.sha256sum",
		"sample":     "server/sample.zip.sha256sum",
	}

	// Concurrently fetch all hash values to improve download efficiency
	for name, hashFile := range hashFiles {
		url := fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/resource/%s",
			r.config.Branch, hashFile)

		// If it's the main GWD component, the URL path is slightly different
		if name == "GWD" {
			url = fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/%s",
				r.config.Branch, hashFile)
		}

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
			URL:          fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/resource/doh/doh_s_%s", r.config.Branch, r.config.Architecture),
			ExpectedHash: hashes["doh_server"],
			LocalPath:    "/opt/GWD/doh-server",
			TempPath:     filepath.Join(r.config.TmpDir, "doh-server"),
			MinSize:      1024 * 1024, // 1MB, minimum reasonable size for DoH server binary
			Executable:   true,
		},
		{
			Name:         "Nginx",
			URL:          fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/resource/nginx/nginx_%s", r.config.Branch, r.config.Architecture),
			ExpectedHash: hashes["nginx"],
			LocalPath:    "/usr/sbin/nginx",
			TempPath:     filepath.Join(r.config.TmpDir, "nginx"),
			MinSize:      1024 * 1024 * 2, // 2MB, minimum reasonable size for Nginx binary
			Executable:   true,
		},
		{
			Name:         "GWD Main Package",
			URL:          fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/GWD_%s.zip", r.config.Branch, r.config.Architecture),
			ExpectedHash: hashes["GWD"],
			LocalPath:    filepath.Join(repoDir, "GWD.zip"),
			TempPath:     filepath.Join(r.config.TmpDir, "GWD.zip"),
			MinSize:      1024 * 1024 * 5, // 5MB, minimum reasonable size for main package
			Executable:   false,
		},
		{
			Name:         "Nginx Configuration",
			URL:          fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/resource/nginx/nginxConf.zip", r.config.Branch),
			ExpectedHash: hashes["nginxConf"],
			LocalPath:    filepath.Join(repoDir, "nginxConf.zip"),
			TempPath:     filepath.Join(r.config.TmpDir, "nginxConf.zip"),
			MinSize:      1024 * 10, // 10KB, minimum reasonable size for configuration file
			Executable:   false,
		},
		{
			Name:         "Sample Configuration",
			URL:          fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/resource/server/sample.zip", r.config.Branch),
			ExpectedHash: hashes["sample"],
			LocalPath:    filepath.Join(repoDir, "sample.zip"),
			TempPath:     filepath.Join(r.config.TmpDir, "sample.zip"),
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

// updateVersionFile updates the version file
// The version file is used to track the currently installed GWD version, supporting auto-update functionality
func (r *Repository) updateVersionFile() error {
	versionPath := filepath.Join(r.config.WorkingDir, "version.php")

	// Get local version (if exists)
	localVersion, _ := r.getLocalVersion(versionPath)

	// Get remote version
	remoteVersion, err := r.getRemoteVersion()
	if err != nil {
		return errors.Wrap(err, "Failed to get remote version")
	}

	// Compare versions, update if different
	if localVersion != remoteVersion {
		r.logger.Info("Version update detected: %s -> %s", localVersion, remoteVersion)

		tempPath := filepath.Join(r.config.TmpDir, "version.php")
		url := fmt.Sprintf("https://raw.githubusercontent.com/jacyl4/GWD/%s/version.php",
			r.config.Branch)

		// Download new version file
		if err := r.downloadFile(url, tempPath); err != nil {
			return errors.Wrap(err, "Failed to download version file")
		}

		// Validate file size (version file should be small but not empty)
		if valid, _ := r.validator.ValidateFileSize(tempPath, 4*1024); !valid {
			os.Remove(tempPath)
			return errors.New("Version file size abnormal")
		}

		// Move to final location
		if err := os.Rename(tempPath, versionPath); err != nil {
			return errors.Wrap(err, "Failed to move version file")
		}

		r.logger.Success("Version file updated successfully")
	}

	return nil
}

// getLocalVersion reads the first line of the local version file
// The first line of the version file contains the current version number
func (r *Repository) getLocalVersion(versionPath string) (string, error) {
	content, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return "", errors.New("Version file is empty")
	}

	return strings.TrimSpace(lines[0]), nil
}

// getRemoteVersion fetches version information from the remote repository
func (r *Repository) getRemoteVersion() (string, error) {
	url := "https://raw.githubusercontent.com/jacyl4/de_GWD/main/version.php"

	resp, err := r.client.Get(url)
	if err != nil {
		return "", errors.Wrap(err, "Failed to request remote version")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Failed to get remote version, status code: %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "Failed to read remote version content")
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return "", errors.New("Remote version file is empty")
	}

	return strings.TrimSpace(lines[0]), nil
}

// CheckForUpdates checks for available updates
// Compares local and remote versions, returns whether an update is needed
func (r *Repository) CheckForUpdates() (bool, string, string, error) {
	versionPath := filepath.Join(r.config.WorkingDir, "version.php")

	localVersion, err := r.getLocalVersion(versionPath)
	if err != nil {
		return false, "", "", errors.Wrap(err, "Failed to get local version")
	}

	remoteVersion, err := r.getRemoteVersion()
	if err != nil {
		return false, "", "", errors.Wrap(err, "Failed to get remote version")
	}

	needsUpdate := localVersion != remoteVersion
	return needsUpdate, localVersion, remoteVersion, nil
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

// downloadFileWithProgress downloads a file with progress display
func (r *Repository) downloadFileWithProgress(url, localPath string, progressCallback func(progress DownloadProgress)) error {
	// This method can implement more complex progress display features in the future
	// The current version uses a simple start/completion status display
	return r.downloadFile(url, localPath)
}
