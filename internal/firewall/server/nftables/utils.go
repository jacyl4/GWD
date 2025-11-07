package nftables

import (
	"sort"
	"strings"
)

const ifNameSize = 16

func zeroBytes(n int) []byte {
	return make([]byte, n)
}

func encodeInterfaceName(name string) []byte {
	var buf [ifNameSize]byte
	copy(buf[:], name)
	return buf[:]
}

func uniqueStrings(values []string) []string {
	keys := sortedKeys(stringSet(values))
	if keys == nil {
		return []string{}
	}
	return keys
}

func mergeStringSets(groups ...[]string) []string {
	set := make(map[string]struct{})
	for _, group := range groups {
		for _, item := range group {
			if item == "" {
				continue
			}
			set[item] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for item := range set {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string{}, a...)
	bc := append([]string{}, b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		set[v] = struct{}{}
	}
	return set
}

func isExcludedByPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
