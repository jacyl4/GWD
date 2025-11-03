package nftables

import (
	"net"
	"os"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

func selectFlowtableDevices(cfg *Config, auto []string) []string {
	if len(cfg.FlowtableDeviceExplicit) > 0 {
		return uniqueStrings(cfg.FlowtableDeviceExplicit)
	}

	deviceSet := stringSet(auto)
	for _, dev := range cfg.FlowtableDeviceInclude {
		deviceSet[dev] = struct{}{}
	}
	for _, dev := range cfg.FlowtableDeviceExclude {
		delete(deviceSet, dev)
	}

	for name := range deviceSet {
		if isExcludedByPrefix(name, cfg.FlowtableDeviceExcludePrefixes) {
			delete(deviceSet, name)
		}
	}
	return sortedKeys(deviceSet)
}

func detectFlowtableDevices(cfg *Config) ([]string, []string, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read /sys/class/net")
	}

	include := make(map[string]struct{}, len(cfg.FlowtableDeviceInclude)+len(cfg.FlowtableDeviceExplicit))
	for _, name := range cfg.FlowtableDeviceInclude {
		if name != "" {
			include[name] = struct{}{}
		}
	}
	for _, name := range cfg.FlowtableDeviceExplicit {
		if name != "" {
			include[name] = struct{}{}
		}
	}

	excludeExact := make(map[string]struct{}, len(cfg.FlowtableDeviceExclude))
	for _, name := range cfg.FlowtableDeviceExclude {
		if name != "" {
			excludeExact[name] = struct{}{}
		}
	}

	var auto []string
	bypass := make(map[string]struct{})

	for _, entry := range entries {
		name := entry.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}

		if _, forced := include[name]; forced {
			auto = append(auto, name)
			continue
		}

		if _, excluded := excludeExact[name]; excluded {
			bypass[name] = struct{}{}
			continue
		}

		excludedByPrefix := false
		for _, prefix := range cfg.FlowtableDeviceExcludePrefixes {
			if prefix == "" {
				continue
			}
			if strings.HasPrefix(name, prefix) {
				excludedByPrefix = true
				break
			}
		}
		if excludedByPrefix {
			bypass[name] = struct{}{}
			continue
		}

		auto = append(auto, name)
	}

	for name := range excludeExact {
		if _, forced := include[name]; forced {
			continue
		}
		bypass[name] = struct{}{}
	}

	bypassList := make([]string, 0, len(bypass))
	for name := range bypass {
		bypassList = append(bypassList, name)
	}
	sort.Strings(bypassList)

	return uniqueStrings(auto), bypassList, nil
}

func detectDefaultLanCIDRs(cfg *Config) ([]string, error) {
	if len(cfg.LanCIDRs) > 0 {
		return uniqueStrings(cfg.LanCIDRs), nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.Wrap(err, "failed to enumerate interfaces")
	}

	explicit := stringSet(cfg.FlowtableDeviceExplicit)
	include := stringSet(cfg.FlowtableDeviceInclude)
	excludeExact := stringSet(cfg.FlowtableDeviceExclude)

	cidrs := make(map[string]struct{})

	for _, iface := range ifaces {
		name := iface.Name
		if name == "" {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		allowed := false

		if len(explicit) > 0 {
			_, allowed = explicit[name]
		} else {
			if _, forced := include[name]; forced {
				allowed = true
			} else {
				if _, excluded := excludeExact[name]; excluded {
					continue
				}
				if isExcludedByPrefix(name, cfg.FlowtableDeviceExcludePrefixes) {
					continue
				}
				allowed = iface.Flags&net.FlagUp != 0
			}
		}

		if !allowed {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet == nil {
				continue
			}

			if ipNet.IP.To4() == nil {
				continue
			}

			cidrs[ipNet.String()] = struct{}{}
		}
	}

	if len(cidrs) == 0 {
		for _, cidr := range defaultLanCIDRs {
			cidrs[cidr] = struct{}{}
		}
	}

	return sortedKeys(cidrs), nil
}
