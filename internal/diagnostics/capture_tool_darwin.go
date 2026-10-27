//go:build darwin

package diagnostics

import "os/exec"

func dumpcapPath() (string, error) {
	if path, err := exec.LookPath("dumpcap"); err == nil {
		return path, nil
	}
	// Finder's PATH normally omits Homebrew and application bundles. Only probe
	// fixed installation locations; never install or elevate the capture helper.
	for _, path := range []string{"/Applications/Wireshark.app/Contents/MacOS/dumpcap", "/opt/homebrew/bin/dumpcap", "/usr/local/bin/dumpcap"} {
		if found, err := exec.LookPath(path); err == nil {
			return found, nil
		}
	}
	return "", exec.ErrNotFound
}
