//go:build !darwin

package diagnostics

import "os/exec"

func dumpcapPath() (string, error) { return exec.LookPath("dumpcap") }
