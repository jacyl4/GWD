package nftables

import (
	nf "github.com/google/nftables"
	"github.com/pkg/errors"
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
		return errors.Wrapf(err, "failed to create table %s", table.Name)
	}
	return nil
}

func tableExists(conn *nf.Conn, name string, family nf.TableFamily) (bool, error) {
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
