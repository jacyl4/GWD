package nftables

import (
	"net"
	"os"
	"strings"

	apperrors "GWD/internal/errors"
)

func selectFlowtableDevices(cfg *Config, auto []string) []string {
	if len(cfg.FlowtableDeviceExplicit) > 0 {
		return uniqueStrings(cfg.FlowtableDeviceExplicit)
	}

	deviceSet := stringSet(auto)
	for _, dev := range cfg.FlowtableDeviceInclude {
		if dev == "" {
			continue
		}
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
		return nil, nil, firewallError("nftables.detectFlowtableDevices", "failed to read network devices", err, apperrors.Metadata{
			"path": "/sys/class/net",
		})
	}

	filter := newDeviceFilter(cfg)
	auto := make(map[string]struct{})
	bypass := make(map[string]struct{})

	for _, entry := range entries {
		name := entry.Name()
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}

		if filter.isForced(name) {
			auto[name] = struct{}{}
			continue
		}

		if filter.isExcluded(name) {
			if filter.shouldBypass(name) {
				bypass[name] = struct{}{}
			}
			continue
		}

		if filter.allow(name, nil) {
			auto[name] = struct{}{}
		}
	}

	for name := range filter.exclude {
		if filter.isForced(name) {
			continue
		}
		bypass[name] = struct{}{}
	}

	autoList := sortedKeys(auto)
	if autoList == nil {
		autoList = []string{}
	}
	bypassList := sortedKeys(bypass)
	if bypassList == nil {
		bypassList = []string{}
	}

	return autoList, bypassList, nil
}

func detectDefaultLanCIDRs(cfg *Config) ([]string, error) {
	if len(cfg.LanCIDRs) > 0 {
		return uniqueStrings(cfg.LanCIDRs), nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, firewallError("nftables.detectDefaultLanCIDRs", "failed to enumerate interfaces", err, nil)
	}

	filter := newDeviceFilter(cfg)
	cidrs := make(map[string]struct{})

	for _, iface := range ifaces {
		name := iface.Name
		if name == "" || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		allowed := filter.allow(name, func() bool {
			return iface.Flags&net.FlagUp != 0
		})
		if !allowed {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet == nil || ipNet.IP.To4() == nil {
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

	result := sortedKeys(cidrs)
	if result == nil {
		return []string{}, nil
	}
	return result, nil
}
