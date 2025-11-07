package server

import (
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

const rngToolsDefaultPath = "/etc/default/rng-tools-debian"

const rngToolsDefaultContent = `# -*- mode: sh -*-

#-

# Configuration for the rng-tools-debian initscript



# Set to the input source for random data, leave undefined
# for the initscript to attempt auto-detection.  Set to /dev/null
# for the viapadlock driver.
#HRNGDEVICE=/dev/hwrng
#HRNGDEVICE=/dev/null
HRNGDEVICE=/dev/urandom



# Additional options to send to rngd. See the rngd(8) manpage for
# more information.  Do not specify -r/--rng-device here, use
# HRNGDEVICE for that instead.
#RNGDOPTIONS="--hrng=intelfwh --fill-watermark=90% --feed-interval=1"
#RNGDOPTIONS="--hrng=viakernel --fill-watermark=90% --feed-interval=1"
#RNGDOPTIONS="--hrng=viapadlock --fill-watermark=90% --feed-interval=1"
# For TPM (also add tpm-rng to /etc/initramfs-tools/modules or /etc/modules):
#RNGDOPTIONS="--fill-watermark=90% --feed-interval=1"



# If you need to configure which RNG to use, do it here:
#HRNGSELECT="virtio_rng.0"
# Use this instead of sysfsutils, which starts too late.
`

const chronyConfPath = "/etc/chrony/chrony.conf"

const chronyConfContent = `server time.cloudflare.com iburst
server time1.google.com iburst
server time1.apple.com iburst
server ntp-3.arkena.net iburst


driftfile /var/lib/chrony/chrony.drift
logdir /var/log/chrony
maxupdateskew 100.0
rtcsync
makestep 1 3
leapsectz right/UTC
`

var (
	rngToolsServiceCandidates = []string{"rng-tools", "rng-tools-debian"}
	chronyServiceCandidates   = []string{"chrony"}
)

// EnsureRngToolsConfigured writes rng-tools default configuration and restarts/enables the service
func EnsureRngToolsConfigured() error {
	if err := os.WriteFile(rngToolsDefaultPath, []byte(rngToolsDefaultContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write rng-tools defaults %s", rngToolsDefaultPath)
	}

	if err := restartService(rngToolsServiceCandidates...); err != nil {
		return err
	}
	if err := enableService(rngToolsServiceCandidates...); err != nil {
		return err
	}
	return nil
}

// EnsureChronyConfigured writes chrony configuration and restarts/enables the service
func EnsureChronyConfigured() error {
	if err := os.WriteFile(chronyConfPath, []byte(chronyConfContent), 0644); err != nil {
		return errors.Wrapf(err, "failed to write chrony configuration %s", chronyConfPath)
	}

	if err := restartService(chronyServiceCandidates...); err != nil {
		return err
	}
	if err := enableService(chronyServiceCandidates...); err != nil {
		return err
	}
	return nil
}

// EnsureEntropyAndTimeConfigured applies both rng-tools and chrony configurations
func EnsureEntropyAndTimeConfigured() error {
	if err := EnsureRngToolsConfigured(); err != nil {
		return err
	}
	if err := EnsureChronyConfigured(); err != nil {
		return err
	}
	return nil
}

// EnsureTimezoneShanghai sets system timezone to Asia/Shanghai via timedatectl
func EnsureTimezoneShanghai() error {
	if err := exec.Command("timedatectl", "set-timezone", "Asia/Shanghai").Run(); err != nil {
		return errors.Wrap(err, "failed to set timezone to Asia/Shanghai")
	}
	return nil
}

func restartService(candidates ...string) error {
	var lastErr error
	for _, name := range candidates {
		if err := exec.Command("systemctl", "restart", name).Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		return errors.New("no service candidates provided for restart")
	}
	return errors.Wrapf(lastErr, "failed to restart services %v", candidates)
}

func enableService(candidates ...string) error {
	var lastErr error
	for _, name := range candidates {
		if err := exec.Command("systemctl", "enable", name).Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		return errors.New("no service candidates provided for enable")
	}
	return errors.Wrapf(lastErr, "failed to enable services %v", candidates)
}
