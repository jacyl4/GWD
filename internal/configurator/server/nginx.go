package server

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	apperrors "GWD/internal/errors"
)

const (
	nginxConfigDir  = "/etc/nginx"
	nginxConfDDir   = "/etc/nginx/conf.d"
	nginxStreamDir  = "/etc/nginx/stream.d"
	sslDirectory    = "/var/www/ssl"
	defaultCertPath = "/var/www/ssl/de_GWD.cer"
	defaultKeyPath  = "/var/www/ssl/de_GWD.key"
	defaultDHParam  = "/var/www/ssl/dhparam.pem"
	defaultWSPath   = "/ws"
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

	if err := createNginxDirectories(); err != nil {
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
			path:      filepath.Join(nginxConfDDir, "80.conf"),
			template:  nginxRedirectTemplate,
			condition: opts.Port == 443,
		},
		{
			name:      "HSTS headers",
			path:      filepath.Join(nginxConfDDir, ".HSTS"),
			template:  nginxHSTSTemplate,
			condition: true,
		},
		{
			name:      "SSL certificates",
			path:      filepath.Join(nginxConfDDir, ".ssl_certs"),
			template:  nginxSSLCertsTemplate,
			condition: true,
		},
		{
			name:      "default server",
			path:      filepath.Join(nginxConfDDir, "default.conf"),
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

	if strings.TrimSpace(opts.WSPath) == "" {
		opts.WSPath = defaultWSPath
	}

	if opts.CertFile == "" {
		opts.CertFile = defaultCertPath
	}

	if opts.KeyFile == "" {
		opts.KeyFile = defaultKeyPath
	}

	if opts.DHParamFile == "" {
		opts.DHParamFile = defaultDHParam
	}

	return nil
}

func createNginxDirectories() error {
	directories := []string{
		"/var/www/html",
		sslDirectory,
		nginxConfigDir,
		nginxConfDDir,
		nginxStreamDir,
		"/var/log/nginx",
		"/var/cache/nginx/client_temp",
		"/var/cache/nginx/proxy_temp",
		"/var/cache/nginx/fastcgi_temp",
		"/var/cache/nginx/scgi_temp",
		"/var/cache/nginx/uwsgi_temp",
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return newConfiguratorError(
				"configurator.createNginxDirectories",
				"failed to create nginx directory",
				err,
				apperrors.Metadata{"path": dir},
			)
		}
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
