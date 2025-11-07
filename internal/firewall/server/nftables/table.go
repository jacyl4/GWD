package nftables

import (
	nf "github.com/google/nftables"

	apperrors "GWD/internal/errors"
)

func ensureTableExists(table *nf.Table) error {
	conn := &nf.Conn{}
	exists, err := tableExists(conn, table.Name, table.Family)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	conn.AddTable(&nf.Table{
		Name:   table.Name,
		Family: table.Family,
	})
	if err := conn.Flush(); err != nil {
		return firewallError("nftables.ensureTableExists.flush", "failed to create table", err, apperrors.Metadata{
			"table": table.Name,
		})
	}
	return nil
}

func tableExists(conn *nf.Conn, name string, family nf.TableFamily) (bool, error) {
	tables, err := conn.ListTablesOfFamily(family)
	if err != nil {
		return false, firewallError("nftables.tableExists", "failed to list tables", err, apperrors.Metadata{
			"family": family,
		})
	}
	for _, t := range tables {
		if t.Name == name {
			return true, nil
		}
	}
	return false, nil
}
