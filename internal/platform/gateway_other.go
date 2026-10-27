//go:build !linux && !windows && !darwin

package platform

func gateway(string) string { return "" }
