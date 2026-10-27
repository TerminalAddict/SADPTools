//go:build windows

package platform

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

// CaptureAdapter maps the selected Windows friendly name to Npcap's device name.
func CaptureAdapter(name string) (string, error) {
	size := uint32(16384)
	for range 3 {
		buf := make([]byte, size)
		a := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		e := windows.GetAdaptersAddresses(windows.AF_INET, 0, 0, a, &size)
		if e == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if e != nil {
			return "", errors.New("cannot enumerate Windows capture adapter identifiers")
		}
		for p := a; p != nil; p = p.Next {
			if windows.UTF16PtrToString(p.FriendlyName) == name {
				return `\Device\NPF_` + windows.BytePtrToString(p.AdapterName), nil
			}
		}
		break
	}
	return "", errors.New("capture adapter identifier unavailable")
}
