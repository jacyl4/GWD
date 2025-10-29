package server

import (
	"archive/zip"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const nginxConfZipPath = "/opt/GWD/.repo/nginxConf.zip"
const nginxConfigDir = "/etc/nginx"
const nginxTempDir = "/tmp/nginx-config-tmp"

// createNginxDirectories creates all necessary directories for nginx operation
func createNginxDirectories() error {
	directories := []struct {
		path  string
		perms os.FileMode
	}{
		{"/var/www/html", 0755},
		{"/var/www/ssl", 0755},
		{"/etc/nginx", 0755},
		{"/etc/nginx/conf.d", 0755},
		{"/etc/nginx/stream.d", 0755},
		{"/var/log/nginx", 0755},
		{"/var/cache/nginx/client_temp", 0755},
		{"/var/cache/nginx/proxy_temp", 0755},
		{"/var/cache/nginx/fastcgi_temp", 0755},
		{"/var/cache/nginx/scgi_temp", 0755},
		{"/var/cache/nginx/uwsgi_temp", 0755},
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir.path, dir.perms); err != nil {
			return errors.Wrapf(err, "failed to create directory: %s", dir.path)
		}
	}

	return nil
}

// EnsureNginxConfig extracts the nginxConf.zip file and places contents to /etc/nginx
// This operation is atomic and complete - extraction happens to a temp directory first,
// then moved to the final location
func EnsureNginxConfig() error {
	// Create all required nginx directories first
	if err := createNginxDirectories(); err != nil {
		return errors.Wrap(err, "failed to create nginx directories")
	}

	// Check if zip file exists
	if _, err := os.Stat(nginxConfZipPath); os.IsNotExist(err) {
		return errors.Wrapf(err, "nginx configuration zip file not found: %s", nginxConfZipPath)
	}

	// Clean up temp directory if it exists
	if err := os.RemoveAll(nginxTempDir); err != nil {
		return errors.Wrapf(err, "failed to clean up temporary directory: %s", nginxTempDir)
	}

	// Create temp directory for extraction
	if err := os.MkdirAll(nginxTempDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create temporary directory: %s", nginxTempDir)
	}

	// Extract zip file to temp directory
	if err := extractZip(nginxConfZipPath, nginxTempDir); err != nil {
		os.RemoveAll(nginxTempDir)
		return errors.Wrapf(err, "failed to extract nginx configuration zip file")
	}

	// Ensure target directory exists
	if err := os.MkdirAll(nginxConfigDir, 0755); err != nil {
		os.RemoveAll(nginxTempDir)
		return errors.Wrapf(err, "failed to create nginx configuration directory: %s", nginxConfigDir)
	}

	// Copy extracted files to /etc/nginx atomically
	if err := copyDirectoryContents(nginxTempDir, nginxConfigDir); err != nil {
		os.RemoveAll(nginxTempDir)
		return errors.Wrapf(err, "failed to copy nginx configuration files")
	}

	// Clean up temp directory
	if err := os.RemoveAll(nginxTempDir); err != nil {
		return errors.Wrapf(err, "failed to clean up temporary directory")
	}

	return nil
}

// extractZip extracts a zip file to the target directory
func extractZip(zipPath, targetDir string) error {
	// Open the zip file
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open zip file: %s", zipPath)
	}
	defer r.Close()

	// Extract each file
	for _, f := range r.File {
		if err := extractFile(f, targetDir); err != nil {
			return errors.Wrapf(err, "failed to extract file: %s", f.Name)
		}
	}

	return nil
}

// extractFile extracts a single file from the zip archive
func extractFile(f *zip.File, targetDir string) error {
	// Build the target path
	targetPath := filepath.Join(targetDir, f.Name)

	// Check for path traversal attacks (zip slip vulnerability)
	if !filepath.IsAbs(targetDir) {
		targetDir, _ = filepath.Abs(targetDir)
	}
	extractPath, _ := filepath.Abs(targetPath)
	if len(extractPath) < len(targetDir) || extractPath[:len(targetDir)] != targetDir {
		return errors.Errorf("invalid file path in zip: %s", f.Name)
	}

	// Create directories if needed
	if f.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, f.Mode())
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory: %s", filepath.Dir(targetPath))
	}

	// Open the file in the zip
	rc, err := f.Open()
	if err != nil {
		return errors.Wrapf(err, "failed to open file in zip: %s", f.Name)
	}
	defer rc.Close()

	// Create the target file
	outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return errors.Wrapf(err, "failed to create target file: %s", targetPath)
	}
	defer outFile.Close()

	// Copy file contents
	_, err = outFile.ReadFrom(rc)
	if err != nil {
		return errors.Wrapf(err, "failed to write file contents: %s", targetPath)
	}

	return nil
}

// copyDirectoryContents copies all files and directories from src to dst
func copyDirectoryContents(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate relative path: %s", path)
		}

		// Build destination path
		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Handle files - copy with proper permissions
		return copyFile(path, dstPath, info.Mode())
	})
}

// copyFile copies a file from src to dst with the specified permissions
func copyFile(src, dst string, mode os.FileMode) error {
	// Read source file
	data, err := os.ReadFile(src)
	if err != nil {
		return errors.Wrapf(err, "failed to read source file: %s", src)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return errors.Wrapf(err, "failed to create destination directory: %s", filepath.Dir(dst))
	}

	// Write to temporary file first for atomic operation
	tmpDst := dst + ".tmp"
	if err := os.WriteFile(tmpDst, data, mode); err != nil {
		return errors.Wrapf(err, "failed to write temporary file: %s", tmpDst)
	}

	// Rename to final location (atomic operation)
	if err := os.Rename(tmpDst, dst); err != nil {
		os.Remove(tmpDst)
		return errors.Wrapf(err, "failed to move file to final location: %s", dst)
	}

	return nil
}

