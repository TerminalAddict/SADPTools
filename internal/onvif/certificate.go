package onvif

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"
)

type Endpoint struct {
	URL                string `json:"url"`
	Scheme             string `json:"scheme"`
	CertificateTrusted bool   `json:"certificateTrusted"`
	CertificateSHA256  string `json:"certificateSHA256"`
	Subject            string `json:"subject"`
}

// InspectEndpoint sends no HTTP request or credentials. TLS inspection alone
// may accept an untrusted certificate to display its exact fingerprint.
func InspectEndpoint(ctx context.Context, t Target) (Endpoint, error) {
	out := Endpoint{}
	if !ValidXAddr(t.URL, t.Address) {
		return out, errors.New("invalid ONVIF service URL")
	}
	u, _ := url.Parse(t.URL)
	out.URL = t.URL
	out.Scheme = u.Scheme
	if u.Scheme != "https" {
		return out, nil
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(t.Source)}}
	port := servicePort(t.URL)
	tcp, e := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(t.Address, strconv.Itoa(port)))
	if e != nil {
		return out, errors.New("could not inspect ONVIF HTTPS endpoint")
	}
	defer tcp.Close()
	conn := tls.Client(tcp, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if e = conn.HandshakeContext(ctx); e != nil {
		return out, errors.New("ONVIF TLS handshake failed")
	}
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return out, errors.New("ONVIF HTTPS certificate missing")
	}
	cert := state.PeerCertificates[0]
	hash := sha256.Sum256(cert.Raw)
	out.CertificateSHA256 = hex.EncodeToString(hash[:])
	out.Subject = cert.Subject.String()
	pool := x509.NewCertPool()
	for _, ca := range state.PeerCertificates[1:] {
		pool.AddCert(ca)
	}
	_, e = cert.Verify(x509.VerifyOptions{DNSName: t.Address, Intermediates: pool})
	out.CertificateTrusted = e == nil
	return out, nil
}
