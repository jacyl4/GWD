package system

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

var defaultTimeSources = []string{
	"https://whatismyip.akamai.com/",
	"https://cloudflare.com/",
	"https://time.cloudflare.com/",
}

// TimeSyncOptions controls how SyncSystemTime obtains and applies network time.
type TimeSyncOptions struct {
	// Sources is an ordered list of HTTPS endpoints whose Date headers will be
	// used as the authoritative clock. Defaults to defaultTimeSources.
	Sources []string
	// Timeout caps the amount of time spent contacting a single source.
	// Defaults to 5 seconds if zero.
	Timeout time.Duration
	// SkipHardwareClock disables attempts to persist the synchronized time to
	// the hardware clock (RTC).
	SkipHardwareClock bool
}

// TimeSyncResult captures details about the synchronization attempt.
type TimeSyncResult struct {
	Source               string
	NetworkTime          time.Time
	HardwareClockSynced  bool
	HardwareClockSkipped bool
	HardwareClockWarning string
}

// SyncSystemTime fetches the current time from the configured HTTP(S) sources,
// updates the system clock, and (optionally) synchronizes the hardware RTC.
// Sources are attempted in order until one succeeds.
func SyncSystemTime(ctx context.Context, opts *TimeSyncOptions) (*TimeSyncResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	sources := defaultTimeSources
	if opts != nil && len(opts.Sources) > 0 {
		sources = opts.Sources
	}

	timeout := 5 * time.Second
	skipHwClock := false
	if opts != nil {
		if opts.Timeout > 0 {
			timeout = opts.Timeout
		}
		skipHwClock = opts.SkipHardwareClock
	}

	httpClient := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var failures []string
	for _, source := range sources {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		networkTime, err := fetchNetworkTime(reqCtx, httpClient, source)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", source, err))
			continue
		}

		if err := setSystemClock(networkTime); err != nil {
			return nil, errors.Wrap(err, "failed to apply network time to system clock")
		}

		result := &TimeSyncResult{
			Source:      source,
			NetworkTime: networkTime,
		}

		if skipHwClock {
			result.HardwareClockSkipped = true
			return result, nil
		}

		synced, warning, err := syncHardwareClock()
		if err != nil {
			return nil, err
		}
		result.HardwareClockSynced = synced
		if warning != "" {
			result.HardwareClockWarning = warning
			result.HardwareClockSkipped = !synced
		}

		return result, nil
	}

	if len(failures) == 0 {
		return nil, errors.New("no time sources provided")
	}

	return nil, errors.Errorf("failed to fetch network time (%s)", strings.Join(failures, "; "))
}

func fetchNetworkTime(ctx context.Context, client *http.Client, source string) (time.Time, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, source, nil)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "failed to build HEAD request")
	}

	response, err := client.Do(request)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "HEAD request failed")
	}
	io.Copy(io.Discard, response.Body) // ignore body so the transport can reuse the connection
	response.Body.Close()

	dateHeader := response.Header.Get("Date")
	if dateHeader == "" {
		request, err = http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "failed to build GET request")
		}

		response, err = client.Do(request)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "GET request failed")
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		dateHeader = response.Header.Get("Date")
	}

	if dateHeader == "" {
		return time.Time{}, errors.Errorf("source %s did not provide a Date header", source)
	}

	parsed, err := http.ParseTime(dateHeader)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "failed to parse Date header %q", dateHeader)
	}

	return parsed.UTC(), nil
}

func setSystemClock(target time.Time) error {
	tv := unix.NsecToTimeval(target.UTC().UnixNano())
	if err := unix.Settimeofday(&tv); err != nil {
		return errors.Wrap(err, "settimeofday failed")
	}
	return nil
}

func syncHardwareClock() (bool, string, error) {
	canSync, reason, err := canSyncHardwareClock()
	if err != nil {
		return false, "", err
	}
	if !canSync {
		return false, reason, nil
	}

	cmd := exec.Command("hwclock", "--systohc")
	if err := cmd.Run(); err != nil {
		return false, "", errors.Wrap(err, "hwclock --systohc failed")
	}

	return true, "", nil
}

func canSyncHardwareClock() (bool, string, error) {
	if _, err := exec.LookPath("hwclock"); err != nil {
		return false, "hwclock command not available", nil
	}

	virt, err := detectVirtualization()
	if err == nil && virt == "container" {
		return false, "hardware clock not accessible inside containers", nil
	}

	if !rtcDeviceExists() {
		return false, "no RTC device detected", nil
	}

	return true, "", nil
}

func rtcDeviceExists() bool {
	paths := []string{"/dev/rtc", "/dev/rtc0"}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
