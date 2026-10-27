//go:build !windows

package platform

func CaptureAdapter(name string) (string, error) { return name, nil }
