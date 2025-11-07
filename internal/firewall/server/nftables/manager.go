package nftables

import (
	nf "github.com/google/nftables"
)

// Ensure reconciles the nftables objects required by GWD: table, flowtable,
// sets and chains. Callers typically supply a config generated from
// DefaultConfig and apply custom overrides prior to calling Ensure.
func Ensure(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	} else {
		cfg = cfg.clone()
		cfg.applyDefaults()
	}

	tableRef := &nf.Table{
		Name:   cfg.TableName,
		Family: nf.TableFamilyINet,
	}

	if err := ensureTableExists(tableRef); err != nil {
		return err
	}

	autoDevices, autoBypass, err := detectFlowtableDevices(cfg)
	// bail early to avoid partial state changes if detection fails.
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

// Remove tears down the managed nftables table, allowing a clean rollback.
func Remove(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	} else {
		cfg = cfg.clone()
		cfg.applyDefaults()
	}

	conn := &nf.Conn{}

	exists, err := tableExists(conn, cfg.TableName, nf.TableFamilyINet)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	conn.DelTable(&nf.Table{
		Name:   cfg.TableName,
		Family: nf.TableFamilyINet,
	})

	if err := conn.Flush(); err != nil {
		return firewallError("nftables.Remove", "failed to delete nftables table", err, nil)
	}

	return nil
}
