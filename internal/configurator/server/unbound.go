package server

import (
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

const unboundConfigDir = "/etc/unbound"

const unboundConfigContent = `server:
  verbosity: 0
  interface: 127.0.0.1
  port: 53

  do-ip4: yes
  do-udp: yes
  do-tcp: yes
  do-ip6: no
  prefer-ip6: no
  edns-buffer-size: 1232

  prefetch: yes
  module-config: "validator iterator"

  so-reuseport: yes
  so-rcvbuf: 512k
  so-sndbuf: 512k

  num-threads: 2
  msg-cache-slabs: 4
  rrset-cache-slabs: 4
  infra-cache-slabs: 4
  key-cache-slabs: 4

forward-zone:
  name: "."
  forward-addr: 1.1.1.1
  forward-addr: 1.0.0.1
  forward-addr: 8.8.8.8
  forward-addr: 8.8.4.4
  forward-addr: 9.9.9.9
  forward-addr: 208.67.222.222
  forward-first: no
`

const unboundServiceContent = `[Unit]
Description=Unbound DNS server
After=network.target

[Service]
Type=simple
Nice=-5
ReadOnlyPaths=/etc/unbound
ExecStart=/usr/sbin/unbound -d
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=2
RestartPreventExitStatus=23
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
`

func EnsureUnboundConfig() error {
    if err := os.MkdirAll(unboundConfigDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create Unbound configuration directory %s", unboundConfigDir)
	}

    if err := os.WriteFile("/etc/unbound/unbound.conf", []byte(unboundConfigContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write Unbound configuration file /etc/unbound/unbound.conf")
	}

    if err := writeUnboundServiceUnit(); err != nil {
        return err
    }

    return nil
}

func writeUnboundServiceUnit() error {
	servicePath := "/etc/systemd/system/unbound.service"
    if err := os.WriteFile(servicePath, []byte(unboundServiceContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write unbound service file %s", servicePath)
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return errors.Wrap(err, "failed to daemon-reload for unbound service")
	}
	if err := exec.Command("systemctl", "enable", "unbound").Run(); err != nil {
		return errors.Wrap(err, "failed to enable unbound service")
	}
	if err := exec.Command("systemctl", "restart", "unbound").Run(); err != nil {
		return errors.Wrap(err, "failed to restart unbound service")
	}
    return nil
}
