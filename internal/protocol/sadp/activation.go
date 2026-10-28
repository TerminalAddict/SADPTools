package sadp

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/devices"
)

// ActivationExchange implements the SADP RSA-1024/PKCS#1 v1.5 + AES-128-ECB
// activation profile. This is distinct from the HTTP ISAPI recovery profile.
// See docs/device-management.md for wire evidence and hardware-validation limits.
// A fresh private key belongs to one operation, never preferences or inventory.
type ActivationExchange struct{ key *rsa.PrivateKey }

func NewActivationExchange() (*ActivationExchange, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, errors.New("could not prepare activation encryption")
	}
	return &ActivationExchange{key: key}, nil
}

func (e *ActivationExchange) Close() {
	if e.key != nil {
		clear(e.key.D.Bits())
		for _, p := range e.key.Primes {
			clear(p.Bits())
		}
		e.key = nil // Go cannot promise complete erasure of crypto internals.
	}
}

func activationMAC(mac string) string {
	return strings.ToLower(strings.ReplaceAll(devices.NormalizeMAC(mac), ":", "-"))
}

func (e *ActivationExchange) Probe(id, mac string) ([]byte, error) {
	if e.key == nil {
		return nil, errors.New("activation exchange is closed")
	}
	public := x509.MarshalPKCS1PublicKey(&e.key.PublicKey)
	return Command(id, activationMAC(mac), ExchangeCode, "", base64.StdEncoding.EncodeToString(public), NetworkSettings{})
}

// EncryptPassword validates the response before producing any activation write.
// SADP uses the first 16 ASCII bytes of the decrypted challenge as the AES key
// AND as a plaintext prefix, then the password in a zero-filled 16-byte field.
// Neither the challenge nor ciphertext is hex-decoded/hex-encoded here. ISAPI's
// superficially similar activation/recovery formats must not be substituted.
func (e *ActivationExchange) EncryptPassword(code, password string) (string, error) {
	bad := errors.New("unsupported activation encryption response; activation was not sent")
	if len(password) < 8 || len(password) > 16 {
		return "", bad
	}
	challenge, err := e.decryptChallenge(code)
	defer clear(challenge)
	if err != nil {
		return "", bad
	}
	return encryptSADPPassword(challenge, password)
}

func (e *ActivationExchange) decryptChallenge(code string) ([]byte, error) {
	bad := errors.New("unsupported SADP encryption challenge")
	if e.key == nil || len(code) > 1024 {
		return nil, bad
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(code)
	if err != nil || len(ciphertext) != e.key.Size() {
		return nil, bad
	}
	defer clear(ciphertext)
	challenge, err := rsa.DecryptPKCS1v15(rand.Reader, e.key, ciphertext)
	if err != nil || len(challenge) != 32 {
		clear(challenge)
		return nil, bad
	}
	for _, c := range challenge {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			clear(challenge)
			return nil, bad
		}
	}
	return challenge, nil
}

func encryptSADPPassword(challenge []byte, password string) (string, error) {
	bad := errors.New("unsupported SADP password encryption input")
	if len(challenge) != 32 || len(password) < 1 || len(password) > 16 {
		return "", bad
	}
	block, err := aes.NewCipher(challenge[:16])
	if err != nil {
		return "", bad
	}
	plain := make([]byte, 32)
	defer clear(plain)
	copy(plain, challenge[:16])
	copy(plain[16:], password)
	block.Encrypt(plain[:16], plain[:16])
	block.Encrypt(plain[16:], plain[16:])
	return base64.StdEncoding.EncodeToString(plain), nil
}

func ActivationCommand(id, mac, encryptedPassword string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(encryptedPassword)
	defer clear(ciphertext)
	if err != nil || len(ciphertext) != 32 {
		return nil, errors.New("invalid encrypted activation password")
	}
	return Command(id, activationMAC(mac), Activate, encryptedPassword, "", NetworkSettings{})
}
