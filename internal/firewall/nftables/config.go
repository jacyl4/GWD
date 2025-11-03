package nftables

import (
	"sort"

	nf "github.com/google/nftables"
)

const (
	defaultTableName        = "gwd"
	defaultFlowtableName    = "gwd_ft"
	defaultLanSetName       = "gwd_lan_cidrs"
	defaultInputChainName   = "gwd_input"
	defaultForwardChainName = "gwd_forward"
	defaultOutputChainName  = "gwd_output"
	defaultInputBypassSet   = "gwd_input_bypass_ifaces"
	defaultForwardBypassSet = "gwd_forward_bypass_ifaces"

	defaultFilterPriority int32 = -151
	defaultFlowtablePrio  int32 = -300
)

var (
	defaultLanCIDRs = []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
	}
	defaultFlowtableExcludeExact = []string{}

	defaultFlowtableExcludePrefixes = []string{}

	defaultInputBypassIfaces = []string{}

	defaultForwardBypassIfaces = []string{}
)

// Config captures the tunables required to manage the gwd nftables table.
type Config struct {
	TableName        string
	FlowtableName    string
	LanSetName       string
	InputChainName   string
	ForwardChainName string
	OutputChainName  string

	FilterPriority    int32
	FlowtableHook     *nf.FlowtableHook
	FlowtablePriority int32

	LanCIDRs []string

	// Flowtable device management.
	FlowtableDeviceExplicit        []string // if non-empty, use exactly this list
	FlowtableDeviceInclude         []string // additional devices to include alongside auto-detected
	FlowtableDeviceExclude         []string
	FlowtableDeviceExcludePrefixes []string

	InputBypassIfaces   []string
	ForwardBypassIfaces []string

	InputBypassSetName   string
	ForwardBypassSetName string
}

// DefaultConfig returns a configuration populated with safe defaults.
func DefaultConfig() *Config {
	cfg := &Config{
		TableName:                      defaultTableName,
		FlowtableName:                  defaultFlowtableName,
		LanSetName:                     defaultLanSetName,
		InputChainName:                 defaultInputChainName,
		ForwardChainName:               defaultForwardChainName,
		OutputChainName:                defaultOutputChainName,
		FilterPriority:                 defaultFilterPriority,
		FlowtablePriority:              defaultFlowtablePrio,
		LanCIDRs:                       append([]string{}, defaultLanCIDRs...),
		FlowtableDeviceExclude:         append([]string{}, defaultFlowtableExcludeExact...),
		FlowtableDeviceExcludePrefixes: append([]string{}, defaultFlowtableExcludePrefixes...),
		InputBypassIfaces:              append([]string{}, defaultInputBypassIfaces...),
		ForwardBypassIfaces:            append([]string{}, defaultForwardBypassIfaces...),
		InputBypassSetName:             defaultInputBypassSet,
		ForwardBypassSetName:           defaultForwardBypassSet,
	}

	cfg.FlowtableHook = nf.FlowtableHookIngress
	return cfg
}

func (c *Config) clone() *Config {
	if c == nil {
		return nil
	}

	clone := *c
	clone.LanCIDRs = append([]string{}, c.LanCIDRs...)
	clone.FlowtableDeviceExplicit = append([]string{}, c.FlowtableDeviceExplicit...)
	clone.FlowtableDeviceInclude = append([]string{}, c.FlowtableDeviceInclude...)
	clone.FlowtableDeviceExclude = append([]string{}, c.FlowtableDeviceExclude...)
	clone.FlowtableDeviceExcludePrefixes = append([]string{}, c.FlowtableDeviceExcludePrefixes...)
	clone.InputBypassIfaces = append([]string{}, c.InputBypassIfaces...)
	clone.ForwardBypassIfaces = append([]string{}, c.ForwardBypassIfaces...)
	clone.InputBypassSetName = c.InputBypassSetName
	clone.ForwardBypassSetName = c.ForwardBypassSetName
	return &clone
}

func (c *Config) applyDefaults() {
	if c.TableName == "" {
		c.TableName = defaultTableName
	}
	if c.FlowtableName == "" {
		c.FlowtableName = defaultFlowtableName
	}
	if c.LanSetName == "" {
		c.LanSetName = defaultLanSetName
	}
	if c.InputChainName == "" {
		c.InputChainName = defaultInputChainName
	}
	if c.ForwardChainName == "" {
		c.ForwardChainName = defaultForwardChainName
	}
	if c.OutputChainName == "" {
		c.OutputChainName = defaultOutputChainName
	}
	if c.FilterPriority == 0 {
		c.FilterPriority = defaultFilterPriority
	}
	if c.FlowtableHook == nil {
		c.FlowtableHook = nf.FlowtableHookIngress
	}
	if c.FlowtablePriority == 0 {
		c.FlowtablePriority = defaultFlowtablePrio
	}
	if len(c.FlowtableDeviceExclude) == 0 {
		c.FlowtableDeviceExclude = append([]string{}, defaultFlowtableExcludeExact...)
	}
	if len(c.FlowtableDeviceExcludePrefixes) == 0 {
		c.FlowtableDeviceExcludePrefixes = append([]string{}, defaultFlowtableExcludePrefixes...)
	}
	if len(c.InputBypassIfaces) == 0 {
		c.InputBypassIfaces = append([]string{}, defaultInputBypassIfaces...)
	}
	if len(c.ForwardBypassIfaces) == 0 {
		c.ForwardBypassIfaces = append([]string{}, defaultForwardBypassIfaces...)
	}
	if c.InputBypassSetName == "" {
		c.InputBypassSetName = defaultInputBypassSet
	}
	if c.ForwardBypassSetName == "" {
		c.ForwardBypassSetName = defaultForwardBypassSet
	}
	if len(c.LanCIDRs) == 0 {
		c.LanCIDRs = append([]string{}, defaultLanCIDRs...)
	}
	sort.Strings(c.LanCIDRs)
}
