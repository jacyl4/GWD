package server

import (
	"os"

	"github.com/pkg/errors"
)

const smartDNSConfigDir = "/opt/GWD/smartdns"

const smartDNSConfigContent = `bind 127.0.0.1:53
bind-tcp 127.0.0.1:53

server 1.1.1.1
server 1.0.0.1
server 8.8.8.8
server 8.8.4.4
server 9.9.9.9
server 208.67.222.222

speed-check-mode ping,tcp:80,tcp:443
response-mode first-ping
cache-size 2048
cache-persist yes
cache-file /opt/GWD/smartdns/smartdns.cache
cache-checkpoint-time 28800
prefetch-domain yes
serve-expired-prefetch-time 21600
serve-expired yes
serve-expired-ttl 3600
serve-expired-reply-ttl 3
rr-ttl 600
rr-ttl-min 60
rr-ttl-max 3600
local-ttl 60
max-reply-ip-num 16
log-level warn
tcp-idle 120

force-AAAA-SOA yes
dualstack-ip-selection no
`

func EnsureSmartDNSConfig() error {
	if err := os.MkdirAll(smartDNSConfigDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create SmartDNS configuration directory %s", smartDNSConfigDir)
	}

	if err := os.WriteFile("/opt/GWD/smartdns/smartdns.conf", []byte(smartDNSConfigContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write SmartDNS configuration file /opt/GWD/smartdns/smartdns.conf")
	}

	return nil
}

