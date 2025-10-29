package deployer

import (
	"os"
	"path/filepath"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

const (
	vtruiConfigDir = "/opt/GWD/vtrui"
)

// Vtrui handles deployment of the vtrui service binary and systemd unit.
type Vtrui struct {
	repoDir string
	logger  *logger.ColoredLogger
}

// NewVtrui creates a new Vtrui component helper.
func NewVtrui(repoDir string, log *logger.ColoredLogger) *Vtrui {
	return &Vtrui{
		repoDir: repoDir,
		logger:  log,
	}
}

// Install deploys the vtrui binary, prepares configuration directory, and installs the service unit.
func (v *Vtrui) Install() error {
	v.logger.Info("Deploying vtrui service...")

	if err := v.installBinary(); err != nil {
		return errors.Wrap(err, "failed to deploy vtrui binary")
	}

	if err := v.writeServiceUnit(); err != nil {
		return errors.Wrap(err, "failed to install vtrui service unit")
	}

	v.logger.Success("vtrui deployment completed")
	return nil
}

func (v *Vtrui) installBinary() error {
	source := filepath.Join(v.repoDir, "vtrui")
	target := "/usr/local/bin/vtrui"

	if err := copyFile(source, target); err != nil {
		return errors.Wrapf(err, "failed to copy vtrui binary from %s to %s", source, target)
	}

	if err := os.Chmod(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to set execute permissions on %s", target)
	}

	return nil
}

func (v *Vtrui) writeServiceUnit() error {
	servicePath := "/etc/systemd/system/vtrui.service"
	if err := os.WriteFile(servicePath, []byte(vtruiServiceContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write vtrui service file %s", servicePath)
	}

	return nil
}

const vtruiServiceContent = `[Unit]
Description=vtrui
After=network.target nss-lookup.target

[Service]
User=www-data
ProtectSystem=strict
PrivateTmp=yes
NoNewPrivileges=yes
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN CAP_NET_BIND_SERVICE
Nice=-8

ExecStart=/usr/local/bin/vtrui run -confdir /opt/GWD/vtrui
Restart=on-failure
RestartSec=2
RestartPreventExitStatus=23
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`
