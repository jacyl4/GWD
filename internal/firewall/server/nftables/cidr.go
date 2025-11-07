package nftables

import (
	"net"
	"strings"

	apperrors "GWD/internal/errors"
	nf "github.com/google/nftables"
)

func cidrToElements(cidrs []string) ([]nf.SetElement, error) {
	if len(cidrs) == 0 {
		return nil, nil
	}

	var elems []nf.SetElement
	for _, cidr := range cidrs {
		trimmed := strings.TrimSpace(cidr)
		if trimmed == "" {
			continue
		}

		_, network, err := net.ParseCIDR(trimmed)
		if err != nil {
			return nil, firewallError("nftables.cidrToElements.parse", "invalid CIDR provided", err, apperrors.Metadata{
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
		return nil, nil, firewallError("nftables.cidrRange", "unsupported address family for network", nil, apperrors.Metadata{
			"network": n.String(),
		})
	}

	last := broadcastIP(n)
	if last == nil {
		return nil, nil, firewallError("nftables.cidrRange", "failed to compute broadcast address", nil, apperrors.Metadata{
			"network": n.String(),
		})
	}

	end := incrementIP(last)
	if end == nil {
		return nil, nil, firewallError("nftables.cidrRange", "CIDR overflowed maximum address", nil, apperrors.Metadata{
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
