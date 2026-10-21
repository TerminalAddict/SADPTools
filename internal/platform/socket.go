package platform

import (
	"fmt"
	"net"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
	"golang.org/x/net/ipv4"
)

type PacketInfo struct {
	InterfaceIndex int
	Source         string
}

type Transport interface {
	Join(Adapter) error
	Send(Adapter, []byte) error
	Read([]byte) (int, PacketInfo, error)
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	Close() error
}

type udpSocket struct {
	conn   *net.UDPConn
	packet *ipv4.PacketConn
}

func Open() (Transport, error) {
	// No address/port reuse: another scanner must not silently steal our replies.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: sadp.Port})
	if err != nil {
		return nil, fmt.Errorf("bind UDP/%d (close other SADP scanners and check local firewall rules): %w", sadp.Port, err)
	}
	s := &udpSocket{conn: conn, packet: ipv4.NewPacketConn(conn)}
	if err = s.enableInfo(); err == nil {
		err = s.packet.SetMulticastTTL(1)
	}
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("configure multicast socket: %w", err)
	}
	return s, nil
}

func (s *udpSocket) Join(a Adapter) error {
	iface, err := net.InterfaceByIndex(a.Index)
	if err != nil {
		return err
	}
	if iface.Name != a.Name {
		return fmt.Errorf("adapter changed since enumeration")
	}
	return s.packet.JoinGroup(iface, &net.UDPAddr{IP: net.ParseIP(sadp.MulticastAddress)})
}

func (s *udpSocket) Send(a Adapter, b []byte) error {
	// All sends occur on the scan goroutine; changing the outgoing adapter is serialized.
	iface, err := net.InterfaceByIndex(a.Index)
	if err != nil {
		return err
	}
	if iface.Name != a.Name {
		return fmt.Errorf("adapter changed since enumeration")
	}
	if err = s.packet.SetMulticastInterface(iface); err != nil {
		return err
	}
	_, err = s.conn.WriteToUDP(b, &net.UDPAddr{IP: net.ParseIP(sadp.MulticastAddress), Port: sadp.Port})
	return err
}
func (s *udpSocket) SetDeadline(t time.Time) error     { return s.conn.SetDeadline(t) }
func (s *udpSocket) SetReadDeadline(t time.Time) error { return s.conn.SetReadDeadline(t) }
func (s *udpSocket) Close() error                      { return s.conn.Close() }
