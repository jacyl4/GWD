package nftables

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"

	nf "github.com/google/nftables"
	"github.com/pkg/errors"
)

func ensureLanCIDRSet(table *nf.Table, cfg *Config) (*nf.Set, error) {
	conn := &nf.Conn{}

	tableRef := &nf.Table{Name: table.Name, Family: table.Family}

	set, err := findSet(conn, tableRef, cfg.LanSetName)
	if err != nil {
		return nil, err
	}

	created := false
	if set == nil {
		set = &nf.Set{
			Table:    tableRef,
			Name:     cfg.LanSetName,
			KeyType:  nf.TypeIPAddr,
			Interval: true,
		}
		if err := conn.AddSet(set, nil); err != nil {
			return nil, errors.Wrapf(err, "failed to create LAN CIDR set %s", cfg.LanSetName)
		}
		if err := conn.Flush(); err != nil {
			return nil, errors.Wrap(err, "failed to materialize LAN CIDR set")
		}
		created = true
		set, err = findSet(conn, tableRef, cfg.LanSetName)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, errors.Errorf("failed to retrieve newly created LAN CIDR set %s", cfg.LanSetName)
		}
	}

	desired, err := cidrToElements(cfg.LanCIDRs)
	if err != nil {
		return nil, err
	}

	current, err := conn.GetSetElements(set)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read existing CIDRs for set %s", set.Name)
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return nil, errors.Wrapf(err, "failed to prune stale CIDRs from set %s", set.Name)
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return nil, errors.Wrapf(err, "failed to add CIDRs to set %s", set.Name)
		}
	}

	if created || len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return nil, errors.Wrapf(err, "failed to apply CIDR updates to set %s", set.Name)
		}
	}

	set.Table = table
	return set, nil
}

func updateLanSet(conn *nf.Conn, set *nf.Set, cidrs []string) error {
	desired, err := cidrToElements(cidrs)
	if err != nil {
		return err
	}

	current, err := conn.GetSetElements(set)
	if err != nil {
		return errors.Wrapf(err, "failed to read existing CIDRs for set %s", set.Name)
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return errors.Wrapf(err, "failed to prune stale CIDRs from set %s", set.Name)
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return errors.Wrapf(err, "failed to add CIDRs to set %s", set.Name)
		}
	}

	if len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return errors.Wrapf(err, "failed to apply CIDR updates to set %s", set.Name)
		}
	}
	return nil
}

func findSet(conn *nf.Conn, table *nf.Table, name string) (*nf.Set, error) {
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

func cidrToElements(cidrs []string) ([]nf.SetElement, error) {
	var elems []nf.SetElement
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
			nf.SetElement{Key: start},
			nf.SetElement{Key: end, IntervalEnd: true},
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

func elementKey(elem nf.SetElement) string {
	return fmt.Sprintf("%x|%t", elem.Key, elem.IntervalEnd)
}

func ensureIfaceSet(conn *nf.Conn, table *nf.Table, name string, ifaces []string) (*nf.Set, error) {
	if name == "" {
		return nil, nil
	}

	tableRef := &nf.Table{Name: table.Name, Family: table.Family}

	set, err := findSet(conn, tableRef, name)
	if err != nil {
		return nil, err
	}

	created := false
	if set == nil {
		set = &nf.Set{
			Table:   tableRef,
			Name:    name,
			KeyType: nf.TypeIFName,
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

	desired := make([]nf.SetElement, 0, len(ifaces))
	for _, iface := range uniqueStrings(ifaces) {
		desired = append(desired, nf.SetElement{Key: encodeInterfaceName(iface)})
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

func sortSetElements(elems []nf.SetElement) {
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

func diffSetElements(current, desired []nf.SetElement) (toAdd, toDel []nf.SetElement) {
	currMap := make(map[string]nf.SetElement, len(current))
	for _, elem := range current {
		currMap[elementKey(elem)] = elem
	}

	desiredMap := make(map[string]nf.SetElement, len(desired))
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
