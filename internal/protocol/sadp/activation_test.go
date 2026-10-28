package sadp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"
)

// Synthetic identities, passwords and challenges only. The fixed vector was
// independently checked against AES-128-ECB and the SADP protocol reference.
func TestActivationCryptoWireVector(t *testing.T) {
	e, err := NewActivationExchange()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	probe, err := e.Probe("synthetic-id", "02:00:00:00:00:01")
	if err != nil {
		t.Fatal(err)
	}
	var wire struct{ MAC, Types, Code string }
	if xml.Unmarshal(probe, &wire) != nil || wire.MAC != "02-00-00-00-00-01" || wire.Types != "exchangecode" {
		t.Fatal("bad exchange envelope")
	}
	der, err := base64.StdEncoding.DecodeString(wire.Code)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.ParsePKCS1PublicKey(der)
	if err != nil || pub.N.BitLen() != 1024 || pub.E != 65537 {
		t.Fatal("bad RSA public key")
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := e.EncryptPassword(base64.StdEncoding.EncodeToString(ciphertext), "Synthetic<&>1")
	if err != nil || encrypted != "cnJ+iB7c/QEApxhoeQm1ZQG//N7uff8FO6nRLrIBzNQ=" {
		t.Fatal("SADP AES vector mismatch", err)
	}
	command, err := ActivationCommand("synthetic-write", wire.MAC, encrypted)
	if err != nil || strings.Contains(string(command), "Synthetic") || !strings.Contains(string(command), encrypted) {
		t.Fatal("bad activation envelope")
	}
	e.Close()
	if _, err = e.Probe("id", wire.MAC); err == nil {
		t.Fatal("closed exchange reused")
	}
	if _, err = e.EncryptPassword(base64.StdEncoding.EncodeToString(ciphertext), "Synthetic<&>1"); err == nil {
		t.Fatal("closed key reused")
	}
}

func TestActivationRejectsUnsupportedCryptoWithoutEchoing(t *testing.T) {
	e, err := NewActivationExchange()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, challenge := range []string{"short", strings.Repeat("z", 32), strings.Repeat("a", 31), strings.Repeat("a", 33)} {
		cipher, err := rsa.EncryptPKCS1v15(rand.Reader, &e.key.PublicKey, []byte(challenge))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.EncryptPassword(base64.StdEncoding.EncodeToString(cipher), "Synthetic1"); err == nil || strings.Contains(err.Error(), challenge) {
			t.Fatal("bad challenge accepted or exposed")
		}
	}
	for _, code := range []string{"", "secret-not-base64", base64.StdEncoding.EncodeToString(make([]byte, 128)), strings.Repeat("A", 2048)} {
		if _, err = e.EncryptPassword(code, "Synthetic1"); err == nil {
			t.Fatal("bad RSA reply accepted")
		}
	}
	cipher, err := rsa.EncryptPKCS1v15(rand.Reader, &e.key.PublicKey, []byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"short", strings.Repeat("x", 17), strings.Repeat("é", 9)} {
		if _, err = e.EncryptPassword(base64.StdEncoding.EncodeToString(cipher), password); err == nil {
			t.Fatal("password truncated")
		}
	}
	for _, password := range []string{"12345678", "1234567890123456", "éééé1234"} {
		got, err := e.EncryptPassword(base64.StdEncoding.EncodeToString(cipher), password)
		if err != nil || len(got) != 44 {
			t.Fatal("valid password length rejected")
		}
	}
	if _, err = ActivationCommand("id", "02:00:00:00:00:01", "Plain-secret1"); err == nil {
		t.Fatal("plaintext accepted as ciphertext")
	}
}
