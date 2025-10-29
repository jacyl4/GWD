package server

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

//go:embed templates_vtrui/config.json
var vtruiConfigTemplate []byte

//go:embed templates_vtrui/inbound.json
var vtruiInboundTemplate []byte

//go:embed templates_vtrui/outbound.json
var vtruiOutboundTemplate []byte

var vtruiTemplates = map[string][]byte{
	"config.json":   vtruiConfigTemplate,
	"inbound.json":  vtruiInboundTemplate,
	"outbound.json": vtruiOutboundTemplate,
}

const vtruiConfigDir = "/opt/GWD/vtrui"

// EnsureVtruiConfig creates vtrui configuration directory and files atomically
func EnsureVtruiConfig() error {
	// Create configuration directory
	if err := os.MkdirAll(vtruiConfigDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create vtrui configuration directory %s", vtruiConfigDir)
	}

	// Define configuration files to write
	configFiles := []string{
		"config.json",
		"inbound.json",
		"outbound.json",
	}

	for _, filename := range configFiles {
		content, ok := vtruiTemplates[filename]
		if !ok {
			return errors.Errorf("missing embedded template %s", filename)
		}

		targetPath := filepath.Join(vtruiConfigDir, filename)

		// Write to temporary file first for atomic operation
		tmpPath := targetPath + ".tmp"
		if err := os.WriteFile(tmpPath, content, 0644); err != nil {
			return errors.Wrapf(err, "failed to write temporary file: %s", tmpPath)
		}

		// Rename to final location (atomic operation)
		if err := os.Rename(tmpPath, targetPath); err != nil {
			os.Remove(tmpPath)
			return errors.Wrapf(err, "failed to move file to final location: %s", targetPath)
		}
	}

	return nil
}
