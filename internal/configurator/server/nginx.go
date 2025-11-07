package server

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	apperrors "GWD/internal/errors"
)

const (
	nginxConfigRoot   = "/etc/nginx"
	nginxConfigZipSrc = "/opt/GWD/.repo/nginxConf.zip"
)

//go:embed templates_nginx/redirect.conf.tmpl
var nginxRedirectTemplate string

//go:embed templates_nginx/hsts.conf
var nginxHSTSTemplate string

//go:embed templates_nginx/ssl_certs.conf.tmpl
var nginxSSLCertsTemplate string

//go:embed templates_nginx/default.conf.tmpl
var nginxDefaultTemplate string

// NginxOptions contains configuration parameters for Nginx.
type NginxOptions struct {
	Port        int
	Domain      string
	ConfigDir   string
	WSPath      string
	CertFile    string
	KeyFile     string
	DHParamFile string
}

// EnsureNginxConfig generates and writes Nginx configuration files.
func EnsureNginxConfig(opts NginxOptions) error {
	if err := validateNginxOptions(&opts); err != nil {
		return err
	}

	if err := extractNginxConfigArchive(); err != nil {
		return err
	}

	configs := []struct {
		name      string
		path      string
		template  string
		condition bool
	}{
		{
			name:      "HTTP redirect",
			path:      filepath.Join(opts.ConfigDir, "80.conf"),
			template:  nginxRedirectTemplate,
			condition: opts.Port == 443,
		},
		{
			name:      "HSTS headers",
			path:      filepath.Join(opts.ConfigDir, ".HSTS"),
			template:  nginxHSTSTemplate,
			condition: true,
		},
		{
			name:      "SSL certificates",
			path:      filepath.Join(opts.ConfigDir, ".ssl_certs"),
			template:  nginxSSLCertsTemplate,
			condition: true,
		},
		{
			name:      "default server",
			path:      filepath.Join(opts.ConfigDir, "default.conf"),
			template:  nginxDefaultTemplate,
			condition: true,
		},
	}

	for _, cfg := range configs {
		if !cfg.condition {
			if cfg.name == "HTTP redirect" {
				_ = os.Remove(cfg.path)
			}
			continue
		}

		content, err := renderNginxTemplate(cfg.template, opts)
		if err != nil {
			return newConfiguratorError(
				"configurator.EnsureNginxConfig",
				"failed to render nginx template",
				err,
				apperrors.Metadata{
					"config": cfg.name,
					"path":   cfg.path,
				},
			)
		}

		if err := os.WriteFile(cfg.path, []byte(content), 0o644); err != nil {
			return newConfiguratorError(
				"configurator.EnsureNginxConfig",
				"failed to write nginx configuration file",
				err,
				apperrors.Metadata{
					"config": cfg.name,
					"path":   cfg.path,
				},
			)
		}
	}

	return nil
}

func validateNginxOptions(opts *NginxOptions) error {
	if opts.Port < 1 || opts.Port > 65535 {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"invalid port number",
			nil,
			apperrors.Metadata{"port": opts.Port},
		)
	}

	if strings.TrimSpace(opts.Domain) == "" {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"domain is required",
			nil,
			nil,
		)
	}

	opts.WSPath = strings.TrimSpace(opts.WSPath)
	if opts.WSPath == "" {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"websocket path is required",
			nil,
			nil,
		)
	}

	opts.ConfigDir = strings.TrimSpace(opts.ConfigDir)
	if opts.ConfigDir == "" {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"configuration directory is required",
			nil,
			nil,
		)
	}

	opts.CertFile = strings.TrimSpace(opts.CertFile)
	if opts.CertFile == "" {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"certificate file path is required",
			nil,
			nil,
		)
	}

	opts.KeyFile = strings.TrimSpace(opts.KeyFile)
	if opts.KeyFile == "" {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"key file path is required",
			nil,
			nil,
		)
	}

	opts.DHParamFile = strings.TrimSpace(opts.DHParamFile)
	if opts.DHParamFile == "" {
		return newConfiguratorError(
			"configurator.validateNginxOptions",
			"DH parameters file path is required",
			nil,
			nil,
		)
	}

	return nil
}

func renderNginxTemplate(tmpl string, data NginxOptions) (string, error) {
	t, err := template.New("nginx").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func extractNginxConfigArchive() error {
	reader, err := zip.OpenReader(nginxConfigZipSrc)
	if err != nil {
		return newConfiguratorError(
			"configurator.extractNginxConfigArchive",
			"failed to open nginx configuration archive",
			err,
			apperrors.Metadata{"archive": nginxConfigZipSrc},
		)
	}
	defer reader.Close()

	root := filepath.Clean(nginxConfigRoot) + string(os.PathSeparator)

	for _, file := range reader.File {
		targetPath := filepath.Join(nginxConfigRoot, file.Name)
		cleanTarget := filepath.Clean(targetPath)
		if !strings.HasPrefix(cleanTarget, root) {
			return newConfiguratorError(
				"configurator.extractNginxConfigArchive",
				"archive entry escapes nginx directory",
				nil,
				apperrors.Metadata{"entry": file.Name},
			)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, 0o755); err != nil {
				return newConfiguratorError(
					"configurator.extractNginxConfigArchive",
					"failed to create nginx configuration directory",
					err,
					apperrors.Metadata{"path": cleanTarget},
				)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return newConfiguratorError(
				"configurator.extractNginxConfigArchive",
				"failed to create parent directory for nginx config file",
				err,
				apperrors.Metadata{"path": filepath.Dir(cleanTarget)},
			)
		}

		srcFile, err := file.Open()
		if err != nil {
			return newConfiguratorError(
				"configurator.extractNginxConfigArchive",
				"failed to open file from nginx configuration archive",
				err,
				apperrors.Metadata{"entry": file.Name},
			)
		}

		targetFile, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = srcFile.Close()
			return newConfiguratorError(
				"configurator.extractNginxConfigArchive",
				"failed to create nginx configuration file",
				err,
				apperrors.Metadata{"path": cleanTarget},
			)
		}

		if _, err := io.Copy(targetFile, srcFile); err != nil {
			_ = srcFile.Close()
			_ = targetFile.Close()
			return newConfiguratorError(
				"configurator.extractNginxConfigArchive",
				"failed to copy nginx configuration file",
				err,
				apperrors.Metadata{"path": cleanTarget},
			)
		}

		_ = srcFile.Close()
		if err := targetFile.Close(); err != nil {
			return newConfiguratorError(
				"configurator.extractNginxConfigArchive",
				"failed to finalize nginx configuration file",
				err,
				apperrors.Metadata{"path": cleanTarget},
			)
		}
	}

	return nil
}
