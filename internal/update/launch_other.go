//go:build !windows

package update

import "errors"

func launchInstaller(string) error {
	return errors.New("in-app installation is currently available on Windows AMD64 only")
}
