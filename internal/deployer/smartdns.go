package deployer

import (
	"os"
	"path/filepath"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

// SmartDNS handles deployment of the SmartDNS service binary, configuration, and service unit.
type SmartDNS struct {
	repoDir string
	logger  *logger.ColoredLogger
}

// NewSmartDNS creates a new SmartDNS component helper.
func NewSmartDNS(repoDir string, log *logger.ColoredLogger) *SmartDNS {
	return &SmartDNS{
		repoDir: repoDir,
		logger:  log,
	}
}

// Install deploys the SmartDNS binary, configuration, and systemd unit.
func (s *SmartDNS) Install() error {
	s.logger.Info("Deploying SmartDNS service...")

	if err := s.installBinary(); err != nil {
		return errors.Wrap(err, "failed to deploy SmartDNS binary")
	}

	if err := s.writeServiceUnit(); err != nil {
		return errors.Wrap(err, "failed to install SmartDNS service unit")
	}

	s.logger.Success("SmartDNS deployment completed")
	return nil
}

func (s *SmartDNS) installBinary() error {
	source := filepath.Join(s.repoDir, "smartdns")
	target := "/usr/local/bin/smartdns"

	if err := copyFile(source, target); err != nil {
		return errors.Wrapf(err, "failed to copy SmartDNS binary from %s to %s", source, target)
	}

	if err := os.Chmod(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to set execute permissions on %s", target)
	}

	return nil
}

func (s *SmartDNS) writeServiceUnit() error {
	servicePath := "/etc/systemd/system/smartdns.service"
	if err := os.WriteFile(servicePath, []byte(smartDNSServiceContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write SmartDNS service file %s", servicePath)
	}

	return nil
}

const smartDNSServiceContent = `[Unit]
Description=SmartDNS Server
After=network-online.target nss-lookup.target
Wants=network-online.target nss-lookup.target

[Service]
Type=forking
RuntimeDirectory=smartdns
PIDFile=/run/smartdns/smartdns.pid
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE
Nice=-10

EnvironmentFile=/opt/GWD/smartdns
ExecStart=/usr/local/bin/smartdns -p /run/smartdns/smartdns.pid -c /opt/GWD/smartdns/smartdns.conf
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=2
RestartPreventExitStatus=23
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`
