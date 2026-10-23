// Package vault protects credentials and portable backups. Secrets never implement
// ordinary JSON serialization; only explicit encrypted payloads contain them.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"

	"golang.org/x/crypto/argon2"
)

const Format = "sadptools-vault-v1"
const MaxBackup = 64 << 20

type Secret struct {
	Username        string `json:"-"`
	Password        string `json:"-"`
	PrivacyPassword string `json:"-"`
}

func (Secret) String() string                 { return "credential (redacted)" }
func (s Secret) GoString() string             { return s.String() }
func (s Secret) LogValue() slog.Value         { return slog.StringValue(s.String()) }
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }
func (s *Secret) Clear()                      { s.Username = ""; s.Password = ""; s.PrivacyPassword = "" }
func EncodeSecret(s Secret) ([]byte, error) {
	return json.Marshal([]string{s.Username, s.Password, s.PrivacyPassword})
}
func DecodeSecret(b []byte) (Secret, error) {
	var a []string
	if json.Unmarshal(b, &a) != nil || len(a) != 3 {
		return Secret{}, errors.New("invalid protected credential")
	}
	return Secret{a[0], a[1], a[2]}, nil
}
func Random(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("operating system random source unavailable")
	}
	return b
}
func Seal(key, plain []byte, purpose string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := Random(a.NonceSize())
	return a.Seal(nonce, nonce, plain, []byte(purpose)), nil
}
func Open(key, sealed []byte, purpose string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < a.NonceSize()+a.Overhead() {
		return nil, errors.New("invalid encrypted data")
	}
	out, err := a.Open(nil, sealed[:a.NonceSize()], sealed[a.NonceSize():], []byte(purpose))
	if err != nil {
		return nil, errors.New("wrong passphrase or damaged encrypted data")
	}
	return out, nil
}
func Derive(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 2, 32)
}
func CheckPassphrase(pass string) error {
	if len(pass) < 12 || len(pass) > 1024 {
		return errors.New("use a master/backup passphrase of 12–1024 bytes")
	}
	return nil
}

type Envelope struct {
	Format string `json:"format"`
	Salt   []byte `json:"salt"`
	Data   []byte `json:"data"`
}

func EncryptBackup(plain []byte, pass string) ([]byte, error) {
	if err := CheckPassphrase(pass); err != nil {
		return nil, err
	}
	if len(plain) > MaxBackup {
		return nil, errors.New("backup exceeds 64 MiB")
	}
	salt := Random(16)
	key := Derive(pass, salt)
	defer clear(key)
	data, err := Seal(key, plain, Format)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Format, salt, data})
}
func DecryptBackup(data []byte, pass string) ([]byte, error) {
	if len(data) > 2*MaxBackup || len(pass) > 1024 {
		return nil, errors.New("backup exceeds size limit")
	}
	var e Envelope
	if json.Unmarshal(data, &e) != nil || e.Format != Format || len(e.Salt) != 16 || len(e.Data) > MaxBackup+64 {
		return nil, errors.New("unsupported or damaged backup")
	}
	key := Derive(pass, e.Salt)
	defer clear(key)
	return Open(key, e.Data, Format)
}
