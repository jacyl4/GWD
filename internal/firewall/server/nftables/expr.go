package nftables

import (
	nf "github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

func ctStateDropExprs(bit uint32) []expr.Any {
	mask := binaryutil.NativeEndian.PutUint32(bit)
	return []expr.Any{
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           mask,
			Xor:            binaryutil.NativeEndian.PutUint32(0),
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     zeroBytes(4),
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	}
}

func tcpNewWithoutSynExprs() []expr.Any {
	maskSyn := []byte{0x02}

	exprs := append(protoTCPMatchExprs(),
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            4,
			Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitNEW),
			Xor:            binaryutil.NativeEndian.PutUint32(0),
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     zeroBytes(4),
		},
		loadTCPFlags(),
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            1,
			Mask:           maskSyn,
			Xor:            []byte{0x00},
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: 1,
			Data:     maskSyn,
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	)

	return exprs
}

func tcpFlagMaskDropExprs(mask, expect byte, op expr.CmpOp) []expr.Any {
	exprs := append(protoTCPMatchExprs(),
		loadTCPFlags(),
		&expr.Bitwise{
			SourceRegister: 1,
			DestRegister:   1,
			Len:            1,
			Mask:           []byte{mask},
			Xor:            []byte{0x00},
		},
		&expr.Cmp{
			Op:       op,
			Register: 1,
			Data:     []byte{expect},
		},
		&expr.Verdict{Kind: expr.VerdictDrop},
	)

	return exprs
}

func flowOffloadExprs(lanSet *nf.Set, flowtableName string, matchSrc bool) []expr.Any {
	offset := uint32(12) // IPv4 source
	if !matchSrc {
		offset = 16 // IPv4 destination
	}

	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     binaryutil.NativeEndian.PutUint32(uint32(unix.NFPROTO_IPV4)),
		},
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       offset,
			Len:          4,
		},
		&expr.Lookup{
			SourceRegister: 1,
			SetName:        lanSet.Name,
			SetID:          lanSet.ID,
		},
		&expr.FlowOffload{Name: flowtableName},
	}
}

func protoTCPMatchExprs() []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     binaryutil.NativeEndian.PutUint32(uint32(unix.IPPROTO_TCP)),
		},
	}
}

func loadTCPFlags() expr.Any {
	return &expr.Payload{
		DestRegister: 1,
		Base:         expr.PayloadBaseTransportHeader,
		Offset:       13,
		Len:          1,
	}
}
