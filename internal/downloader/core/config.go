package core

import (
	"embed"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const defaultArchiveBaseURL = "https://raw.githubusercontent.com/jacyl4/GWD"

// DownloadConfig describes download behaviour and available components.
type DownloadConfig struct {
	BaseURL    string            `yaml:"base_url"`
	Branch     string            `yaml:"branch"`
	Timeout    time.Duration     `yaml:"timeout"`
	MaxRetries int               `yaml:"max_retries"`
	Components []ComponentConfig `yaml:"components"`
}

// ComponentConfig describes a downloadable component.
type ComponentConfig struct {
	Name          string            `yaml:"name"`
	DisplayName   string            `yaml:"display_name"`
	Paths         map[string]string `yaml:"paths"`
	MinSize       int64             `yaml:"min_size"`
	Executable    bool              `yaml:"executable"`
	SupportResume bool              `yaml:"support_resume"`
}

// PathForArch returns the archive path for the requested architecture.
func (c ComponentConfig) PathForArch(arch string) (string, bool) {
	if c.Paths == nil {
		return "", false
	}

	if path, ok := c.Paths[arch]; ok {
		return strings.TrimSpace(path), true
	}

	if path, ok := c.Paths["all"]; ok {
		return strings.TrimSpace(path), true
	}

	if path, ok := c.Paths["default"]; ok {
		return strings.TrimSpace(path), true
	}

	return "", false
}

//go:embed base-config.yaml
var embeddedBaseConfig embed.FS

// BaseConfig returns the embedded base download configuration.
func BaseConfig() (*DownloadConfig, error) {
	data, err := embeddedBaseConfig.ReadFile("base-config.yaml")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read embedded base config")
	}
	return decodeConfig(data)
}

// DefaultConfig returns the embedded base configuration for backward compatibility.
func DefaultConfig() (*DownloadConfig, error) {
	return BaseConfig()
}

// LoadConfig loads a configuration file from disk.
func LoadConfig(path string) (*DownloadConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file: %s", path)
	}
	return decodeConfig(data)
}

// ParseConfig decodes configuration data from bytes.
func ParseConfig(data []byte) (*DownloadConfig, error) {
	if len(data) == 0 {
		return &DownloadConfig{}, nil
	}
	return decodeConfig(data)
}

// MergeConfigs merges multiple configurations together, later entries overriding earlier ones.
func MergeConfigs(cfgs ...*DownloadConfig) (*DownloadConfig, error) {
	if len(cfgs) == 0 {
		return nil, errors.New("no configurations provided")
	}

	var result DownloadConfig
	componentIndex := make(map[string]int)

	for i, cfg := range cfgs {
		if cfg == nil {
			continue
		}

		if i == 0 {
			result = *cfg
			result.Components = append([]ComponentConfig(nil), cfg.Components...)
			for idx, comp := range result.Components {
				componentIndex[comp.Name] = idx
			}
			continue
		}

		if trimmed := strings.TrimSpace(cfg.BaseURL); trimmed != "" {
			result.BaseURL = trimmed
		}
		if trimmed := strings.TrimSpace(cfg.Branch); trimmed != "" {
			result.Branch = trimmed
		}
		if cfg.Timeout > 0 {
			result.Timeout = cfg.Timeout
		}
		if cfg.MaxRetries > 0 {
			result.MaxRetries = cfg.MaxRetries
		}

		for _, comp := range cfg.Components {
			if comp.Name == "" {
				continue
			}
			if idx, ok := componentIndex[comp.Name]; ok {
				result.Components[idx] = comp
			} else {
				componentIndex[comp.Name] = len(result.Components)
				result.Components = append(result.Components, comp)
			}
		}
	}

	if strings.TrimSpace(result.BaseURL) == "" {
		result.BaseURL = defaultArchiveBaseURL
	}
	if strings.TrimSpace(result.Branch) == "" {
		result.Branch = "main"
	}
	if result.Timeout == 0 {
		result.Timeout = 300 * time.Second
	}
	if result.MaxRetries <= 0 {
		result.MaxRetries = 3
	}

	return &result, nil
}

func decodeConfig(data []byte) (*DownloadConfig, error) {
	var cfg DownloadConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrap(err, "failed to parse download configuration")
	}
	return &cfg, nil
}
