package nftables

import (
	nf "github.com/google/nftables"
	"github.com/pkg/errors"
)

func ensureFlowtable(table *nf.Table, cfg *Config, devices []string) error {
	conn := &nf.Conn{}

	existing, err := findFlowtable(conn, table, cfg.FlowtableName)
	if err != nil {
		return err
	}

	priority := nf.FlowtablePriority(cfg.FlowtablePriority)

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
		conn.DelFlowtable(&nf.Flowtable{Table: table, Name: cfg.FlowtableName})
		if err := conn.Flush(); err != nil {
			return errors.Wrap(err, "failed to delete existing flowtable")
		}
	}

	conn.AddFlowtable(&nf.Flowtable{
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

func findFlowtable(conn *nf.Conn, table *nf.Table, name string) (*nf.Flowtable, error) {
	flowtables, err := conn.ListFlowtables(table)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list flowtables")
	}
	for _, ft := range flowtables {
		if ft.Name == name {
			return ft, nil
		}
	}
	return nil, nil
}
