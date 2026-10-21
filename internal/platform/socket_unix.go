//go:build !windows

package platform

import "golang.org/x/net/ipv4"

func (s *udpSocket) enableInfo() error { return s.packet.SetControlMessage(ipv4.FlagInterface, true) }
func (s *udpSocket) Read(b []byte) (int, PacketInfo, error) {
	n, cm, source, err := s.packet.ReadFrom(b)
	info := PacketInfo{}
	if cm != nil {
		info.InterfaceIndex = cm.IfIndex
	}
	if source != nil {
		info.Source = source.String()
	}
	return n, info, err
}
