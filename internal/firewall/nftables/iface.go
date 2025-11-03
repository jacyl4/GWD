package nftables

func encodeInterfaceName(name string) []byte {
	var buf [ifNameSize]byte
	copy(buf[:], name)
	return buf[:]
}
