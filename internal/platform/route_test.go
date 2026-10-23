package platform

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestIPv4RouteSourceDoesNotSendDatagrams(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for range 10 {
		source, err := IPv4RouteSource("127.0.0.1", listener.LocalAddr().(*net.UDPAddr).Port)
		if err != nil || source != "127.0.0.1" {
			t.Fatal(source, err)
		}
	}
	if err := listener.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var packet [1]byte
	_, _, err = listener.ReadFromUDP(packet[:])
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatal("route lookup sent a datagram", err)
	}
}

func TestIPv4RouteSourceRejectsInvalidDestinations(t *testing.T) {
	for _, address := range []string{"", "not-a-hostname", "2001:db8::1"} {
		if _, err := IPv4RouteSource(address, 80); err == nil {
			t.Fatal("invalid destination accepted")
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if _, err := IPv4RouteSource("127.0.0.1", port); err == nil {
			t.Fatal("invalid port accepted")
		}
	}
}
