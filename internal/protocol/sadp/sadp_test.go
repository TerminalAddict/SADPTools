package sadp

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

func TestParseFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/inquiry.xml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Device
	if r.UUID != "11111111-2222-4333-8444-555555555555" || d.MAC != "02:AA:BB:CC:DD:01" || d.Model != "TEST-CAMERA" || d.IPv4 != "192.0.2.50" || d.Serial != "SYNTHETIC-SERIAL-001" || d.Firmware != "TEST-1.0" || d.Name != "Test & fixture" {
		t.Fatalf("unexpected reply: %+v", r)
	}
	if d.Activated == nil || *d.Activated || d.DHCP == nil || !*d.DHCP || d.HTTPPort == nil || *d.HTTPPort != 8080 {
		t.Fatalf("bad optional fields: %+v", d)
	}
}
func TestInvalidReplies(t *testing.T) {
	for name, input := range map[string]string{
		"empty": "", "malformed": "<ProbeMatch>", "probe": "<Probe><Uuid>x</Uuid></Probe>", "identity missing": "<ProbeMatch><IPv4Address>192.0.2.1</IPv4Address></ProbeMatch>",
		"invalid MAC": "<ProbeMatch><MAC>garbage</MAC></ProbeMatch>", "multicast MAC": "<ProbeMatch><MAC>ff:ff:ff:ff:ff:ff</MAC></ProbeMatch>",
		"zero MAC": "<ProbeMatch><MAC>00:00:00:00:00:00</MAC></ProbeMatch>", "trailing root": "<ProbeMatch><DeviceSN>test</DeviceSN></ProbeMatch><extra/>",
		"trailing text": "<ProbeMatch><DeviceSN>test</DeviceSN></ProbeMatch>garbage", "entity": "<!DOCTYPE x [<!ENTITY x SYSTEM 'file:///etc/passwd'>]><ProbeMatch><DeviceSN>&x;</DeviceSN></ProbeMatch>",
		"wrong type": "<ProbeMatch><DeviceSN>test</DeviceSN><Types>activate</Types></ProbeMatch>", "oversize": strings.Repeat("x", MaxPacketSize+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
func TestIncompleteAndUnknownFields(t *testing.T) {
	input := `<ProbeMatch xmlns="urn:test"><DeviceSN> synthetic </DeviceSN><IPv4Address>bad</IPv4Address><Activated>maybe</Activated><DHCP>unsupported</DHCP><HttpPort>99999</HttpPort></ProbeMatch>`
	r, err := Parse(append([]byte{0xef, 0xbb, 0xbf}, append([]byte(input), 0, 0)...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Device.ID != "serial:synthetic" || r.Device.Activated != nil || r.Device.DHCP != nil || r.Device.HTTPPort != nil || r.Device.IPv4 != "" || r.Device.Model != "" {
		t.Fatalf("fabricated values: %+v", r.Device)
	}
}
func TestProbes(t *testing.T) {
	uuid, err := NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(uuid) != 36 || uuid[14] != '4' {
		t.Fatal(uuid)
	}
	packets, err := Probes(uuid)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range packets {
		var p probe
		if err := xml.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		want := []string{"inquiry", "inquiry_v32"}[i]
		if p.UUID != uuid || p.Types != want {
			t.Fatal(p)
		}
	}
	if _, err := Probes(""); err == nil {
		t.Fatal("empty UUID accepted")
	}
}
func FuzzParse(f *testing.F) {
	b, _ := os.ReadFile("testdata/inquiry.xml")
	f.Add(b)
	f.Add([]byte("<ProbeMatch/>"))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := Parse(b)
		if err == nil && r.Device.Key() == "" {
			t.Fatal("accepted unidentified device")
		}
	})
}
