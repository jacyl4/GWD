package nftables

import (
	"bytes"
	"fmt"
	"sort"

	apperrors "GWD/internal/errors"
	nf "github.com/google/nftables"
)

type setEnsureOptions struct {
	Name           string
	KeyType        nf.SetDatatype
	Interval       bool
	Operation      string
	Entity         string
	SetDescription string
}

type setUpdateContext struct {
	Operation string
	Entity    string
}

func ensureSet(conn *nf.Conn, table *nf.Table, opts setEnsureOptions, elementsBuilder func() ([]nf.SetElement, error)) (*nf.Set, error) {
	tableRef := &nf.Table{Name: table.Name, Family: table.Family}

	set, err := findSet(conn, tableRef, opts.Name)
	if err != nil {
		return nil, err
	}

	if set == nil {
		set = &nf.Set{
			Table:    tableRef,
			Name:     opts.Name,
			KeyType:  opts.KeyType,
			Interval: opts.Interval,
		}
		if err := conn.AddSet(set, nil); err != nil {
			return nil, firewallError(opts.Operation+".addSet", fmt.Sprintf("failed to create %s", opts.SetDescription), err, apperrors.Metadata{
				"set": opts.Name,
			})
		}
		if err := conn.Flush(); err != nil {
			return nil, firewallError(opts.Operation+".flushCreate", fmt.Sprintf("failed to materialize %s", opts.SetDescription), err, apperrors.Metadata{
				"set": opts.Name,
			})
		}

		set, err = findSet(conn, tableRef, opts.Name)
		if err != nil {
			return nil, err
		}
		if set == nil {
			return nil, firewallError(opts.Operation+".lookup", fmt.Sprintf("failed to retrieve newly created %s", opts.SetDescription), nil, apperrors.Metadata{
				"set": opts.Name,
			})
		}
	}

	desired, err := elementsBuilder()
	if err != nil {
		return nil, err
	}

	if _, err := syncSetElements(conn, set, desired, setUpdateContext{
		Operation: opts.Operation,
		Entity:    opts.Entity,
	}); err != nil {
		return nil, err
	}

	set.Table = table
	return set, nil
}

func findSet(conn *nf.Conn, table *nf.Table, name string) (*nf.Set, error) {
	sets, err := conn.GetSets(table)
	if err != nil {
		return nil, firewallError("nftables.findSet", "failed to enumerate sets", err, apperrors.Metadata{
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

func syncSetElements(conn *nf.Conn, set *nf.Set, desired []nf.SetElement, ctx setUpdateContext) (bool, error) {
	metadata := apperrors.Metadata{
		"set": set.Name,
	}

	current, err := conn.GetSetElements(set)
	if err != nil {
		return false, firewallError(ctx.Operation+".getElements", fmt.Sprintf("failed to read existing %s for set", ctx.Entity), err, metadata)
	}

	sortSetElements(desired)
	sortSetElements(current)

	toAdd, toDel := diffSetElements(current, desired)

	changed := false
	if len(toDel) > 0 {
		if err := conn.SetDeleteElements(set, toDel); err != nil {
			return false, firewallError(ctx.Operation+".deleteElements", fmt.Sprintf("failed to prune stale %s from set", ctx.Entity), err, metadata)
		}
		changed = true
	}

	if len(toAdd) > 0 {
		if err := conn.SetAddElements(set, toAdd); err != nil {
			return false, firewallError(ctx.Operation+".addElements", fmt.Sprintf("failed to add %s to set", ctx.Entity), err, metadata)
		}
		changed = true
	}

	if changed {
		if err := conn.Flush(); err != nil {
			return false, firewallError(ctx.Operation+".flush", fmt.Sprintf("failed to apply %s updates to set", ctx.Entity), err, metadata)
		}
	}

	return changed, nil
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

func elementKey(elem nf.SetElement) string {
	return fmt.Sprintf("%x|%t", elem.Key, elem.IntervalEnd)
}
