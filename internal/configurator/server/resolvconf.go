package server

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	// resolvconf files
	resolvconfHeadFile = "/etc/resolvconf/resolv.conf.d/head"
	resolvconfOriginal = "/etc/resolvconf/resolv.conf.d/original"
	resolvconfBase     = "/etc/resolvconf/resolv.conf.d/base"
	resolvconfTail     = "/etc/resolvconf/resolv.conf.d/tail"

	// resolv.conf targets
	etcResolvConf        = "/etc/resolv.conf"
	systemInterfacesFile = "/etc/network/interfaces"

	// Force local resolver
	resolvconfHeadContent = "nameserver 127.0.0.1\n"
)

// EnsureResolvconfConfig guarantees /etc/resolv.conf resolves via 127.0.0.1.
// Steps: ensure base files -> write head -> strip dns-nameservers from interfaces
// -> try "resolvconf -u" -> on failure, write /etc/resolv.conf directly.
func EnsureResolvconfConfig() error {
	// 1) Ensure resolvconf base files exist (empty)
	if err := ensureEmptyFile(resolvconfOriginal); err != nil {
		return errors.Wrapf(err, "prepare %s", resolvconfOriginal)
	}
	if err := ensureEmptyFile(resolvconfBase); err != nil {
		return errors.Wrapf(err, "prepare %s", resolvconfBase)
	}
	if err := ensureEmptyFile(resolvconfTail); err != nil {
		return errors.Wrapf(err, "prepare %s", resolvconfTail)
	}

	// 2) Write head with local nameserver
	if err := os.WriteFile(resolvconfHeadFile, []byte(resolvconfHeadContent), 0644); err != nil {
		return errors.Wrapf(err, "write %s", resolvconfHeadFile)
	}

	// 3) Remove "dns-nameservers" lines from /etc/network/interfaces (avoid ifupdown injecting DNS)
	if err := stripDnsNameservers(systemInterfacesFile); err != nil {
		return errors.Wrapf(err, "update %s", systemInterfacesFile)
	}

	// 4) Try resolvconf update; if it fails, fallback to writing /etc/resolv.conf directly
	if out, err := exec.Command("resolvconf", "-u").CombinedOutput(); err == nil {
		return nil
	} else {
		// Fallback: make /etc/resolv.conf a plain file with local nameserver
		_ = os.RemoveAll(etcResolvConf) // remove broken symlink if present
		if writeErr := os.WriteFile(etcResolvConf, []byte(resolvconfHeadContent), 0644); writeErr != nil {
			return errors.Wrapf(err, "resolvconf -u failed (%s) and fallback write %s failed", string(out), etcResolvConf)
		}
		return nil
	}
}

// ---- helpers ----

func ensureEmptyFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return errors.Wrapf(err, "mkdir for %s", path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "truncate %s", path)
	}
	return f.Close()
}

// stripDnsNameservers removes lines containing "dns-nameservers " from /etc/network/interfaces,
// preserving all other content and the original trailing newline behavior.
func stripDnsNameservers(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "read %s", path)
	}
	lines := bytes.Split(data, []byte{'\n'})
	hasTrail := len(data) > 0 && data[len(data)-1] == '\n'

	out := lines[:0]
	changed := false
	for i, ln := range lines {
		// Preserve the original trailing empty line if present
		if hasTrail && i == len(lines)-1 && len(ln) == 0 {
			out = append(out, ln)
			continue
		}
		if bytes.Contains(ln, []byte("dns-nameservers ")) {
			changed = true
			continue
		}
		out = append(out, ln)
	}
	if !changed {
		return nil
	}
	dst := bytes.Join(out, []byte{'\n'})
	if hasTrail && (len(dst) == 0 || dst[len(dst)-1] != '\n') {
		dst = append(dst, '\n')
	}
	return os.WriteFile(path, dst, 0644)
}
