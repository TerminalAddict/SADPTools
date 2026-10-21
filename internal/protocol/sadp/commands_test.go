package sadp

import (
	"encoding/xml"
	"strings"
	"testing"
)

// All identities, passwords and messages in these tests are synthetic.
func TestCommandWireFieldsAndEscaping(t *testing.T) {
	for _, action := range []Action{Activate, UpdateNetwork, ExchangeCode, EncryptionString, EncryptionStringV31, ResetPassword, SecurityCode} {
		t.Run(string(action), func(t *testing.T) {
			password := `Synthetic<&"'>123`
			code := `SUPPORT<&>CODE`
			data, err := Command("synthetic-id", "02:00:00:00:00:01", action, password, code, NetworkSettings{"192.0.2.10", "255.255.255.0", "192.0.2.1", 8123, true})
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				XMLName      xml.Name
				UUID         string `xml:"Uuid"`
				MAC          string `xml:"MAC"`
				Types        string `xml:"Types"`
				Password     string
				Code         string
				SecurityCode string
				IP           string `xml:"IPv4Address"`
				Port         int    `xml:"CommandPort"`
				DHCP         bool
				Parse        bool `xml:"PWErrorParse"`
			}
			if err = xml.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.XMLName.Local != "Probe" || got.UUID != "synthetic-id" || got.Types != string(action) || got.MAC != "02:00:00:00:00:01" {
				t.Fatal("wrong command envelope")
			}
			if action.ReadOnly() {
				if got.Password != "" || strings.Contains(string(data), password) {
					t.Fatal("password on read-only request")
				}
			} else if got.Password != password {
				t.Fatal("password changed by encoding")
			}
			if action == ResetPassword && got.Code != code {
				t.Fatal("lost reset code")
			}
			if action == SecurityCode && got.SecurityCode != code {
				t.Fatal("lost security code")
			}
			if action == UpdateNetwork && (!got.Parse || got.Port != 8123 || !got.DHCP || got.IP != "192.0.2.10") {
				t.Fatal("wrong network values")
			}
		})
	}
}
func TestCommandReplyRejectsAmbiguityAndDoesNotEchoInput(t *testing.T) {
	for _, data := range []string{
		``, `<ProbeMatch><MAC>A</MAC><MAC>B</MAC></ProbeMatch>`,
		`<ProbeMatch><Result>success<Code>secret</Code></Result></ProbeMatch>`,
		`<ProbeMatch/><ProbeMatch/>`, `<ProbeMatch><Password>synthetic-secret`,
		`<!DOCTYPE ProbeMatch><ProbeMatch/>`, `<ProbeMatch xmlns="unexpected"/>`,
		`<ProbeMatch><Uuid>one</Uuid><Uuid>two</Uuid></ProbeMatch>`,
	} {
		_, err := ParseCommandReply([]byte(data))
		if err == nil {
			t.Fatalf("accepted %q", data)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("input leaked in error")
		}
	}
	got, err := ParseCommandReply([]byte(`<ProbeMatch><Uuid>id</Uuid><MAC>02-00-00-00-00-01</MAC><Types>activate</Types><Result>success</Result><Password>SYNTHETIC-ECHO</Password></ProbeMatch>`))
	if err != nil || got.Result != "success" || got.UUID != "id" {
		t.Fatal(got, err)
	}
}
func TestDiscoveryRetainsManagementFields(t *testing.T) {
	r, err := Parse([]byte(`<ProbeMatch><MAC>02:00:00:00:00:01</MAC><CommandPort>8123</CommandPort><PasswordResetAbility>true</PasswordResetAbility><PasswordResetModeSecond>synthetic-mode</PasswordResetModeSecond></ProbeMatch>`))
	if err != nil || r.Device.CommandPort == nil || *r.Device.CommandPort != 8123 || r.Device.PasswordResetAbility != "true" || r.Device.PasswordResetMode != "synthetic-mode" {
		t.Fatal(r, err)
	}
}
