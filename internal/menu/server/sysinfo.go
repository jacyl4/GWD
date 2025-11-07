package menu

import (
	"bufio"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ui "GWD/internal/ui/server"

	"GWD/internal/system"
)

// SystemProbe abstracts system status collection for the menu.
type SystemProbe interface {
	ServiceStatus(name string) ui.ServiceStatus
	SSLExpireDate() string
	DebianVersion() string
	KernelVersion() string
	IsWireGuardEnabled() bool
	IsHAProxyEnabled() bool
}

type execSystemProbe struct {
	certPath     string
	servicePaths map[string][]string
}

func newExecSystemProbe(cfg *system.Config) SystemProbe {
	workingDir := "/opt/GWD"
	if cfg != nil && cfg.WorkingDir != "" {
		workingDir = cfg.WorkingDir
	}

	servicePaths := map[string][]string{
		"nginx":      {"/usr/sbin/nginx"},
		"vtrui":      {filepath.Join(workingDir, "vtrui", "vtrui")},
		"doh-server": {filepath.Join(workingDir, "doh-server")},
	}

	return &execSystemProbe{
		certPath:     "/var/www/ssl/GWD.cer",
		servicePaths: servicePaths,
	}
}

func (p *execSystemProbe) ServiceStatus(name string) ui.ServiceStatus {
	cmd := exec.Command("systemctl", "is-active", name)
	output, err := cmd.Output()
	if err == nil {
		return statusFromString(strings.TrimSpace(string(output)))
	}

	if p.isServiceInstalled(name) {
		return ui.StatusInactive
	}

	return ui.StatusNotInstalled
}

func (p *execSystemProbe) SSLExpireDate() string {
	data, err := os.ReadFile(p.certPath)
	if errors.Is(err, os.ErrNotExist) {
		return "Not installed"
	}
	if err != nil {
		return "Failed to read certificate"
	}

	cert, parseErr := parseCertificate(data)
	if parseErr != nil {
		return "Failed to parse certificate"
	}

	return cert.NotAfter.Local().Format(time.RFC1123)
}

func parseCertificate(data []byte) (*x509.Certificate, error) {
	if block, _ := pem.Decode(data); block != nil {
		return x509.ParseCertificate(block.Bytes)
	}
	return x509.ParseCertificate(data)
}

func (p *execSystemProbe) DebianVersion() string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "Unknown"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var version string
	var codename string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "VERSION_ID=") {
			version = trimQuotes(strings.TrimPrefix(line, "VERSION_ID="))
		}
		if strings.HasPrefix(line, "VERSION_CODENAME=") {
			codename = trimQuotes(strings.TrimPrefix(line, "VERSION_CODENAME="))
		}
	}

	if codename != "" {
		return codename
	}
	if version != "" {
		return version
	}
	return "Unknown"
}

func (p *execSystemProbe) KernelVersion() string {
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(output))
}

func (p *execSystemProbe) IsWireGuardEnabled() bool {
	return p.ServiceStatus("wg-quick@wgcf") == ui.StatusActive
}

func (p *execSystemProbe) IsHAProxyEnabled() bool {
	return p.ServiceStatus("haproxy") == ui.StatusActive
}

func (p *execSystemProbe) isServiceInstalled(name string) bool {
	if paths, ok := p.servicePaths[name]; ok {
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				return true
			}
		}
	}

	if binary, err := exec.LookPath(name); err == nil && binary != "" {
		return true
	}

	return false
}

func statusFromString(status string) ui.ServiceStatus {
	switch status {
	case "active":
		return ui.StatusActive
	case "inactive":
		return ui.StatusInactive
	case "not-installed":
		return ui.StatusNotInstalled
	case "disabled":
		return ui.StatusDisabled
	default:
		return ui.StatusUnknown
	}
}

func trimQuotes(input string) string {
	return strings.Trim(input, "\"")
}
