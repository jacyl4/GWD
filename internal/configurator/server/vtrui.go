package server

import (
	_ "embed"
	"os"
	"path/filepath"

	apperrors "GWD/internal/errors"
)

//go:embed templates_vtrui/config.json
var vtruiConfigTemplate []byte

//go:embed templates_vtrui/inbound.json
var vtruiInboundTemplate []byte

//go:embed templates_vtrui/outbound.json
var vtruiOutboundTemplate []byte

const vtruiConfigDir = "/opt/GWD/vtrui"

// EnsureVtruiConfig creates vtrui configuration directory and files atomically
func EnsureVtruiConfig() error {
	// Create configuration directory
	if err := os.MkdirAll(vtruiConfigDir, 0o755); err != nil {
		return newConfiguratorError(
			"configurator.EnsureVtruiConfig",
			"failed to create vtrui configuration directory",
			err,
			apperrors.Metadata{"path": vtruiConfigDir},
		)
	}

	files := []struct {
		name    string
		content []byte
	}{
		{"config.json", vtruiConfigTemplate},
		{"inbound.json", vtruiInboundTemplate},
		{"outbound.json", vtruiOutboundTemplate},
	}

	for _, file := range files {
		path := filepath.Join(vtruiConfigDir, file.name)
		if err := os.WriteFile(path, file.content, 0o644); err != nil {
			return newConfiguratorError(
				"configurator.EnsureVtruiConfig",
				"failed to write vtrui configuration file",
				err,
				apperrors.Metadata{
					"path": path,
					"file": file.name,
				},
			)
		}
	}

	return nil
}
