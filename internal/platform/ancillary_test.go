package platform

import (
	"encoding/binary"
	"testing"
)

func TestWindowsPacketInfoLayouts(t *testing.T) {
	for _, word := range []int{4, 8} {
		header := word + 8
		packet := make([]byte, header+8)
		if word == 8 {
			binary.LittleEndian.PutUint64(packet, uint64(len(packet)))
		} else {
			binary.LittleEndian.PutUint32(packet, uint32(len(packet)))
		}
		binary.LittleEndian.PutUint32(packet[word+4:], 19)
		binary.LittleEndian.PutUint32(packet[header+4:], 42)
		if got := windowsInterfaceIndex(packet, word); got != 42 {
			t.Fatalf("word=%d index=%d", word, got)
		}
		for n := 0; n < len(packet); n++ {
			if got := windowsInterfaceIndex(packet[:n], word); got != 0 {
				t.Fatalf("truncated packet accepted: %d", n)
			}
		}
		unknown := append([]byte{}, packet...)
		binary.LittleEndian.PutUint32(unknown[word+4:], 99)
		if got := windowsInterfaceIndex(append(unknown, packet...), word); got != 42 {
			t.Fatalf("second cmsg index=%d", got)
		}
		for n := 0; n < word; n++ {
			packet[n] = 255
		}
		if got := windowsInterfaceIndex(packet, word); got != 0 {
			t.Fatal("overflow accepted")
		}
	}
}
