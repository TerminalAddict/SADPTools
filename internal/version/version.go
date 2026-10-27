package version

import "strings"

// Value can be set with go build -ldflags '-X github.com/TerminalAddict/SADPTools/internal/version.Value=...'.
// Release builds frame the linker string so packaging can identify this exact
// value in stripped executables without relying on Go's alignment padding.
var Value = "1.0.0"

func init() { Value = displayVersion(Value) }

func displayVersion(value string) string {
	const prefix, suffix = "SADPToolsVersion[", "]"
	if strings.HasPrefix(value, prefix) && strings.HasSuffix(value, suffix) {
		return strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
	}
	return value
}
