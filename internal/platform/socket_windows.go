package platform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// x/net/ipv4 does not implement Windows receive control messages. Go's
// UDPConn.ReadMsgUDP uses WSARecvMsg; enable IP_PKTINFO and decode IN_PKTINFO.
func (s *udpSocket) enableInfo() error {
	raw, err := s.conn.SyscallConn()
	if err != nil {
		return err
	}
	var optionErr error
	err = raw.Control(func(fd uintptr) {
		optionErr = windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, windows.IP_PKTINFO, 1)
	})
	if err != nil {
		return err
	}
	return optionErr
}
func (s *udpSocket) Read(b []byte) (int, PacketInfo, error) {
	oob := make([]byte, 128)
	n, nn, flags, addr, err := s.conn.ReadMsgUDP(b, oob)
	info := PacketInfo{}
	if addr != nil {
		info.Source = addr.String()
	}
	if err != nil {
		return n, info, err
	}
	if flags&windows.MSG_CTRUNC != 0 {
		return n, info, fmt.Errorf("truncated Windows packet metadata")
	}
	info.InterfaceIndex = windowsInterfaceIndex(oob[:nn], int(unsafe.Sizeof(uintptr(0))))
	return n, info, nil
}
