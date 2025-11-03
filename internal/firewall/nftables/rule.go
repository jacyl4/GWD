package nftables

import (
	nf "github.com/google/nftables"
	"github.com/google/nftables/expr"
)

func ensureBypassRule(conn *nf.Conn, table *nf.Table, chainName, setName string, ifaces []string) error {
	if chainName == "" || setName == "" {
		return nil
	}

	ifaceSet, err := ensureIfaceSet(conn, table, setName, ifaces)
	if err != nil || ifaceSet == nil {
		return err
	}

	chain := &nf.Chain{Name: chainName, Table: table}
	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Lookup{SourceRegister: 1, SetName: ifaceSet.Name, SetID: ifaceSet.ID},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	})

	return nil
}

func addSanityRules(conn *nf.Conn, table *nf.Table, chainName string) error {
	if chainName == "" {
		return nil
	}

	chain := &nf.Chain{Name: chainName, Table: table}

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: ctStateDropExprs(expr.CtStateBitINVALID),
	})

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpNewWithoutSynExprs(),
	})

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x03, 0x03, expr.CmpOpEq), // FIN|SYN
	})

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x06, 0x06, expr.CmpOpEq), // SYN|RST
	})

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x3f, 0x00, expr.CmpOpEq), // NULL
	})

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: tcpFlagMaskDropExprs(0x29, 0x29, expr.CmpOpEq), // XMAS (FIN|PSH|URG)
	})

	return nil
}

func addFlowOffloadRules(conn *nf.Conn, table *nf.Table, chainName, flowtableName string, lanSet *nf.Set) error {
	if chainName == "" || lanSet == nil || flowtableName == "" {
		return nil
	}

	chain := &nf.Chain{Name: chainName, Table: table}

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: flowOffloadExprs(lanSet, flowtableName, true),
	})

	conn.AddRule(&nf.Rule{
		Table: table,
		Chain: chain,
		Exprs: flowOffloadExprs(lanSet, flowtableName, false),
	})
	return nil
}
