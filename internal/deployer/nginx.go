package deployer

import (
	"os"
	"path/filepath"

	"GWD/internal/logger"

	"github.com/pkg/errors"
)

// Nginx handles deployment of the Nginx binary and systemd units.
type Nginx struct {
	repoDir string
	logger  *logger.ColoredLogger
}

// NewNginx creates a new Nginx component helper.
func NewNginx(repoDir string, log *logger.ColoredLogger) *Nginx {
	return &Nginx{
		repoDir: repoDir,
		logger:  log,
	}
}

// Install deploys the Nginx binary and installs systemd service files.
func (n *Nginx) Install() error {
	n.logger.Info("Deploying Nginx server...")

	if err := n.installBinary(); err != nil {
		return errors.Wrap(err, "failed to deploy Nginx binary")
	}

	if err := n.writeServiceUnit(); err != nil {
		return errors.Wrap(err, "failed to install Nginx service unit")
	}

	if err := n.writeServiceOverride(); err != nil {
		return errors.Wrap(err, "failed to install Nginx service override")
	}

	n.logger.Success("Nginx deployment completed")
	return nil
}

func (n *Nginx) installBinary() error {
	source := filepath.Join(n.repoDir, "nginx")
	target := "/usr/local/bin/nginx"

	if err := copyFile(source, target); err != nil {
		return errors.Wrapf(err, "failed to copy Nginx binary from %s to %s", source, target)
	}

	if err := os.Chmod(target, 0755); err != nil {
		return errors.Wrapf(err, "failed to set execute permissions on %s", target)
	}

	return nil
}

func (n *Nginx) writeServiceUnit() error {
	servicePath := "/etc/systemd/system/nginx.service"
	if err := os.WriteFile(servicePath, []byte(nginxServiceContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write Nginx service file %s", servicePath)
	}

	return nil
}

func (n *Nginx) writeServiceOverride() error {
	overrideDir := "/etc/systemd/system/nginx.service.d"
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create Nginx override directory %s", overrideDir)
	}

	overridePath := filepath.Join(overrideDir, "override.conf")
	if err := os.WriteFile(overridePath, []byte(nginxOverrideContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write Nginx override file %s", overridePath)
	}

	return nil
}

const nginxServiceContent = `[Unit]
Description=NGINX
After=network.target

[Service]
Type=forking
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
Nice=-9

PIDFile=/run/nginx.pid
ExecStart=/usr/local/bin/nginx -c /etc/nginx/nginx.conf
ExecReload=/usr/local/bin/nginx -s reload
ExecStop=/bin/kill -s QUIT $MAINPID
KillMode=mixed
Restart=on-failure
RestartSec=2
RestartPreventExitStatus=23
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`

const nginxOverrideContent = `[Service]
ExecStartPost=/bin/sleep 0.1
`
