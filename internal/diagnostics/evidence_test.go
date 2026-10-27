package diagnostics

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"strings"
	"testing"
)

func TestScrubbingUntrustedXMLAndJSON(t *testing.T) {
	data := []byte(`<Envelope><Password>SENTINEL1</Password><SimpleItem Name="Password" Value="SENTINEL2"/><Camera token="SENTINEL3"><Text>literal-SENTINEL4</Text><URI>rtsp://user:SENTINEL5@192.0.2.2/live</URI></Camera><PlateNumber>SENTINEL6</PlateNumber></Envelope>`)
	out, e := ScrubXML(data, "SENTINEL4")
	if e != nil || strings.Contains(out, "SENTINEL") {
		t.Fatal(out, e)
	}
	for _, input := range []string{`<!DOCTYPE x><x/>`, `<x>`, `<x/><y/>`} {
		if raw, e := ScrubXML([]byte(input)); e == nil || raw != "" {
			t.Fatal("unsafe malformed payload", input)
		}
	}
	b := SafeJSON(map[string]any{"password": "BAD", "plate": "BAD", "url": "https://u:BAD@host/", "nested": map[string]any{"privateKey": "BAD"}})
	if bytes.Contains(b, []byte("BAD")) {
		t.Fatal(string(b))
	}
}
func TestBundleExactPreviewChecksumsAndEncryption(t *testing.T) {
	b, e := PrepareBundle(map[string][]byte{"test.json": []byte(`{"password":"SECRET","outcome":"not-tested"}`)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.Export("wrong", ""); e == nil {
		t.Fatal("unreviewed export")
	}
	raw, e := b.Export(b.Digest, "")
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil || len(z.File) != 3 {
		t.Fatal(e)
	}
	for _, f := range z.File {
		r, _ := f.Open()
		data, _ := io.ReadAll(r)
		r.Close()
		if bytes.Contains(data, []byte("SECRET")) {
			t.Fatal("secret leaked")
		}
		if f.Name == "test.json" && string(data) != b.Preview[f.Name] {
			t.Fatal("preview differs")
		}
	}
	encrypted, e := b.Export(b.Digest, "Synthetic-passphrase")
	if e != nil {
		t.Fatal(e)
	}
	plain, e := vault.DecryptBackup(encrypted, "Synthetic-passphrase")
	if e != nil || !bytes.Equal(plain, raw) {
		t.Fatal("encrypted bundle mismatch", e)
	}
	b.Preview["test.json"] = "{}"
	if _, e = b.Export(b.Digest, ""); e == nil {
		t.Fatal("changed preview accepted")
	}
}
func TestLabelsOpaqueRoundTripAndEscaping(t *testing.T) {
	site, id := strings.Repeat("a", 32), strings.Repeat("b", 32)
	code, e := AssetCode(site, id)
	if e != nil {
		t.Fatal(e)
	}
	s, i, e := ParseAssetCode(code)
	if e != nil || s != site || i != id {
		t.Fatal(s, i, e)
	}
	svg, e := LabelSVG(Label{SiteID: site, AssetID: id, Lines: []string{"Synthetic site", "<camera>"}, WidthMM: 75, HeightMM: 35, Copies: 4})
	if e != nil || bytes.Contains(svg, []byte("<camera>")) || !bytes.Contains(svg, []byte("150mm")) {
		t.Fatal(string(svg), e)
	}
	if _, e = LabelSVG(Label{SiteID: site, AssetID: id, Lines: []string{"password=SECRET"}, WidthMM: 75, HeightMM: 35, Copies: 1}); e == nil {
		t.Fatal("secret label accepted")
	}
	if _, _, e = ParseAssetCode("https://example.invalid"); e == nil {
		t.Fatal("URL accepted")
	}
}
func TestFirmwareHashProvenanceAndCompatibility(t *testing.T) {
	f := Firmware{Vendor: "Synthetic", Model: "Camera", SourceURL: "https://example.invalid/official", Region: "NZ", Hardware: "A", Platform: "P"}
	got, e := HashFirmware(context.Background(), strings.NewReader("Synthetic firmware fixture"), f)
	if e != nil || len(got.SHA256) != 64 || !strings.Contains(got.Signature, "unknown") {
		t.Fatal(got, e)
	}
	got.Bytes = 0
	got.ExpectedSHA256 = strings.Repeat("0", 64)
	if _, e = HashFirmware(context.Background(), strings.NewReader("changed"), got); e == nil {
		t.Fatal("hash mismatch accepted")
	}
	r := MatchFirmware(f, FirmwareIdentity{Model: "Camera"})
	if r.State != "unable-to-verify" {
		t.Fatal(r)
	}
	r = MatchFirmware(f, FirmwareIdentity{Vendor: "Synthetic", Model: "Other"})
	if r.State != "blocked" {
		t.Fatal(r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = HashFirmware(ctx, strings.NewReader("fixture"), f); e == nil {
		t.Fatal("cancellation ignored")
	}
	if _, e = json.Marshal(r); e != nil {
		t.Fatal(e)
	}
}

func TestTraceScopeRequiresConsentAndBounds(t *testing.T) {
	base := TraceOptions{Interface: "Synthetic adapter", Target: "192.0.2.3", Seconds: 10, Approved: true}
	args, e := TraceArguments(base, "synthetic.pcapng")
	if e != nil || !strings.Contains(strings.Join(args, "|"), "host 192.0.2.3") || !strings.Contains(strings.Join(args, "|"), "filesize:8192") {
		t.Fatal(args, e)
	}
	for _, change := range []func(*TraceOptions){func(o *TraceOptions) { o.Approved = false }, func(o *TraceOptions) { o.Seconds = 61 }, func(o *TraceOptions) { o.Target = "192.0.2.0/24" }, func(o *TraceOptions) { o.Interface = "" }} {
		o := base
		change(&o)
		if _, e := TraceArguments(o, "fixture"); e == nil {
			t.Fatal("unsafe trace scope accepted")
		}
	}
}

func TestProtocolNamespacesAndEndpointsDoNotExposeCredentials(t *testing.T) {
	raw := []byte(`<Envelope xmlns="http://user:SECRET@bad.invalid/private"><Service><Namespace>http://www.onvif.org/ver10/media/wsdl</Namespace><XAddr>http://192.0.2.3:8080/onvif/media</XAddr></Service><Service><Namespace>http://evil.invalid/SECRET</Namespace><XAddr>http://user:SECRET@192.0.2.4/relay</XAddr></Service></Envelope>`)
	safe, e := ScrubXML(raw, "SECRET")
	if e != nil || strings.Contains(safe, "SECRET") || !strings.Contains(safe, "http://www.onvif.org/ver10/media/wsdl") {
		t.Fatal(safe, e)
	}
	endpoints := ServiceEndpoints(raw, "192.0.2.3", "SECRET")
	if len(endpoints) != 2 || endpoints[0].Authority != "192.0.2.3:8080" || endpoints[1].Authority != "" || endpoints[1].Namespace != "" {
		t.Fatal(endpoints)
	}
}

func TestProtocolAndBundleRemoveSessionTokensAndPlateAttributes(t *testing.T) {
	xml := `<Event><SimpleItem Name="LicensePlate" Value="SENTINEL-PLATE"/><sessionID>SENTINEL-SESSION</sessionID><Nonce>SENTINEL-NONCE</Nonce><Challenge>SENTINEL-CHALLENGE</Challenge><Cookie>SENTINEL-COOKIE</Cookie></Event>`
	out, e := ScrubXML([]byte(xml))
	if e != nil || strings.Contains(out, "SENTINEL") {
		t.Fatal(out, e)
	}
	raw := SafeJSON(map[string]any{"sessionId": "SENTINEL-SESSION", "headers": map[string]string{"Cookie": "SENTINEL-COOKIE"}, "xml": xml})
	if bytes.Contains(raw, []byte("SENTINEL")) {
		t.Fatal(string(raw))
	}
}
