package system

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/mdlayher/netlink"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
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
	ifNameSize                  = 16
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

// NFTablesConfig captures the tunables required to manage the gwd nftables table.
type NFTablesConfig struct {
	TableName        string
	FlowtableName    string
	LanSetName       string
	InputChainName   string
	ForwardChainName string
	OutputChainName  string

	FilterPriority    int32
	FlowtableHook     *nftables.FlowtableHook
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

// DefaultNFTablesConfig returns a configuration populated with safe defaults.
func DefaultNFTablesConfig() *NFTablesConfig {
	cfg := &NFTablesConfig{
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

	cfg.FlowtableHook = nftables.FlowtableHookIngress
	return cfg
}

func (c *NFTablesConfig) clone() *NFTablesConfig {
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

func (c *NFTablesConfig) applyDefaults() {
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
		c.FlowtableHook = nftables.FlowtableHookIngress
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
		if autoCIDRs, err := detectDefaultLanCIDRs(c); err == nil && len(autoCIDRs) > 0 {
			c.LanCIDRs = autoCIDRs
		} else {
			c.LanCIDRs = append([]string{}, defaultLanCIDRs...)
		}
	}
}

// EnsureNftables reconciles the nftables objects required by GWD: table, flowtable,
// CIDR set, chains, and deterministic rules. The implementation is idempotent and
// keeps its changes scoped to the gwd-prefixed artifacts.
func EnsureNftables(cfg *NFTablesConfig) error {
	if cfg == nil {
		cfg = DefaultNFTablesConfig()
	} else {
		cfg = cfg.clone()
		cfg.applyDefaults()
	}

	tableRef := &nftables.Table{
		Name:   cfg.TableName,
		Family: nftables.TableFamilyINet,
	}

	if err := ensureTableExists(tableRef); err != nil {
		return err
	}

	autoDevices, autoBypass, err := detectFlowtableDevices(cfg)
	if err != nil {
		return err
	}

	flowDevices := selectFlowtableDevices(cfg, autoDevices)
	inputBypass := mergeStringSets(autoBypass, cfg.InputBypassIfaces)
	forwardBypass := mergeStringSets(autoBypass, cfg.ForwardBypassIfaces)

	if err := ensureFlowtable(tableRef, cfg, flowDevices); err != nil {
		return err
	}

	lanSet, err := ensureLanCIDRSet(tableRef, cfg)
	if err != nil {
		return err
	}

	if err := ensureBaseChains(tableRef, cfg); err != nil {
		return err
	}

	if err := programChains(tableRef, cfg, lanSet, flowDevices, inputBypass, forwardBypass); err != nil {
		return err
	}

	return nil
}

// RemoveNftables tears down the managed nftables table, allowing a clean rollback.
func RemoveNftables(cfg *NFTablesConfig) error {
	if cfg == nil {
		cfg = DefaultNFTablesConfig()
	} else {
		cfg = cfg.clone()
		cfg.applyDefaults()
	}

	conn := &nftables.Conn{}

	exists, err := tableExists(conn, cfg.TableName, nftables.TableFamilyINet)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	conn.DelTable(&nftables.Table{
		Name:   cfg.TableName,
		Family: nftables.TableFamilyINet,
	})

	if err := conn.Flush(); err != nil {
		return errors.Wrap(err, "failed to delete nftables table")
	}

	return nil
}

func ensureTableExists(table *nftables.Table) error {
	conn := &nftables.Conn{}
	exists, err := tableExists(conn, table.Name, table.Family)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	conn.AddTable(&nftables.Table{
		Name:   table.Name,
		Family: table.Family,
	})
	if err := conn.Flush(); err != nil {
		return errors.Wrapf(err, "failed to create table %s", table.Name)
	}
	return nil
}

func tableExists(conn *nftables.Conn, name string, family nftables.TableFamily) (bool, error) {
	tables, err := conn.ListTablesOfFamily(family)
	if err != nil {
		return false, errors.Wrap(err, "failed to list tables")
	}
	for _, t := range tables {
		if t.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func ensureFlowtable(table *nftables.Table, cfg *NFTablesConfig, devices []string) error {
	conn := &nftables.Conn{}

	existing, err := findFlowtable(conn, table, cfg.FlowtableName)
	if err != nil {
		return err
	}

	priority := nftables.FlowtablePriority(cfg.FlowtablePriority)

	needsUpdate := existing == nil ||
		!stringSlicesEqual(existing.Devices, devices) ||
		existing.Priority == nil ||
		int32(*existing.Priority) != cfg.FlowtablePriority ||
		existing.Hooknum == nil ||
		(existing.Hooknum != nil && cfg.FlowtableHook != nil && *existing.Hooknum != *cfg.FlowtableHook)

	if !needsUpdate {
		return nil
	}

	if existing != nil {
		conn.DelFlowtable(&nftables.Flowtable{
			Table: table,
			Name:  cfg.FlowtableName,
		})
		if err := conn.Flush(); err != nil {
			return errors.Wrap(err, "failed to delete existing flowtable")
		}
	}

	conn.AddFlowtable(&nftables.Flowtable{
		Table:    table,
		Name:     cfg.FlowtableName,
		Hooknum:  cfg.FlowtableHook,
		Priority: &priority,
		Devices:  devices,
	})

	if err := conn.Flush(); err != nil {
		return errors.Wrap(err, "failed to apply flowtable configuration")
	}

	return nil
}

func findFlowtable(conn *nftables.Conn, table *nftables.Table, name string) (*nftables.Flowtable, error) {
	fts, err := conn.ListFlowtables(table)
	if err != nil {
		var opErr *netlink.OpError
		if errors.As(err, &opErr) {
			if errors.Is(opErr.Err, unix.ENOENT) || errors.Is(opErr.Err, os.ErrNotExist) {
				return nil, nil
			}
		}
		if errors.Is(err, unix.ENOENT) || errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to list flowtables")
	}
	for _, ft := range fts {
		if ft.Name == name {
			return ft, nil
		}
	}
	return nil, nil
}

func ensureBaseChains(table *nftables.Table, cfg *NFTablesConfig) error {
	conn := &nftables.Conn{}
	chains, err := conn.ListChainsOfTableFamily(table.Family)
	if err != nil {
		return errors.Wrap(err, "failed to enumerate chains")
	}

	existing := make(map[string]struct{})
	for _, ch := range chains {
		if ch.Table != nil && ch.Table.Name == table.Name {
			existing[ch.Name] = struct{}{}
		}
	}

	chainSpecs := []struct {
		name string
		hook *nftables.ChainHook
	}{
		{cfg.InputChainName, nftables.ChainHookInput},
		{cfg.ForwardChainName, nftables.ChainHookForward},
		{cfg.OutputChainName, nftables.ChainHookOutput},
	}

	policy := nftables.ChainPolicyAccept
	priority := nftables.ChainPriority(cfg.FilterPriority)
	priorityPtr := nftables.ChainPriorityRef(priority)

	for _, spec := range chainSpecs {
		if spec.name == "" {
			continue
		}
		if _, ok := existing[spec.name]; ok {
			continue
		}

		conn.AddChain(&nftables.Chain{
			Name:     spec.name,
			Table:    table,
			Hooknum:  spec.hook,
			Type:     nftables.ChainTypeFilter,
			Policy:   &policy,
			Priority: priorityPtr,
		})
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrap(err, "failed to ensure chains")
	}

	return nil
}

func programChains(table *nftables.Table, cfg *NFTablesConfig, lanSet *nftables.Set, flowDevices []string, inputBypass, forwardBypass []string) error {
	conn := &nftables.Conn{}

	conn.FlushChain(&nftables.Chain{Name: cfg.InputChainName, Table: table})
	conn.FlushChain(&nftables.Chain{Name: cfg.ForwardChainName, Table: table})
	if cfg.OutputChainName != "" {
		conn.FlushChain(&nftables.Chain{Name: cfg.OutputChainName, Table: table})
	}

	if err := ensureBypassRule(conn, table, cfg.InputChainName, cfg.InputBypassSetName, inputBypass); err != nil {
		return err
	}
	if err := addSanityRules(conn, table, cfg.InputChainName); err != nil {
		return err
	}

	if err := ensureBypassRule(conn, table, cfg.ForwardChainName, cfg.ForwardBypassSetName, forwardBypass); err != nil {
		return err
	}
	if err := addSanityRules(conn, table, cfg.ForwardChainName); err != nil {
		return err
	}
	if len(flowDevices) > 0 && lanSet != nil && lanSet.ID != 0 {
		if err := addFlowOffloadRules(conn, table, cfg.ForwardChainName, cfg.FlowtableName, lanSet); err != nil {
			return err
		}
	}

	if err := conn.Flush(); err != nil {
		return errors.Wrap(err, "failed to program gwd chains")
	}

	return nil
}

func ensureBypassRule(conn *nftables.Conn, table *nftables.Table, chainName, setName string, ifaces []string) error {
	if chainName == "" || setName == "" {
		return nil
	}

	chain := &nftables.Chain{Name: chainName, Table: table}

	set, err := ensureIfaceSet(conn, table, setName, ifaces)
	if err != nil {
		return err
	}
	if set == nil {
		return nil
	}

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Lookup{
				SourceRegister: 1,
				SetName:        set.Name,
				SetID:          set.ID,
			},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})

	return nil
}

func ensureIfaceSet(conn *nftables.Conn, table *nftables.Table, name string, ifaces []string) (*nftables.Set, error) {
	if name == "" {
		return nil, nil
	}

	tableRef := &nftables.Table{Name: table.Name, Family: table.Family}

	set, err := findSet(conn, tableRef, name)
	if err != nil {
		return nil, err
	}

	created := false
	if set == nil {
		set = &nftables.Set{
			Table:   tableRef,
			Name:    name,
			KeyType: nftables.TypeIFName,
		}
		if err := conn.AddSet(set, nil); err != nil {
			return nil, errors.Wrapf(err, "failed to create interface set %s", name)
		}
		if err := conn.Flush(); err != nil {
			return nil, errors.Wrapf(err, "failed to materialize interface set %s", name)
		}
		created = true
		set, err = findSet(conn, tableRef, name)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, errors.Errorf("failed to retrieve newly created interface set %s", name)
		}
	}

	desired := make([]nftables.SetElement, 0, len(ifaces))
	for _, iface := range uniqueStrings(ifaces) {
		desired = append(desired, nftables.SetElement{Key: encodeInterfaceName(iface)})
	}

	current, err := conn.GetSetElements(set)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list elements for set %s", name)
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return nil, errors.Wrapf(err, "failed to delete stale interface entries from set %s", name)
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return nil, errors.Wrapf(err, "failed to add interface entries to set %s", name)
		}
	}

	if created || len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return nil, errors.Wrapf(err, "failed to apply updates to interface set %s", name)
		}
	}

	set.Table = table
	return set, nil
}

func addSanityRules(conn *nftables.Conn, table *nftables.Table, chainName string) error {
	chain := &nftables.Chain{Name: chainName, Table: table}

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: ctStateDropExprs(expr.CtStateBitINVALID),
	})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpNewWithoutSynExprs(),
	})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x03, 0x03, expr.CmpOpEq), // FIN|SYN
	})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x06, 0x06, expr.CmpOpEq), // SYN|RST
	})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x3f, 0x00, expr.CmpOpEq), // NULL
	})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x29, 0x29, expr.CmpOpEq), // XMAS (FIN|PSH|URG)
	})

	return nil
}

func addFlowOffloadRules(conn *nftables.Conn, table *nftables.Table, chainName, flowtableName string, lanSet *nftables.Set) error {
	chain := &nftables.Chain{Name: chainName, Table: table}

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: flowOffloadExprs(lanSet, flowtableName, true),
	})

	conn.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: flowOffloadExprs(lanSet, flowtableName, false),
	})
	return nil
}

func detectFlowtableDevices(cfg *NFTablesConfig) ([]string, []string, error) {
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

func detectDefaultLanCIDRs(cfg *NFTablesConfig) ([]string, error) {
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
			if !ok {
				continue
			}

			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}

			if ip4[0] == 169 && ip4[1] == 254 {
				continue
			}

			network := ip4.Mask(ipNet.Mask)
			if network == nil || network.IsLoopback() || network.IsUnspecified() {
				continue
			}

			cidr := (&net.IPNet{IP: network, Mask: ipNet.Mask}).String()
			cidrs[cidr] = struct{}{}
		}
	}

	return sortedKeys(cidrs), nil
}

func selectFlowtableDevices(cfg *NFTablesConfig, auto []string) []string {
	if len(cfg.FlowtableDeviceExplicit) > 0 {
		return uniqueStrings(cfg.FlowtableDeviceExplicit)
	}

	autoSet := make(map[string]struct{}, len(auto))
	for _, name := range auto {
		if name != "" {
			autoSet[name] = struct{}{}
		}
	}

	for _, name := range cfg.FlowtableDeviceInclude {
		if name != "" {
			autoSet[name] = struct{}{}
		}
	}

	result := make([]string, 0, len(autoSet))
	for name := range autoSet {
		result = append(result, name)
	}
	sort.Strings(result)

	return result
}

func ensureLanCIDRSet(table *nftables.Table, cfg *NFTablesConfig) (*nftables.Set, error) {
	conn := &nftables.Conn{}
	tableRef := &nftables.Table{Name: table.Name, Family: table.Family}

	set, err := findSet(conn, tableRef, cfg.LanSetName)
	if err != nil {
		return nil, err
	}

	created := false
	if set == nil {
		set = &nftables.Set{
			Table:    tableRef,
			Name:     cfg.LanSetName,
			KeyType:  nftables.TypeIPAddr,
			Interval: true,
		}
		if err := conn.AddSet(set, nil); err != nil {
			return nil, errors.Wrap(err, "failed to create LAN CIDR set")
		}
		if err := conn.Flush(); err != nil {
			return nil, errors.Wrap(err, "failed to materialize LAN set")
		}
		created = true
		set, err = findSet(conn, tableRef, cfg.LanSetName)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, errors.Errorf("failed to retrieve newly created set %s", cfg.LanSetName)
		}
	}

	desiredElems, err := cidrToElements(cfg.LanCIDRs)
	if err != nil {
		return nil, err
	}

	sortSetElements(desiredElems)

	currentElems, err := conn.GetSetElements(set)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list set elements")
	}
	sortSetElements(currentElems)

	toAdd, toDel := diffSetElements(currentElems, desiredElems)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return nil, errors.Wrap(err, "failed to delete obsolete set elements")
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return nil, errors.Wrap(err, "failed to add CIDR set elements")
		}
	}

	if created || len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return nil, errors.Wrap(err, "failed to apply set updates")
		}
	}

	set.Table = table
	return set, nil
}

func findSet(conn *nftables.Conn, table *nftables.Table, name string) (*nftables.Set, error) {
	sets, err := conn.GetSets(table)
	if err != nil {
		return nil, errors.Wrap(err, "failed to enumerate sets")
	}
	for _, s := range sets {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, nil
}

func sortSetElements(elems []nftables.SetElement) {
	sort.Slice(elems, func(i, j int) bool {
		if cmp := bytes.Compare(elems[i].Key, elems[j].Key); cmp != 0 {
			return cmp < 0
		}
		if elems[i].IntervalEnd != elems[j].IntervalEnd {
			return !elems[i].IntervalEnd && elems[j].IntervalEnd
		}
		return false
	})
}

func diffSetElements(current, desired []nftables.SetElement) (toAdd, toDel []nftables.SetElement) {
	currMap := make(map[string]nftables.SetElement, len(current))
	for _, elem := range current {
		currMap[elementKey(elem)] = elem
	}

	desiredMap := make(map[string]nftables.SetElement, len(desired))
	for _, elem := range desired {
		desiredMap[elementKey(elem)] = elem
		if _, exists := currMap[elementKey(elem)]; !exists {
			toAdd = append(toAdd, elem)
		}
	}

	for key, elem := range currMap {
		if _, exists := desiredMap[key]; !exists {
			toDel = append(toDel, elem)
		}
	}

	return toAdd, toDel
}

func elementKey(elem nftables.SetElement) string {
	return fmt.Sprintf("%x|%t", elem.Key, elem.IntervalEnd)
}

func cidrToElements(cidrs []string) ([]nftables.SetElement, error) {
	var elems []nftables.SetElement
	for _, cidr := range cidrs {
		if strings.TrimSpace(cidr) == "" {
			continue
		}
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			return nil, errors.Wrapf(err, "invalid CIDR: %s", cidr)
		}

		start, end, err := cidrRange(network)
		if err != nil {
			return nil, err
		}

		elems = append(elems,
			nftables.SetElement{Key: start},
			nftables.SetElement{Key: end, IntervalEnd: true},
		)
	}

	return elems, nil
}

func cidrRange(n *net.IPNet) ([]byte, []byte, error) {
	start := networkIP(n)
	if start == nil {
		return nil, nil, errors.Errorf("unsupported address family for network %s", n.String())
	}

	last := broadcastIP(n)
	if last == nil {
		return nil, nil, errors.Errorf("failed to compute broadcast for %s", n.String())
	}

	end := incrementIP(last)
	if end == nil {
		return nil, nil, errors.Errorf("CIDR %s overflowed maximum address", n.String())
	}

	return start, end, nil
}

func networkIP(n *net.IPNet) []byte {
	if ip := n.IP.To4(); ip != nil {
		m := ip.Mask(n.Mask)
		return append([]byte(nil), m...)
	}
	if ip := n.IP.To16(); ip != nil {
		m := ip.Mask(n.Mask)
		return append([]byte(nil), m...)
	}
	return nil
}

func broadcastIP(n *net.IPNet) []byte {
	var ip []byte
	if v4 := n.IP.To4(); v4 != nil {
		ip = append([]byte(nil), v4...)
	} else {
		ip = append([]byte(nil), n.IP.To16()...)
	}
	if ip == nil {
		return nil
	}

	for i := range ip {
		ip[i] |= ^n.Mask[i]
	}
	return ip
}

func incrementIP(ip []byte) []byte {
	out := append([]byte(nil), ip...)
	for i := len(out) - 1; i >= 0; i-- {
		out[i]++
		if out[i] != 0 {
			return out
		}
	}
	return nil
}

func encodeInterfaceName(name string) []byte {
	buf := make([]byte, ifNameSize)
	copy(buf, []byte(name+"\x00"))
	return buf
}

func ctStateDropExprs(bit uint32) []expr.Any {
	mask := binaryutil.NativeEndian.PutUint32(bit)
	return []expr.Any{
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           mask,
			Xor:            binaryutil.NativeEndian.PutUint32(0),
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     zeroBytes(4),
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	}
}

func tcpNewWithoutSynExprs() []expr.Any {
	maskSyn := []byte{0x02}

	exprs := append(protoTCPMatchExprs(),
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitNEW),
			Xor:            binaryutil.NativeEndian.PutUint32(0),
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     zeroBytes(4),
		},
		loadTCPFlags(), // overwrites register 1 with flags
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            1,
			Mask:           maskSyn,
			Xor:            []byte{0x00},
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     maskSyn,
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	)

	return exprs
}

func tcpFlagMaskDropExprs(mask, expect byte, op expr.CmpOp) []expr.Any {
	exprs := append(protoTCPMatchExprs(),
		loadTCPFlags(),
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            1,
			Mask:           []byte{mask},
			Xor:            []byte{0x00},
		},
		&expr.Cmp{
			Op:       op,
			Register: 1,
			Data:     []byte{expect},
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	)

	return exprs
}

func flowOffloadExprs(lanSet *nftables.Set, flowtableName string, matchSrc bool) []expr.Any {
	offset := uint32(12) // IPv4 source
	if !matchSrc {
		offset = 16 // IPv4 destination
	}

	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     binaryutil.NativeEndian.PutUint32(uint32(unix.NFPROTO_IPV4)),
		},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       offset,
			Len:          4,
		},
		&expr.Lookup{
			SourceRegister: 1,
			SetName:        lanSet.Name,
			SetID:          lanSet.ID,
		},
		&expr.FlowOffload{Name: flowtableName},
	}
}

func protoTCPMatchExprs() []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     binaryutil.NativeEndian.PutUint32(uint32(unix.IPPROTO_TCP)),
		},
	}
}

func loadTCPFlags() expr.Any {
	return &expr.Payload{
		DestRegister: 1,
		Base:         expr.PayloadBaseTransportHeader,
		Offset:       13,
		Len:          1,
	}
}

func zeroBytes(n int) []byte {
	return make([]byte, n)
}

func uniqueStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for v := range set {
		result = append(result, v)
	}
	sort.Strings(result)
	return result
}

func mergeStringSets(groups ...[]string) []string {
	set := make(map[string]struct{})
	for _, group := range groups {
		for _, item := range group {
			if item == "" {
				continue
			}
			set[item] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for item := range set {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string{}, a...)
	bc := append([]string{}, b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	return set
}

func isExcludedByPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
