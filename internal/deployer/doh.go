package deployer

import (
	"os"
	"path/filepath"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

// DoH handles deployment of the DNS-over-HTTPS server.
type DoH struct {
	repoDir string
	logger  *logger.ColoredLogger
}

// NewDoH creates a new DoH component helper.
func NewDoH(repoDir string, log *logger.ColoredLogger) *DoH {
	return &DoH{
		repoDir: repoDir,
		logger:  log,
	}
}

// Install deploys the DoH binary, configuration, and systemd service.
func (d *DoH) Install() error {
	d.logger.Info("Deploying DoH server...")

	if err := d.installBinary(); err != nil {
		return errors.Wrap(err, "failed to deploy DoH binary")
	}


	if err := d.writeServiceUnit(); err != nil {
		return errors.Wrap(err, "failed to install DoH service unit")
	}

	d.logger.Success("DoH server deployment completed")
	return nil
}

func (d *DoH) installBinary() error {
	source := filepath.Join(d.repoDir, "doh-server")
	target := "/usr/local/bin/doh-server"

	if err := copyFile(source, target); err != nil {
		return errors.Wrapf(err, "failed to copy DoH binary from %s to %s", source, target)
	}

	if err := os.Chmod(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to set execute permissions on %s", target)
	}

	return nil
}

func (d *DoH) writeServiceUnit() error {
	servicePath := "/etc/systemd/system/doh-server.service"
	if err := os.WriteFile(servicePath, []byte(dohServiceContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write DoH service file %s", servicePath)
	}

	return nil
}

const dohServiceContent = `[Unit]
Description=DNS-over-HTTPS server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
Nice=-8

ExecStart=/usr/local/bin/doh-server -conf %s
Restart=on-failure
RestartSec=2
RestartPreventExitStatus=23
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`
