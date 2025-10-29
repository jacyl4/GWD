package deployer

import (
	"os"
	"path/filepath"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

// Tcsss handles deployment of the Traffic Control Smart Shaping Service binary and systemd unit.
type Tcsss struct {
	repoDir string
	logger  *logger.ColoredLogger
}

// NewTcsss creates a new Tcsss component helper.
func NewTcsss(repoDir string, log *logger.ColoredLogger) *Tcsss {
	return &Tcsss{
		repoDir: repoDir,
		logger:  log,
	}
}

// Install deploys the tcsss binary and installs the systemd service unit.
func (t *Tcsss) Install() error {
	t.logger.Info("Deploying tcsss service...")

	if err := t.installBinary(); err != nil {
		return errors.Wrap(err, "failed to deploy tcsss binary")
	}

	if err := t.writeServiceUnit(); err != nil {
		return errors.Wrap(err, "failed to install tcsss service unit")
	}

	t.logger.Success("tcsss deployment completed")
	return nil
}

func (t *Tcsss) installBinary() error {
	source := filepath.Join(t.repoDir, "tcsss")
	target := "/usr/local/bin/tcsss"

	if err := copyFile(source, target); err != nil {
		return errors.Wrapf(err, "failed to copy tcsss binary from %s to %s", source, target)
	}

	if err := os.Chmod(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to set execute permissions on %s", target)
	}

	return nil
}

func (t *Tcsss) writeServiceUnit() error {
	servicePath := "/etc/systemd/system/tcsss.service"
	if err := os.WriteFile(servicePath, []byte(tcsssServiceContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write tcsss service file %s", servicePath)
	}

	return nil
}

const tcsssServiceContent = `[Unit]
Description=Traffic Control Smart Shaping Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_SYS_ADMIN
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_SYS_ADMIN
NoNewPrivileges=true

RuntimeDirectory=tcsss
StateDirectory=tcsss
LogsDirectory=tcsss
StandardOutput=journal
StandardError=journal

ExecStart=/usr/local/bin/tcsss s
ExecStop=/bin/kill -s SIGTERM $MAINPID
Restart=on-failure
RestartSec=2
RestartPreventExitStatus=23
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`
