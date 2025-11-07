package menu

import (
	ui "GWD/internal/ui/server"
)

type SystemStatus struct {
	Services         map[string]ui.ServiceStatus
	DebianVersion    string
	KernelVersion    string
	SSLExpireDate    string
	WireGuardEnabled bool
	HAProxyEnabled   bool
}

func (m *Menu) collectSystemStatus() SystemStatus {
	services := map[string]string{
		"Nginx":      "nginx",
		"Xray":       "vtrui",
		"DoH Server": "doh-server",
		"AutoUpdate": "",
	}

	serviceStatuses := make(map[string]ui.ServiceStatus, len(services))
	for displayName, serviceName := range services {
		if serviceName == "" {
			serviceStatuses[displayName] = ui.StatusDisabled
			continue
		}

		serviceStatuses[displayName] = m.sysProbe.ServiceStatus(serviceName)
	}

	return SystemStatus{
		Services:         serviceStatuses,
		DebianVersion:    m.sysProbe.DebianVersion(),
		KernelVersion:    m.sysProbe.KernelVersion(),
		SSLExpireDate:    m.sysProbe.SSLExpireDate(),
		WireGuardEnabled: m.sysProbe.IsWireGuardEnabled(),
		HAProxyEnabled:   m.sysProbe.IsHAProxyEnabled(),
	}
}

func (m *Menu) displaySystemStatus(status SystemStatus) {
	for service, svcStatus := range status.Services {
		m.printer.PrintServiceStatus(service, svcStatus)
	}

	m.printer.PrintSeparator("-", 64)
	m.logger.Info("Debian Version: %s", status.DebianVersion)
	m.logger.Info("Kernel Version: %s", status.KernelVersion)
	m.printer.PrintSeparator("-", 64)
	m.logger.Info("SSL Certificate Expiration: %s", status.SSLExpireDate)

	if status.WireGuardEnabled {
		m.writeLine("🟣 [Enabled] Cloudflare Wireguard Upstream (WARP)")
	}

	if status.HAProxyEnabled {
		m.writeLine("🟣 [Enabled] HAProxy TCP Port Forwarding")
	}
}

func (m *Menu) writeLine(format string, args ...interface{}) {
	if m.console != nil {
		m.console.WriteLine(format, args...)
		return
	}
	m.logger.Info(format, args...)
}
