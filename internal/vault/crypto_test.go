package vault

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestAuthenticatedEncryptionPurposeAndTamper(t *testing.T) {
	key := Random(32)
	plain := []byte("SYNTHETIC SECRET")
	sealed, e := Seal(key, plain, "profile:a")
	if e != nil {
		t.Fatal(e)
	}
	got, e := Open(key, sealed, "profile:a")
	if e != nil || !bytes.Equal(got, plain) {
		t.Fatal(e)
	}
	if _, e = Open(key, sealed, "profile:b"); e == nil {
		t.Fatal("ciphertext moved between profiles")
	}
	sealed[len(sealed)-1] ^= 1
	if _, e = Open(key, sealed, "profile:a"); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestSecretsCannotSerialize(t *testing.T) {
	s := Secret{Username: "synthetic-user", Password: "Synthetic secret", PrivacyPassword: "Synthetic privacy"}
	b, _ := json.Marshal(s)
	if string(b) != "{}" {
		t.Fatal("secret serialized")
	}
	if s.String() != s.GoString() {
		t.Fatal("logging guard")
	}
}
