// Package sanitize handles untrusted display text and routine evidence exports.
package sanitize

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var urls = regexp.MustCompile(`(?i)(?:rtsps?|https?)://[^\s<>"']+`)
var secrets = regexp.MustCompile(`(?i)(password|passwd|secret|token|recoverycode|securityanswer|authorization|privatekey)\s*[:=]\s*[^\s,;]+`)

func Text(s string) string {
	s = urls.ReplaceAllString(s, "[endpoint omitted]")
	s = secrets.ReplaceAllString(s, "$1=[redacted]")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	return bounded(s, 8192)
}
func Label(s string) string {
	s = Text(strings.TrimSpace(s))
	return bounded(s, 256)
}
func bounded(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}
func SensitiveKey(s string) bool {
	s = strings.ToLower(s)
	for _, k := range []string{"password", "passwd", "secret", "token", "credential", "key", "certificate", "recovery", "answer", "username", "authorization"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
