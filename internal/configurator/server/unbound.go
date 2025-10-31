package server

import (
	"os"

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

so-reuseport: yes
so-rcvbuf: 4m
so-sndbuf: 4m

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

func EnsureUnboundConfig() error {
	if err := os.MkdirAll(unboundConfigDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create Unbound configuration directory %s", unboundConfigDir)
	}

	if err := os.WriteFile("/etc/unbound/unbound.conf", []byte(unboundConfigContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write Unbound configuration file /etc/unbound/unbound.conf")
	}

	return nil
}

