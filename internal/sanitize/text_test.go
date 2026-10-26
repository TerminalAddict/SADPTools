package sanitize

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundedUnicodeAndSecretRedaction(t *testing.T) {
	for _, text := range []string{Text(strings.Repeat("māori", 4000)), Label(strings.Repeat("庭園", 200))} {
		if !utf8.ValidString(text) {
			t.Fatal("truncation split a Unicode character")
		}
	}
	if len(Text(strings.Repeat("ā", 5000))) > 8192 || len(Label(strings.Repeat("庭", 200))) > 256 {
		t.Fatal("size bound exceeded")
	}
	result := Text("camera password=SYNTHETIC-SECRET rtsp://admin:SECRET@camera/video\x00")
	if strings.Contains(result, "SECRET") || strings.Contains(result, "\x00") || strings.Contains(result, "rtsp://") {
		t.Fatal("sensitive or control text retained")
	}
}
