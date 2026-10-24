package streaming

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"time"
)

type Certificate struct {
	Authority string    `json:"authority"`
	SHA256    string    `json:"sha256"`
	Subject   string    `json:"subject"`
	Expires   time.Time `json:"expires"`
}

// InspectCertificate performs a TLS handshake only, never an authenticated
// request. Its result conveys identity to the operator, not automatic trust.
func InspectCertificate(ctx context.Context, address, raw string) (Certificate, error) {
	u, e := ValidateURL(address, raw)
	if e != nil {
		return Certificate{}, e
	}
	if u.Scheme != "https" && u.Scheme != "rtsps" {
		return Certificate{}, ErrEndpoint
	}
	authority := net.JoinHostPort(address, endpointPort(u))
	c, e := (&tls.Dialer{NetDialer: &net.Dialer{Timeout: 3 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}}).DialContext(ctx, "tcp4", authority)
	if e != nil {
		return Certificate{}, errors.New("could not inspect the camera TLS certificate")
	}
	defer c.Close()
	state := c.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return Certificate{}, errors.New("camera did not provide a TLS certificate")
	}
	cert := state.PeerCertificates[0]
	sum := sha256.Sum256(cert.Raw)
	subject := cert.Subject.String()
	if len(subject) > 256 {
		subject = subject[:256]
	}
	return Certificate{Authority: u.Scheme + "://" + authority, SHA256: hex.EncodeToString(sum[:]), Subject: subject, Expires: cert.NotAfter}, nil
}
