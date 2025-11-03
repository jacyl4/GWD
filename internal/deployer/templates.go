package deployer

import (
	"bytes"
	"embed"
	"path/filepath"
	"text/template"

	"github.com/pkg/errors"
)

//go:embed systemd/*
var systemdFS embed.FS

func renderTemplate(name string, data any) (string, error) {
	if name == "" {
		return "", errors.New("template name cannot be empty")
	}

	content, err := loadTemplate(name)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New(name).Parse(content)
	if err != nil {
		return "", errors.Wrapf(err, "parsing template %s", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", errors.Wrapf(err, "executing template %s", name)
	}

	return buf.String(), nil
}

func loadTemplate(name string) (string, error) {
	path := filepath.Join("systemd", name)

	data, err := systemdFS.ReadFile(path)
	if err != nil {
		return "", errors.Wrapf(err, "loading template %s", name)
	}

	return string(data), nil
}
