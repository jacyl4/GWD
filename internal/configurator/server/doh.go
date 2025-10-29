package server

import (
	"os"

	"github.com/pkg/errors"
)

const dohConfigDir = "/opt/GWD/doh"

func EnsureDoHConfig() error {
	if err := os.MkdirAll(dohConfigDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create DoH configuration directory %s", dohConfigDir)
	}

	if err := os.WriteFile("/opt/GWD/doh/doh-server.conf", []byte(dohConfigContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write DoH configuration file /opt/GWD/doh/doh-server.conf")
	}

	return nil
}

const dohConfigContent = `listen = [ "127.0.0.1:9853" ]
path = "/dq"
upstream = [
  "udp:127.0.0.1:53",
  "tcp:127.0.0.1:53"
]
timeout = 10
tries = 3
verbose = false
log_guessed_client_ip = false
ecs_allow_non_global_ip = false
ecs_use_precise_ip = false
`
