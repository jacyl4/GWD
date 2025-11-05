package nftables

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"

	apperrors "GWD/internal/errors"
	nf "github.com/google/nftables"
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
			return nil, wrapFirewallError(err, "nftables.ensureLanCIDRSet.addSet", "failed to create LAN CIDR set", apperrors.Metadata{
				"set": cfg.LanSetName,
			})
		}
		if err := conn.Flush(); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureLanCIDRSet.flushCreate", "failed to materialize LAN CIDR set", apperrors.Metadata{
				"set": cfg.LanSetName,
			})
		}
		created = true
		set, err = findSet(conn, tableRef, cfg.LanSetName)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, wrapFirewallError(nil, "nftables.ensureLanCIDRSet.lookup", "failed to retrieve newly created LAN CIDR set", apperrors.Metadata{
				"set": cfg.LanSetName,
			})
		}
	}

	desired, err := cidrToElements(cfg.LanCIDRs)
	if err != nil {
		return nil, err
	}

	current, err := conn.GetSetElements(set)
	if err != nil {
		return nil, wrapFirewallError(err, "nftables.ensureLanCIDRSet.getElements", "failed to read existing CIDRs for set", apperrors.Metadata{
			"set": set.Name,
		})
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureLanCIDRSet.deleteElements", "failed to prune stale CIDRs from set", apperrors.Metadata{
				"set": set.Name,
			})
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureLanCIDRSet.addElements", "failed to add CIDRs to set", apperrors.Metadata{
				"set": set.Name,
			})
		}
	}

	if created || len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureLanCIDRSet.flush", "failed to apply CIDR updates to set", apperrors.Metadata{
				"set": set.Name,
			})
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
		return wrapFirewallError(err, "nftables.updateLanSet.getElements", "failed to read existing CIDRs for set", apperrors.Metadata{
			"set": set.Name,
		})
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return wrapFirewallError(err, "nftables.updateLanSet.deleteElements", "failed to prune stale CIDRs from set", apperrors.Metadata{
				"set": set.Name,
			})
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return wrapFirewallError(err, "nftables.updateLanSet.addElements", "failed to add CIDRs to set", apperrors.Metadata{
				"set": set.Name,
			})
		}
	}

	if len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return wrapFirewallError(err, "nftables.updateLanSet.flush", "failed to apply CIDR updates to set", apperrors.Metadata{
				"set": set.Name,
			})
		}
	}
	return nil
}

func findSet(conn *nf.Conn, table *nf.Table, name string) (*nf.Set, error) {
	sets, err := conn.GetSets(table)
	if err != nil {
		return nil, wrapFirewallError(err, "nftables.findSet", "failed to enumerate sets", apperrors.Metadata{
			"table": table.Name,
		})
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
			return nil, wrapFirewallError(err, "nftables.cidrToElements.parse", "invalid CIDR provided", apperrors.Metadata{
				"cidr": cidr,
			})
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
		return nil, nil, wrapFirewallError(nil, "nftables.cidrRange", "unsupported address family for network", apperrors.Metadata{
			"network": n.String(),
		})
	}

	last := broadcastIP(n)
	if last == nil {
		return nil, nil, wrapFirewallError(nil, "nftables.cidrRange", "failed to compute broadcast address", apperrors.Metadata{
			"network": n.String(),
		})
	}

	end := incrementIP(last)
	if end == nil {
		return nil, nil, wrapFirewallError(nil, "nftables.cidrRange", "CIDR overflowed maximum address", apperrors.Metadata{
			"network": n.String(),
		})
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
			return nil, wrapFirewallError(err, "nftables.ensureIfaceSet.addSet", "failed to create interface set", apperrors.Metadata{
				"set": name,
			})
		}
		if err := conn.Flush(); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureIfaceSet.flushCreate", "failed to materialize interface set", apperrors.Metadata{
				"set": name,
			})
		}
		created = true
		set, err = findSet(conn, tableRef, name)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, wrapFirewallError(nil, "nftables.ensureIfaceSet.lookup", "failed to retrieve newly created interface set", apperrors.Metadata{
				"set": name,
			})
		}
	}

	desired := make([]nf.SetElement, 0, len(ifaces))
	for _, iface := range uniqueStrings(ifaces) {
		desired = append(desired, nf.SetElement{Key: encodeInterfaceName(iface)})
	}

	current, err := conn.GetSetElements(set)
	if err != nil {
		return nil, wrapFirewallError(err, "nftables.ensureIfaceSet.getElements", "failed to list elements for set", apperrors.Metadata{
			"set": name,
		})
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureIfaceSet.deleteElements", "failed to delete stale interface entries from set", apperrors.Metadata{
				"set": name,
			})
		}
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureIfaceSet.addElements", "failed to add interface entries to set", apperrors.Metadata{
				"set": name,
			})
		}
	}

	if created || len(toAdd) > 0 || len(toDel) > 0 {
		if err := conn.Flush(); err != nil {
			return nil, wrapFirewallError(err, "nftables.ensureIfaceSet.flush", "failed to apply updates to interface set", apperrors.Metadata{
				"set": name,
			})
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
