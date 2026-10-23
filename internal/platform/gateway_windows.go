//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func gateway(name string) string {
	size := uint32(16 * 1024)
	for range 3 {
		buf := make([]byte, size)
		a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		e := windows.GetAdaptersAddresses(windows.AF_INET, 0x0080, 0, a, &size)
		if e == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if e != nil {
			return ""
		}
		for p := a; p != nil; p = p.Next {
			if windows.UTF16PtrToString(p.FriendlyName) != name {
				continue
			}
			for g := p.FirstGatewayAddress; g != nil; g = g.Next {
				ip := g.Address.IP()
				if ip.To4() != nil && !ip.IsUnspecified() {
					return ip.String()
				}
			}
		}
		return ""
	}
	return ""
}
