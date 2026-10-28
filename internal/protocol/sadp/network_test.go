package sadp

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// Synthetic fixture and independently checked vectors; no camera secrets/captures.
const networkID = "01234567-89ab-4cde-8fab-0123456789ab"
const networkMAC = "02:00:00:00:00:01"
const networkSalt = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const networkProfileXML = `<ProbeMatch><Uuid>` + networkID + `</Uuid><Types>inquiry</Types><MAC>` + networkMAC + `</MAC><IPv4Address>192.0.2.10</IPv4Address><Activated>true</Activated><Encrypt>true</Encrypt><Salt>` + networkSalt + `</Salt><HttpPort>8080</HttpPort><IPv6Address>fe80::200:ff:fe00:1</IPv6Address><IPv6Gateway>::</IPv6Gateway><IPv6MaskLen>64</IPv6MaskLen><SDKOverTLSPort>8443</SDKOverTLSPort></ProbeMatch>`

func TestNetworkAuthenticationIndependentVectors(t *testing.T) {
	// Compared with isolated SADP library digest/AES routines under emulation;
	// see the research note. These values do not come from this Go implementation.
	for _, tc := range []struct {
		salt    bool
		encrypt bool
		want    string
	}{
		{true, true, "HIortnuY3mwkklnPvbEUW5Wi9OHtuDEnm2wTncBx+bkuoZ5HM9LBmIrDPmxt7ruU1UTr3JtusOwMEGjra+QZvQ=="},
		{false, true, "nQ8k/N1BPlJvZvxBMms142wb87VtM80LcSt5C8CO3fmiUQUuGTkznvELQKUAsL7yS+IDRWpbS8hB/RZDK/f5FA=="},
		{true, false, "+LyvyPchNl+MXV+Pq1au+g=="},
		{false, false, "jNsyYxl2NxjKMxUhnvmyBQ=="},
	} {
		t.Run(fmt.Sprintf("salt=%t/encrypt=%t", tc.salt, tc.encrypt), func(t *testing.T) {
			fixture := networkProfileXML
			if !tc.salt {
				fixture = strings.ReplaceAll(fixture, "<Salt>"+networkSalt+"</Salt>", "")
			}
			if !tc.encrypt {
				fixture = strings.ReplaceAll(fixture, "<Encrypt>true", "<Encrypt>false")
			}
			profile, err := ParseNetworkProfile([]byte(fixture))
			if err != nil {
				t.Fatal(err)
			}
			network := NetworkSettings{IPv4: "192.0.2.20", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", CommandPort: 8123, DHCP: true}
			data, err := NetworkCommand(networkID, networkMAC, "Synthetic<&>1", network, profile)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				UUID         string `xml:"Uuid"`
				MAC          string `xml:"MAC"`
				Types        string
				PWErrorParse bool
				Password     struct {
					Value string `xml:",chardata"`
					Salt  string `xml:"bSalt,attr"`
				}
				IPv4Address, IPv4SubnetMask, IPv4Gateway           string
				CommandPort, HttpPort, SDKOverTLSPort, IPv6MaskLen int
				IPv6Address, IPv6Gateway                           string
				DHCP                                               bool
			}
			if err := xml.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Password.Value != tc.want || (got.Password.Salt == "true") != tc.salt {
				t.Fatal("incorrect authentication vector/attribute")
			}
			if got.UUID != networkID || got.MAC != "02-00-00-00-00-01" || got.Types != "update" || !got.PWErrorParse || !got.DHCP || got.IPv4Address != network.IPv4 || got.IPv4SubnetMask != network.SubnetMask || got.IPv4Gateway != network.Gateway || got.CommandPort != 8123 {
				t.Fatal("incorrect requested network settings")
			}
			if got.HttpPort != 8080 || got.SDKOverTLSPort != 8443 || got.IPv6Address != "fe80::200:ff:fe00:1" || got.IPv6Gateway != "::" || got.IPv6MaskLen != 64 {
				t.Fatal("unrelated settings changed")
			}
			if bytes.Contains(data, []byte("Synthetic")) || bytes.Contains(data, []byte(networkSalt)) {
				t.Fatal("raw authentication input on wire")
			}
			if tc.encrypt {
				other, err := NetworkCommand("11234567-89ab-4cde-8fab-0123456789ab", networkMAC, "Synthetic<&>1", network, profile)
				if err != nil || bytes.Contains(other, []byte(tc.want)) {
					t.Fatal("authentication not bound to request UUID")
				}
			}
		})
	}
}

func TestNetworkProfileRejectsUnsafeOrAmbiguousReplies(t *testing.T) {
	for _, change := range [][2]string{
		{"<Encrypt>true</Encrypt>", ""}, {"<Encrypt>true</Encrypt>", "<Encrypt>unknown</Encrypt>"},
		{"<Encrypt>true</Encrypt>", "<Encrypt>true</Encrypt><Encrypt>false</Encrypt>"},
		{"<Activated>true</Activated>", ""}, {"<Activated>true", "<Activated>false"},
		{"<HttpPort>8080</HttpPort>", ""}, {"<HttpPort>8080", "<HttpPort>65536"},
		{"<Salt>" + networkSalt, "<Salt>short"}, {networkSalt, strings.Repeat("x", 63) + "\n"},
		{"<IPv6MaskLen>64", "<IPv6MaskLen>129"}, {"<IPv6Gateway>::</IPv6Gateway>", ""},
		{"<IPv6Address>fe80::200:ff:fe00:1", "<IPv6Address>192.0.2.1"},
		{"<SDKOverTLSPort>8443", "<SDKOverTLSPort>-1"},
		{"<MAC>" + networkMAC, "<MAC>bad"}, {"<Types>inquiry", "<Types>update"},
	} {
		_, err := ParseNetworkProfile([]byte(strings.Replace(networkProfileXML, change[0], change[1], 1)))
		if err == nil {
			t.Fatalf("unsafe profile accepted: %q", change[0])
		}
		if strings.Contains(err.Error(), networkSalt) {
			t.Fatal("authentication echoed in error")
		}
	}
}

func TestNetworkProfileIsolationAndNoPlaintextBypass(t *testing.T) {
	p, err := ParseNetworkProfile([]byte(networkProfileXML))
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("test", "profile", p)
	j, _ := json.Marshal(p)
	for _, s := range []string{fmt.Sprintf("%v", p), fmt.Sprintf("%+v", p), fmt.Sprintf("%#v", p), log.String(), string(j)} {
		if strings.Contains(s, networkSalt) {
			t.Fatal("authentication material exposed")
		}
	}
	for _, tc := range []struct {
		id, mac, password string
		profile           NetworkProfile
	}{
		{networkID, networkMAC, "Synthetic<&>1", NetworkProfile{}},
		{networkID, "02:00:00:00:00:02", "Synthetic<&>1", p},
		{"bad", networkMAC, "Synthetic<&>1", p},
		{strings.Repeat("!", 36), networkMAC, "Synthetic<&>1", p},
		{networkID, networkMAC, strings.Repeat("x", 17), p},
		{networkID, networkMAC, "", p},
	} {
		if _, err := NetworkCommand(tc.id, tc.mac, tc.password, NetworkSettings{}, tc.profile); err == nil {
			t.Fatal("unsafe authentication accepted")
		}
	}
	if _, err := Command(networkID, networkMAC, UpdateNetwork, "plaintext", "", NetworkSettings{}); err == nil {
		t.Fatal("plaintext network bypass")
	}
}
