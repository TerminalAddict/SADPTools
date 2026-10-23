// Package webinterface detects a camera's web endpoint using credential-free
// requests. Detection does not establish certificate trust for later writes.
package webinterface

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"
)

type Target struct {
	Address   string
	Port      int    // Advertised web port; zero means unknown.
	HTTPSPort int    // Separately advertised HTTPS port, for example from ONVIF.
	Source    string // Optional local source for an explicit recovery adapter.
}

type Endpoint struct {
	URL                string `json:"url"`
	Scheme             string `json:"scheme"`
	Port               int    `json:"port"`
	CertificateSHA256  string `json:"certificateSHA256"`
	CertificateTrusted bool   `json:"certificateTrusted"`
}

type Result struct {
	Endpoint Endpoint `json:"endpoint"`
	Message  string   `json:"message"`
}

type Detector interface {
	Detect(context.Context, Target) (Result, error)
}

type Client struct {
	transport func(Target) *http.Transport
}

func New() *Client { return &Client{transport: transport} }

func UsableAddress(address string) bool {
	ip, err := netip.ParseAddr(address)
	return err == nil && ip.Is4() && !ip.IsUnspecified() && !ip.IsLoopback() && !ip.IsMulticast() && address != "255.255.255.255"
}

func (t Target) Validate() error {
	if !UsableAddress(t.Address) || t.Port < 0 || t.Port > 65535 || t.HTTPSPort < 0 || t.HTTPSPort > 65535 || (t.Source != "" && !UsableAddress(t.Source)) {
		return errors.New("camera has no usable IPv4 web address or port")
	}
	return nil
}

func (e Endpoint) ValidFor(address string) bool {
	return UsableAddress(address) && (e.Scheme == "http" || e.Scheme == "https") && e.Port > 0 && e.Port <= 65535 && e.URL == origin(e.Scheme, address, e.Port)
}

func origin(scheme, address string, port int) string {
	host := address
	if (scheme == "http" && port != 80) || (scheme == "https" && port != 443) {
		host = net.JoinHostPort(address, strconv.Itoa(port))
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: "/"}).String()
}

func transport(t Target) *http.Transport {
	dialer := &net.Dialer{Timeout: 1500 * time.Millisecond}
	if t.Source != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(t.Source)}
	}
	return &http.Transport{
		DialContext: dialer.DialContext, DisableKeepAlives: true,
		TLSHandshakeTimeout: 1500 * time.Millisecond, ResponseHeaderTimeout: 1500 * time.Millisecond,
		MaxResponseHeaderBytes: 32 << 10,
		// Only unauthenticated GET / probes use this transport. The certificate
		// is inspected and reported; recovery uses a separate verified/pinned TLS
		// transport. No credentials, cookies or auth headers are ever supplied.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
}

func (c *Client) Detect(parent context.Context, target Target) (Result, error) {
	if err := target.Validate(); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	port := target.Port
	if port == 0 {
		port = 80
	}
	urls := []string{}
	if target.HTTPSPort != 0 && target.HTTPSPort != port && target.HTTPSPort != 443 {
		urls = append(urls, origin("https", target.Address, target.HTTPSPort))
	}
	urls = append(urls, origin("http", target.Address, port), origin("https", target.Address, port))
	if port != 443 {
		urls = append(urls, origin("https", target.Address, 443))
	}
	type answer struct {
		index    int
		endpoint Endpoint
	}
	answers := make(chan answer, len(urls))
	for i, u := range urls {
		go func() { answers <- answer{i, c.probe(ctx, target, u)} }()
	}
	results := make([]Endpoint, len(urls))
	// Join every bounded probe, including after cancellation. No probe outlives
	// the operation or keeps a socket open after the controller shuts down.
	for range urls {
		a := <-answers
		results[a.index] = a.endpoint
	}
	if parent.Err() != nil {
		return Result{}, parent.Err()
	}
	for _, scheme := range []string{"https", "http"} {
		for _, endpoint := range results {
			if endpoint.Scheme == scheme {
				message := "HTTP web interface detected."
				if scheme == "https" {
					message = "HTTPS web interface detected."
					if !endpoint.CertificateTrusted {
						message += " The camera's certificate is not trusted by this computer; your browser may ask you to confirm it."
					}
				}
				return Result{endpoint, message}, nil
			}
		}
	}
	return Result{Message: "No HTTP or HTTPS web interface responded. Check routing, the camera's web ports and firewall, then check again."}, nil
}

func (c *Client) probe(ctx context.Context, target Target, address string) Endpoint {
	tr := c.transport(target)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for hop := 0; hop < 3; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return Endpoint{}
		}
		req.Header.Set("Cache-Control", "no-cache")
		response, err := client.Do(req)
		if err != nil {
			return Endpoint{}
		}
		response.Body.Close() // No response body or credentials are retained.
		u := req.URL
		if response.StatusCode >= 300 && response.StatusCode <= 399 {
			location, err := response.Location()
			if err != nil || location.User != nil || location.Hostname() != target.Address || (location.Scheme != "http" && location.Scheme != "https") {
				return Endpoint{}
			}
			port := portOf(location)
			if port == 0 {
				return Endpoint{}
			}
			next := origin(location.Scheme, target.Address, port)
			if next != address {
				// Never downgrade HTTPS, follow another host or request a redirect
				// path/query that could represent an action. Only probe its origin.
				if u.Scheme == "https" && location.Scheme != "https" {
					return Endpoint{}
				}
				address = next
				continue
			}
		}
		e := Endpoint{URL: address, Scheme: u.Scheme, Port: portOf(u)}
		if response.TLS != nil && len(response.TLS.PeerCertificates) > 0 {
			certs := response.TLS.PeerCertificates
			hash := sha256.Sum256(certs[0].Raw)
			e.CertificateSHA256 = hex.EncodeToString(hash[:])
			intermediates := x509.NewCertPool()
			for _, cert := range certs[1:] {
				intermediates.AddCert(cert)
			}
			_, err := certs[0].Verify(x509.VerifyOptions{DNSName: target.Address, Intermediates: intermediates})
			e.CertificateTrusted = err == nil
		}
		return e
	}
	return Endpoint{}
}

func portOf(u *url.URL) int {
	if u.Port() == "" {
		if u.Scheme == "https" {
			return 443
		}
		return 80
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 {
		return 0
	}
	return p
}
