package server

import (
	"strconv"

	apperrors "GWD/internal/errors"
	menu "GWD/internal/menu/server"
)

// InstallConfigFromDomainInfo converts menu input into installer configuration.
func InstallConfigFromDomainInfo(info *menu.DomainInfo) (*InstallConfig, error) {
	if info == nil {
		return nil, apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"domain information is required",
			nil,
		)
	}

	port, err := strconv.Atoi(info.Port)
	if err != nil {
		return nil, apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"port must be numeric",
			err,
			apperrors.WithMetadata(apperrors.Metadata{"port": info.Port}),
		)
	}

	cfg := &InstallConfig{
		Domain: info.Domain,
		Port:   port,
	}

	tlsCfg := &TLSConfig{}
	if port == 443 {
		tlsCfg.Provider = TLSProviderLetsEncrypt
	} else {
		tlsCfg.Provider = TLSProviderCloudflare
		if info.CloudflareConfig == nil {
			return nil, apperrors.New(
				apperrors.ErrCategoryValidation,
				apperrors.CodeValidationGeneric,
				"Cloudflare credentials are required for non-standard ports",
				nil,
			)
		}
		tlsCfg.APIKey = info.CloudflareConfig.APIKey
		tlsCfg.Email = info.CloudflareConfig.Email
	}

	cfg.TLS = tlsCfg

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
