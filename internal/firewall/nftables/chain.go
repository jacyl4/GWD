package nftables

import (
	nf "github.com/google/nftables"

	apperrors "GWD/internal/errors"
)

func ensureBaseChains(table *nf.Table, cfg *Config) error {
	conn := &nf.Conn{}

	chains, err := conn.ListChainsOfTableFamily(table.Family)
	if err != nil {
		return wrapFirewallError(err, "nftables.ensureBaseChains.list", "failed to enumerate chains", apperrors.Metadata{
			"table": table.Name,
		})
	}

	existing := make(map[string]struct{})
	for _, ch := range chains {
		if ch.Table != nil && ch.Table.Name == table.Name {
			existing[ch.Name] = struct{}{}
		}
	}

	chainSpecs := []struct {
		name string
		hook *nf.ChainHook
	}{
		{cfg.InputChainName, nf.ChainHookInput},
		{cfg.ForwardChainName, nf.ChainHookForward},
		{cfg.OutputChainName, nf.ChainHookOutput},
	}

	policy := nf.ChainPolicyAccept
	priority := nf.ChainPriority(cfg.FilterPriority)
	priorityPtr := nf.ChainPriorityRef(priority)

	for _, spec := range chainSpecs {
		if spec.name == "" {
			continue
		}
		if _, ok := existing[spec.name]; ok {
			continue
		}

		conn.AddChain(&nf.Chain{
			Name:     spec.name,
			Table:    table,
			Hooknum:  spec.hook,
			Type:     nf.ChainTypeFilter,
			Policy:   &policy,
			Priority: priorityPtr,
		})
	}

	if err := conn.Flush(); err != nil {
		return wrapFirewallError(err, "nftables.ensureBaseChains.flush", "failed to ensure chains", apperrors.Metadata{
			"table": table.Name,
		})
	}

	return nil
}

func programChains(table *nf.Table, cfg *Config, lanSet *nf.Set, flowDevices, inputBypass, forwardBypass []string) error {
	conn := &nf.Conn{}

	if cfg.InputChainName != "" {
		conn.FlushChain(&nf.Chain{Name: cfg.InputChainName, Table: table})
	}
	if cfg.ForwardChainName != "" {
		conn.FlushChain(&nf.Chain{Name: cfg.ForwardChainName, Table: table})
	}
	if cfg.OutputChainName != "" {
		conn.FlushChain(&nf.Chain{Name: cfg.OutputChainName, Table: table})
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
		return wrapFirewallError(err, "nftables.programChains.flush", "failed to program nftables chains", apperrors.Metadata{
			"table": table.Name,
		})
	}

	return nil
}
