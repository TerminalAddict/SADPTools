package sites

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Synthetic NX camera; URL tokens must be protected just like passwords.
const previewFixture = `{"summary":{"passwords_revealed":true},"cameras":[{"name":"Synthetic camera","ip":"192.0.2.41","vendor":"GENERIC_RTSP","username":"fixture","password":"SYNTHETIC-password","has_password":true,"credential_status":"recovered","onvif_url":"http://192.0.2.41:2020/onvif/device_service","stream_urls":{"2":"rtsp://192.0.2.41:554/sub?token=SYNTHETIC-token","1":"rtsp://embedded:discarded@192.0.2.41:554/main","3":"rtsp://192.0.2.41:554/main"}}]}`

func TestImportedEndpointsEncryptedPortableAndPrivate(t *testing.T) {
	p, err := PrepareImport([]byte(previewFixture), "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Clear()
	if !p.Preview().NeedsVault || p.Preview().Attachments != 1 {
		t.Fatal("endpoint import omitted")
	}
	s := testStore(t)
	if err = s.Unlock("Synthetic source passphrase"); err != nil {
		t.Fatal(err)
	}
	site, err := s.ImportSite(p, "Synthetic source")
	if err != nil {
		t.Fatal(err)
	}
	check := func(s *Store, site Site) Record {
		t.Helper()
		rows, _ := s.Records(site.ID)
		r := rows[0]
		if !s.HasPreviewEndpoints(site.ID, r.ID) {
			t.Fatal("missing endpoint metadata")
		}
		ep, err := s.ReadPreviewEndpoints(site.ID, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer ep.Clear()
		if ep.StreamCount() != 2 || ep.ONVIF() != "http://192.0.2.41:2020/onvif/device_service" {
			t.Fatal("bad endpoint import")
		}
		for _, profile := range []string{"sub", "auto", "saved:1"} {
			raw, err := ep.Stream(profile)
			if err != nil || raw != "rtsp://192.0.2.41:554/sub?token=SYNTHETIC-token" {
				t.Fatal("wrong sub stream")
			}
		}
		raw, err := ep.Stream("main")
		if err != nil || raw != "rtsp://192.0.2.41:554/main" {
			t.Fatal("wrong main stream/userinfo retained")
		}
		for _, profile := range []string{"saved:-1", "saved:2", "saved:x", "1"} {
			if _, err = ep.Stream(profile); err == nil {
				t.Fatal("invalid stream selected")
			}
		}
		report, _ := s.Export(site.ID)
		metadata, _ := s.Assets(site.ID)
		for _, value := range []any{ep, r, p, p.Preview(), metadata} {
			j, _ := json.Marshal(value)
			if strings.Contains(string(j)+fmt.Sprintf("%#v", value), "SYNTHETIC-token") {
				t.Fatal("endpoint disclosed")
			}
		}
		if bytes.Contains(report, []byte("SYNTHETIC-token")) {
			t.Fatal("ordinary report disclosed endpoint")
		}
		var cipher []byte
		if err = s.db.QueryRow(`SELECT payload FROM site_assets WHERE site_id=?`, site.ID).Scan(&cipher); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(cipher, []byte("SYNTHETIC-token")) || bytes.Contains(cipher, []byte("rtsp://")) {
			t.Fatal("plaintext endpoint in database")
		}
		return r
	}
	check(s, site)
	backup, err := s.Backup(site.ID, "Synthetic transport passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backup, []byte("SYNTHETIC-token")) {
		t.Fatal("plaintext endpoint in site export")
	}
	destination := testStore(t)
	if err = destination.Unlock("Synthetic different laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	restored, err := destination.Restore(backup, "Synthetic transport passphrase")
	if err != nil {
		t.Fatal(err)
	}
	r := check(destination, restored)
	destination.Lock()
	if !destination.HasPreviewEndpoints(restored.ID, r.ID) {
		t.Fatal("metadata needs vault")
	}
	if _, err = destination.ReadPreviewEndpoints(restored.ID, r.ID); err == nil {
		t.Fatal("locked endpoint read")
	}
	if err = destination.Unlock("Synthetic different laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	r.Device.IPv4 = "192.0.2.42"
	if _, err = destination.SaveRecord(r); err != nil {
		t.Fatal(err)
	}
	if _, err = destination.ReadPreviewEndpoints(restored.ID, r.ID); err == nil {
		t.Fatal("stale endpoint accepted")
	}
}

func TestNXRejectsForeignStreamDestinationsWithoutDroppingCamera(t *testing.T) {
	for _, endpoint := range []string{"rtsp://192.0.2.42/main", "rtsp://example.invalid/main", "file:///secret", "http://192.0.2.41/snapshot", "rtsp://192.0.2.41:99999/main"} {
		raw, _ := json.Marshal(map[string]any{"summary": map[string]any{}, "cameras": []any{map[string]any{"ip": "192.0.2.41", "stream_urls": map[string]string{"1": endpoint}}}})
		p, err := PrepareImport(raw, "")
		if err != nil {
			t.Fatal(err)
		}
		v := p.Preview()
		p.Clear()
		if v.Devices != 1 || v.Attachments != 0 || len(v.Warnings) < 2 || strings.Contains(strings.Join(v.Warnings, " "), endpoint) {
			t.Fatal("unsafe URL accepted or disclosed")
		}
	}
}

func TestProtectedEndpointsRejectMalformedOrUnencryptedBackups(t *testing.T) {
	for _, mutate := range []func(*backupData){
		func(b *backupData) { b.Assets[0].Meta.Encrypted = false },
		func(b *backupData) { b.Assets[0].Meta.DeviceID = "" },
		func(b *backupData) {
			b.Assets[0].Payload = []byte(`{"version":1,"address":"192.0.2.41","rtsp":["rtsp://192.0.2.42/other"]}`)
		},
	} {
		b, _, err := decodeNX([]byte(previewFixture))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&b)
		s := testStore(t)
		if err = s.Unlock("Synthetic test passphrase"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.restoreBackup(b, "Unsafe"); err == nil {
			t.Fatal("invalid endpoints restored")
		}
		if list, _ := s.Sites(); len(list) != 0 {
			t.Fatal("failed import partially committed")
		}
	}
}
