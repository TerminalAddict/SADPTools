package recovery

import (
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"strings"
)

// This is the camera's RSA-1024/PKCS#1 v1.5 challenge profile, not general
// password-based ISAPI CBC encryption. See docs/security-question-recovery.md.
func publicKeyXML(key *rsa.PrivateKey) []byte {
	value := base64.StdEncoding.EncodeToString([]byte(key.N.Text(16)))
	return []byte(xml.Header + "<PublicKey><key>" + value + "</key></PublicKey>")
}
func decryptChallenge(body []byte, key *rsa.PrivateKey) ([]byte, error) {
	var response struct {
		XMLName xml.Name `xml:"Challenge"`
		Key     string   `xml:"key"`
	}
	bad := errors.New("camera returned an unsupported encryption challenge; no reset was sent")
	if xml.Unmarshal(body, &response) != nil {
		return nil, bad
	}
	hexText, err := base64.StdEncoding.DecodeString(strings.TrimSpace(response.Key))
	if err != nil {
		return nil, bad
	}
	defer clear(hexText)
	ciphertext, err := hex.DecodeString(strings.TrimSuffix(string(hexText), "?"))
	if err != nil || len(ciphertext) != key.Size() {
		return nil, bad
	}
	defer clear(ciphertext)
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, key, ciphertext)
	if err != nil {
		return nil, bad
	}
	decoded, err := hex.DecodeString(string(plain))
	clear(decoded)
	if err != nil || len(plain) != 32 {
		clear(plain)
		return nil, bad
	}
	return plain, nil
}

// Each value is base64(ASCII hex(AES(challenge[:16]) || AES(base64(UTF-8
// value)))). The device's ECB blocks use zero padding, with no extra block when
// the input length is a multiple of 16. Every value uses the same challenge.
func encryptValue(value string, challenge []byte) (string, error) {
	key, err := hex.DecodeString(string(challenge))
	if err != nil || len(key) != 16 || len(challenge) != 32 {
		return "", errors.New("invalid encryption challenge")
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("could not prepare recovery encryption")
	}
	encoded := []byte(base64.StdEncoding.EncodeToString([]byte(value)))
	defer clear(encoded)
	plain := make([]byte, 16+((len(encoded)+15)/16)*16)
	defer clear(plain)
	copy(plain, challenge[:16])
	copy(plain[16:], encoded)
	for i := 0; i < len(plain); i += 16 {
		block.Encrypt(plain[i:i+16], plain[i:i+16])
	}
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(plain))), nil
}
func resetXML(r Request, challenge []byte) ([]byte, error) {
	type question struct {
		ID     int    `xml:"id"`
		Answer string `xml:"answer"`
	}
	body := struct {
		XMLName   xml.Name   `xml:"SecurityQuestion"`
		Questions []question `xml:"QuestionList>Question"`
		Password  string     `xml:"Resetpassword"`
	}{}
	for _, a := range r.Answers {
		value, err := encryptValue(a.Answer, challenge)
		if err != nil {
			return nil, err
		}
		body.Questions = append(body.Questions, question{a.ID, value})
	}
	var err error
	body.Password, err = encryptValue(r.Password, challenge)
	if err != nil {
		return nil, err
	}
	encoded, err := xml.Marshal(body)
	return append([]byte(xml.Header), encoded...), err
}
