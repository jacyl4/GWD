package server

import (
	"strings"

	apperrors "GWD/internal/errors"
)

// TLSProvider represents the supported TLS automation providers.
type TLSProvider string

const (
	// TLSProviderLetsEncrypt indicates Let's Encrypt automation.
	TLSProviderLetsEncrypt TLSProvider = "letsencrypt"
	// TLSProviderCloudflare indicates Cloudflare SSL automation.
	TLSProviderCloudflare TLSProvider = "cloudflare"
)

// TLSConfig captures certificate automation configuration.
type TLSConfig struct {
	Provider TLSProvider
	APIKey   string
	Email    string
}

// InstallConfig stores the domain level installation inputs.
type InstallConfig struct {
	Domain string
	Port   int
	TLS    *TLSConfig
}

// Validate performs basic domain and TLS validation.
func (cfg *InstallConfig) Validate() error {
	if cfg == nil {
		return apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"install configuration is required",
			nil,
		)
	}

	cfg.Domain = strings.TrimSpace(cfg.Domain)
	if cfg.Domain == "" {
		return apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"domain is required",
			nil,
		)
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		return apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"port must be between 1 and 65535",
			nil,
			apperrors.WithMetadata(apperrors.Metadata{"port": cfg.Port}),
		)
	}

	if cfg.TLS == nil {
		return apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"TLS configuration is required",
			nil,
		)
	}

	switch cfg.TLS.Provider {
	case TLSProviderLetsEncrypt:
		// No additional fields required today.
	case TLSProviderCloudflare:
		if strings.TrimSpace(cfg.TLS.APIKey) == "" {
			return apperrors.New(
				apperrors.ErrCategoryValidation,
				apperrors.CodeValidationGeneric,
				"Cloudflare API key is required",
				nil,
			)
		}
		if strings.TrimSpace(cfg.TLS.Email) == "" {
			return apperrors.New(
				apperrors.ErrCategoryValidation,
				apperrors.CodeValidationGeneric,
				"Cloudflare account email is required",
				nil,
			)
		}
	default:
		return apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"unsupported TLS provider",
			nil,
			apperrors.WithMetadata(apperrors.Metadata{"provider": cfg.TLS.Provider}),
		)
	}

	return nil
}
