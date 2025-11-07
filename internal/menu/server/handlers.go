package menu

import (
	apperrors "GWD/internal/errors"
)

func (m *Menu) handleInstallGWD() error {
	m.logger.Info("Starting GWD server installation...")

	domain, err := m.promptDomain()
	if err != nil {
		return apperrors.New(
			apperrors.ErrCategoryValidation,
			apperrors.CodeValidationGeneric,
			"failed to capture domain input",
			err,
		).
			WithModule("menu").
			WithOperation("menu.handleInstallGWD")
	}

	domainInfo := m.parseDomainInput(domain)

	if domainInfo.Port != "443" {
		cf, err := m.promptCloudflareConfig()
		if err != nil {
			return apperrors.New(
				apperrors.ErrCategoryValidation,
				apperrors.CodeValidationGeneric,
				"failed to capture Cloudflare configuration",
				err,
			).
				WithModule("menu").
				WithOperation("menu.handleInstallGWD")
		}
		domainInfo.CloudflareConfig = cf
	}

	m.logger.Info("Domain: %s, Port: %s", domainInfo.Domain, domainInfo.Port)

	if m.installHandler == nil {
		return apperrors.New(
			apperrors.ErrCategoryConfig,
			apperrors.CodeConfigGeneric,
			"installer handler is not configured",
			nil,
		).
			WithModule("menu").
			WithOperation("menu.handleInstallGWD")
	}

	if err := m.installHandler(domainInfo); err != nil {
		return apperrors.New(
			apperrors.ErrCategoryDeployment,
			apperrors.CodeDeploymentGeneric,
			"GWD installation failed",
			err,
		).
			WithModule("menu").
			WithOperation("menu.handleInstallGWD")
	}

	m.waitForUserInput("\nPress Enter to continue...")

	return nil
}
