package nftables

type deviceFilter struct {
	explicitMode    bool
	explicit        map[string]struct{}
	include         map[string]struct{}
	exclude         map[string]struct{}
	excludePrefixes []string
}

func newDeviceFilter(cfg *Config) deviceFilter {
	return deviceFilter{
		explicitMode:    len(cfg.FlowtableDeviceExplicit) > 0,
		explicit:        stringSet(cfg.FlowtableDeviceExplicit),
		include:         stringSet(cfg.FlowtableDeviceInclude),
		exclude:         stringSet(cfg.FlowtableDeviceExclude),
		excludePrefixes: append([]string(nil), cfg.FlowtableDeviceExcludePrefixes...),
	}
}

func (f deviceFilter) isForced(name string) bool {
	if f.explicitMode {
		_, ok := f.explicit[name]
		return ok
	}
	_, ok := f.include[name]
	return ok
}

func (f deviceFilter) isExcluded(name string) bool {
	if _, ok := f.exclude[name]; ok {
		return true
	}
	return isExcludedByPrefix(name, f.excludePrefixes)
}

func (f deviceFilter) allow(name string, fallback func() bool) bool {
	if name == "" {
		return false
	}
	if f.explicitMode {
		_, ok := f.explicit[name]
		return ok
	}
	if _, ok := f.include[name]; ok {
		return true
	}
	if f.isExcluded(name) {
		return false
	}
	if fallback != nil {
		return fallback()
	}
	return true
}

func (f deviceFilter) shouldBypass(name string) bool {
	return f.isExcluded(name) && !f.isForced(name)
}
