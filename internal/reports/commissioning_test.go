package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestOfflineLargeUnicodeReportWithImages(t *testing.T) {
	s := Snapshot{Version: 1, At: "2026-01-01T00:00:00Z", Title: "Synthetic commissioning — Māori café", Site: "Fixture site", Company: "Synthetic installer", Date: "2026-01-01", Summary: "Synthetic evidence, no live device tests", Sections: []Section{{Title: "Commissioning matrix", Headers: []string{"Camera", "Check", "Result", "Provenance"}, Rows: [][]string{}}}, Figures: []Figure{{ImageID: "fixture-image", Caption: "Synthetic image", Markers: []installation.Marker{{ID: "m", Label: "Camera ā", X: .5, Y: .5}}}}}
	for i := 0; i < 200; i++ {
		s.Sections[0].Rows = append(s.Sections[0].Rows, []string{fmt.Sprintf("Camera %d", i), "Live video", "not-tested", "manual"})
	}
	var imageData bytes.Buffer
	png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 80, 40)))
	load := func(string) ([]byte, error) { return imageData.Bytes(), nil }
	pdf, e := Commissioning("pdf", s, load)
	if e != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal(e)
	}
	if bytes.Count(pdf, []byte("/Type /Page")) < 5 {
		t.Fatal("large report did not paginate")
	}
	again, e := Commissioning("pdf", s, load)
	if e != nil || !bytes.Equal(pdf, again) {
		t.Fatal("unchanged PDF snapshot is not deterministic", e)
	}
	html, e := Commissioning("html", s, load)
	if e != nil || !strings.Contains(string(html), "Māori") || !strings.Contains(string(html), "data:image/png;base64,") || strings.Contains(string(html), "ZgotmplZ") {
		t.Fatal("image or Unicode rendering failed", e)
	}
	for _, f := range []string{"json", "csv"} {
		if _, e = Commissioning(f, s, load); e != nil {
			t.Fatal(e)
		}
	}
}
func TestEvidenceRedactionAndEscaping(t *testing.T) {
	raw := []byte(`{"password":"NEVER-EXPORT","token":"TOKEN-SECRET","streamURL":"rtsp://fixture:LOGIN-SECRET@192.0.2.2/stream?token=SECRET","serial":"SERIAL-UNIQUE","ipv4":"192.0.2.2","notes":"password=NOTE-SECRET","children":[{"recoveryCode":"RECOVERY-SECRET"}]}`)
	text, e := CleanJSON(raw, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"NEVER-EXPORT", "TOKEN-SECRET", "LOGIN-SECRET", "SERIAL-UNIQUE", "192.0.2.2", "NOTE-SECRET", "RECOVERY-SECRET"} {
		if strings.Contains(text, secret) {
			t.Fatal("secret or identity leaked", secret, text)
		}
	}
	s := Snapshot{Version: 1, At: "2026-01-01T00:00:00Z", Title: "<script>fixture</script>", Summary: "<img src=x onerror=alert(1)>"}
	html, e := Commissioning("html", s, nil)
	if e != nil || bytes.Contains(html, []byte("<script>")) || bytes.Contains(html, []byte("<img src=x")) {
		t.Fatal("unsafe HTML", e)
	}
	var value map[string]any
	if json.Unmarshal([]byte(text), &value) != nil {
		t.Fatal("cleaned JSON invalid")
	}
}
