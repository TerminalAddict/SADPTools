package platform

import "encoding/binary"

// Decode Windows ancillary data on any host so both ABI layouts can be tested.
func windowsInterfaceIndex(oob []byte, word int) int {
	if word != 4 && word != 8 {
		return 0
	}
	// WSACMSGHDR: SIZE_T cmsg_len; INT cmsg_level; INT cmsg_type.
	header := word + 8
	for len(oob) >= header {
		var length uint64
		if word == 8 {
			length = binary.LittleEndian.Uint64(oob)
		} else {
			length = uint64(binary.LittleEndian.Uint32(oob))
		}
		if length < uint64(header) || length > uint64(len(oob)) {
			return 0
		}
		level := binary.LittleEndian.Uint32(oob[word:])
		kind := binary.LittleEndian.Uint32(oob[word+4:])
		if level == 0 && kind == 19 && length >= uint64(header+8) {
			// IN_PKTINFO: IN_ADDR ipi_addr; ULONG ipi_ifindex.
			return int(binary.LittleEndian.Uint32(oob[header+4:]))
		}
		step := (int(length) + word - 1) & ^(word - 1)
		if step > len(oob) {
			break
		}
		oob = oob[step:]
	}
	return 0
}
