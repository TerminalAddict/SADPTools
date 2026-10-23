//go:build !linux && !windows

package platform

func gateway(string) string { return "" }
