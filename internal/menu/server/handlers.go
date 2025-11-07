package menu

import "github.com/pkg/errors"

func (m *Menu) handleInstallGWD() error {
	m.logger.Info("Starting GWD server installation...")

	domain, err := m.promptDomain()
	if err != nil {
		return errors.Wrap(err, "failed to get domain")
	}

	domainInfo := m.parseDomainInput(domain)

	if domainInfo.Port != "443" {
		cf, err := m.promptCloudflareConfig()
		if err != nil {
			return errors.Wrap(err, "failed to get Cloudflare configuration")
		}
		domainInfo.CloudflareConfig = cf
	}

	m.logger.Info("Domain: %s, Port: %s", domainInfo.Domain, domainInfo.Port)

	if m.installHandler == nil {
		return errors.New("installer handler is not configured")
	}

	if err := m.installHandler(domainInfo); err != nil {
		return errors.Wrap(err, "GWD installation failed")
	}

	m.waitForUserInput("\nPress Enter to continue...")

	return nil
}
