package nftables

import (
	nf "github.com/google/nftables"
)

func ensureLanCIDRSet(table *nf.Table, cfg *Config) (*nf.Set, error) {
	conn := &nf.Conn{}

	opts := setEnsureOptions{
		Name:           cfg.LanSetName,
		KeyType:        nf.TypeIPAddr,
		Interval:       true,
		Operation:      "nftables.ensureLanCIDRSet",
		Entity:         "CIDRs",
		SetDescription: "LAN CIDR set",
	}

	builder := func() ([]nf.SetElement, error) {
		return cidrToElements(cfg.LanCIDRs)
	}

	return ensureSet(conn, table, opts, builder)
}

func updateLanSet(conn *nf.Conn, set *nf.Set, cidrs []string) error {
	elements, err := cidrToElements(cidrs)
	if err != nil {
		return err
	}

	_, err = syncSetElements(conn, set, elements, setUpdateContext{
		Operation: "nftables.updateLanSet",
		Entity:    "CIDRs",
	})
	return err
}

func ensureIfaceSet(conn *nf.Conn, table *nf.Table, name string, ifaces []string) (*nf.Set, error) {
	if name == "" {
		return nil, nil
	}

	opts := setEnsureOptions{
		Name:           name,
		KeyType:        nf.TypeIFName,
		Interval:       false,
		Operation:      "nftables.ensureIfaceSet",
		Entity:         "interface entries",
		SetDescription: "interface set",
	}

	builder := func() ([]nf.SetElement, error) {
		if len(ifaces) == 0 {
			return nil, nil
		}
		desired := make([]nf.SetElement, 0, len(ifaces))
		for _, iface := range uniqueStrings(ifaces) {
			desired = append(desired, nf.SetElement{Key: encodeInterfaceName(iface)})
		}
		return desired, nil
	}

	return ensureSet(conn, table, opts, builder)
}
