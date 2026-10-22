// Package update checks public GitHub releases and stages verified installers.
// It has no dependency on Wails or device networking.
package update

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(dev|beta|rc)(?:\.([1-9][0-9]*))?)?$`)

type version struct {
	numbers [5]int
	channel string
}

func parseVersion(value string) (version, error) {
	var v version
	m := versionPattern.FindStringSubmatch(strings.TrimPrefix(value, "v"))
	if m == nil {
		return v, fmt.Errorf("unsupported version %q", value)
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil || n > 65535 {
			return v, fmt.Errorf("invalid version number")
		}
		v.numbers[i] = n
	}
	v.channel = m[4]
	v.numbers[3] = map[string]int{"dev": 0, "beta": 1, "rc": 2, "": 3}[v.channel]
	if v.channel == "beta" || v.channel == "rc" {
		n, err := strconv.Atoi(m[5])
		if err != nil || n < 1 || n > 19999 {
			return v, fmt.Errorf("invalid prerelease sequence")
		}
		v.numbers[4] = n
	} else if m[5] != "" {
		return v, fmt.Errorf("invalid development version")
	}
	return v, nil
}

func (v version) newer(other version) bool {
	for i, n := range v.numbers {
		if n != other.numbers[i] {
			return n > other.numbers[i]
		}
	}
	return false
}

func DefaultChannel(current string) string {
	v, err := parseVersion(current)
	if err == nil && v.channel != "" {
		return "beta"
	}
	return "stable"
}
