//go:build windows

package vault

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

const PlatformMode = "dpapi"

func protect(data []byte, unwrap bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty protected key")
	}
	input := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output windows.DataBlob
	var err error
	if unwrap {
		err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	} else {
		err = windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	}
	if err != nil {
		return nil, errors.New("Windows could not unlock/protect this vault for the current user")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	protected := unsafe.Slice(output.Data, int(output.Size))
	defer clear(protected)
	return append([]byte{}, protected...), nil
}
func Protect(data []byte) ([]byte, error)   { return protect(data, false) }
func Unprotect(data []byte) ([]byte, error) { return protect(data, true) }
