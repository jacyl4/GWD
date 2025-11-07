package timesync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperrors "GWD/internal/errors"
	"golang.org/x/sys/unix"
)

var categoryToCode = map[apperrors.ErrorCategory]string{
	apperrors.ErrCategoryNetwork:    apperrors.CodeNetworkGeneric,
	apperrors.ErrCategorySystem:     apperrors.CodeSystemGeneric,
	apperrors.ErrCategoryValidation: apperrors.CodeValidationGeneric,
}

// Options controls how Sync obtains and applies network time.
type Options struct {
	Sources []string
	Timeout time.Duration
}

// Result captures details about the synchronization attempt.
type Result struct {
	Source      string
	NetworkTime time.Time
}

// Sync fetches the current time from the configured HTTP(S) sources,
// updates the system clock, and optionally synchronizes the hardware RTC.
func Sync(ctx context.Context, opts *Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	finalOpts := Options{
		Sources: []string{
			"https://whatismyip.akamai.com/",
			"https://cloudflare.com/",
			"https://time.cloudflare.com/",
		},
		Timeout: 5 * time.Second,
	}

	if opts != nil {
		if len(opts.Sources) > 0 {
			finalOpts.Sources = opts.Sources
		}
		if opts.Timeout > 0 {
			finalOpts.Timeout = opts.Timeout
		}
	}

	httpClient := &http.Client{
		Timeout: finalOpts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Prevent redirects so time is derived from the original source host.
			return http.ErrUseLastResponse
		},
	}

	var failures []string
	for _, source := range finalOpts.Sources {
		reqCtx, cancel := context.WithTimeout(ctx, finalOpts.Timeout)
		networkTime, err := fetchNetworkTime(reqCtx, httpClient, source)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", source, err))
			continue
		}

		if err := setSystemClock(networkTime); err != nil {
			return nil, err
		}

		return &Result{
			Source:      source,
			NetworkTime: networkTime,
		}, nil
	}

	return nil, newTimesyncError(apperrors.ErrCategoryNetwork, "Sync", "failed to fetch network time from sources", nil).
		WithField("failures", strings.Join(failures, "; "))
}

func fetchNetworkTime(ctx context.Context, client *http.Client, source string) (time.Time, error) {
	wrapErr := func(err error) error {
		return newTimesyncError(apperrors.ErrCategoryNetwork, "fetchNetworkTime", "failed to fetch time", err).
			WithField("source", source)
	}

	doRequest := func(method string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, method, source, nil)
		if err != nil {
			return "", fmt.Errorf("build %s request: %w", method, err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("%s request: %w", method, err)
		}
		defer resp.Body.Close()

		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return "", fmt.Errorf("drain %s response: %w", method, err)
		}

		return resp.Header.Get("Date"), nil
	}

	dateHeader, err := doRequest(http.MethodHead)
	if err != nil {
		return time.Time{}, wrapErr(err)
	}

	if dateHeader == "" {
		dateHeader, err = doRequest(http.MethodGet)
		if err != nil {
			return time.Time{}, wrapErr(err)
		}
	}

	if dateHeader == "" {
		return time.Time{}, newTimesyncError(apperrors.ErrCategoryNetwork, "fetchNetworkTime", "no Date header from source", nil).
			WithField("source", source)
	}

	parsed, err := http.ParseTime(dateHeader)
	if err != nil {
		return time.Time{}, newTimesyncError(apperrors.ErrCategoryNetwork, "fetchNetworkTime", "invalid Date header", err).
			WithField("source", source).
			WithField("date_header", dateHeader)
	}

	return parsed.UTC(), nil
}

func setSystemClock(target time.Time) error {
	tv := unix.NsecToTimeval(target.UTC().UnixNano())
	if err := unix.Settimeofday(&tv); err != nil {
		return newTimesyncError(apperrors.ErrCategorySystem, "setSystemClock", "settimeofday failed", err)
	}
	return nil
}

func newTimesyncError(category apperrors.ErrorCategory, operation, message string, err error) *apperrors.AppError {
	code, ok := categoryToCode[category]
	if !ok {
		code = apperrors.CodeSystemGeneric
	}

	return apperrors.New(category, code, message, err).
		WithModule("timesync").
		WithOperation(operation)
}
