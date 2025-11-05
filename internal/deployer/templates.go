package deployer

import (
	apperrors "GWD/internal/errors"
	"bytes"
	"embed"
	"path/filepath"
	"text/template"
)

//go:embed systemd/*
var systemdFS embed.FS

func renderTemplate(name string, data any) (string, error) {
	if name == "" {
		return "", newDeployerError("deployer.renderTemplate", "template name cannot be empty", nil, nil)
	}

	content, err := loadTemplate(name)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(name).Parse(content)
	if err != nil {
		return "", newDeployerError("deployer.renderTemplate", "failed to parse template", err, apperrors.Metadata{
			"template": name,
		})
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", newDeployerError("deployer.renderTemplate", "failed to execute template", err, apperrors.Metadata{
			"template": name,
		})
	}

	return buf.String(), nil
}

func loadTemplate(name string) (string, error) {
	path := filepath.Join("systemd", name)

	data, err := systemdFS.ReadFile(path)
	if err != nil {
		return "", newDeployerError("deployer.loadTemplate", "failed to load template", err, apperrors.Metadata{
			"template": name,
		})
	}

	return string(data), nil
}
