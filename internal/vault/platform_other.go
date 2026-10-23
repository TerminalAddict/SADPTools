//go:build !windows

package vault

import "errors"

const PlatformMode = "passphrase"

func Protect([]byte) ([]byte, error) {
	return nil, errors.New("use a master-passphrase vault on this platform")
}
func Unprotect([]byte) ([]byte, error) {
	return nil, errors.New("this vault is bound to another Windows account; restore a portable encrypted backup")
}
