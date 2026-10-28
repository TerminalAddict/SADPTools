package recovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

const capabilitiesPath = "/ISAPI/Security/extern/capabilities"
const questionsPath = "/ISAPI/Security/questionConfiguration"
const challengePath = "/ISAPI/Security/challenge"
const resetPath = "/ISAPI/Security/questionCertification"
const catalogPath = "/doc/i18n/en/Common.json"

var errCertificateChanged = errors.New("camera HTTPS certificate changed; check and confirm it again")

type Client struct {
	newHTTP  func(Target) *http.Client
	detector webinterface.Detector
}

func New() *Client { return &Client{newHTTP: localHTTP, detector: webinterface.New()} }
func localHTTP(t Target) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(t.Source)}}
	var tlsConfig *tls.Config
	if t.CertificateSHA256 != "" {
		// A user explicitly accepted this exact camera certificate for this
		// operation. Every connection checks the pin; changed certificates fail.
		tlsConfig = &tls.Config{InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) > 0 {
				hash := sha256.Sum256(cs.PeerCertificates[0].Raw)
				if hex.EncodeToString(hash[:]) == strings.ToLower(t.CertificateSHA256) {
					return nil
				}
			}
			return errCertificateChanged
		}}
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		// No proxies, cookies, auth guesses, redirects, connection reuse or retries.
		Transport:     &http.Transport{DialContext: dialer.DialContext, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 5 * time.Second, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
func request(ctx context.Context, client *http.Client, target Target, method, path string, body []byte) (int, []byte, error) {
	url := target.Scheme + "://" + net.JoinHostPort(target.Address, strconv.Itoa(target.Port)) + path
	r, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errors.New("could not prepare device request")
	}
	r.GetBody = nil
	r.Header.Set("Accept", "application/xml, application/json")
	r.Header.Set("Cache-Control", "no-cache")
	if body != nil {
		r.Header.Set("Content-Type", "application/xml; charset=UTF-8")
	}
	response, err := client.Do(r)
	if err != nil {
		return 0, nil, connectionError(ctx, target, err)
	}
	defer response.Body.Close()
	limit := int64(64 << 10)
	if path == catalogPath {
		limit = 256 << 10
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		clear(data)
		return response.StatusCode, nil, errors.New("camera returned an incomplete or oversized response")
	}
	return response.StatusCode, data, nil
}

func connectionError(ctx context.Context, target Target, cause error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	connection := fmt.Sprintf("camera %s connection to %s from %s through %s", strings.ToUpper(target.Scheme), net.JoinHostPort(target.Address, strconv.Itoa(target.Port)), target.Source, target.Interface)
	var certificateError *tls.CertificateVerificationError
	if errors.Is(cause, errCertificateChanged) {
		return errCertificateChanged
	}
	if errors.As(cause, &certificateError) {
		return fmt.Errorf("%s could not verify the HTTPS certificate; check the camera connection and certificate again", connection)
	}
	var networkError net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(cause, &networkError) && networkError.Timeout()) {
		return fmt.Errorf("%s timed out; check routing, firewall rules and the camera's web service: %w", connection, context.DeadlineExceeded)
	}
	// A transport error can contain URLs or device-supplied content. Report the
	// attempted connection without echoing the underlying error or request data.
	return fmt.Errorf("%s failed; check routing, firewall rules and the camera's web service", connection)
}

func state(value, message string) Status {
	return Status{State: value, Message: message, Questions: []Question{}}
}
func (c *Client) Check(ctx context.Context, target Target) (Status, error) {
	if err := target.validate(); err != nil {
		return state("unavailable", err.Error()), err
	}
	target, endpoint, blocked, err := c.resolve(ctx, target)
	if err != nil || blocked.State != "" {
		return blocked, err
	}
	client := c.newHTTP(target)
	defer client.CloseIdleConnections()
	status, err := check(ctx, client, target)
	status.Endpoint = endpoint
	return status, err
}

func (c *Client) resolve(ctx context.Context, target Target) (Target, webinterface.Endpoint, Status, error) {
	if err := ctx.Err(); err != nil {
		return target, webinterface.Endpoint{}, state("unavailable", err.Error()), err
	}
	if target.Scheme != "" {
		host := target.Address
		if (target.Scheme == "http" && target.Port != 80) || (target.Scheme == "https" && target.Port != 443) {
			host = net.JoinHostPort(host, strconv.Itoa(target.Port))
		}
		endpoint := webinterface.Endpoint{URL: (&url.URL{Scheme: target.Scheme, Host: host, Path: "/"}).String(), Scheme: target.Scheme, Port: target.Port, CertificateSHA256: target.CertificateSHA256}
		return target, endpoint, Status{}, nil
	}
	result, err := c.detector.Detect(ctx, webinterface.Target{Address: target.Address, Port: target.Port, HTTPSPort: target.HTTPSPort, Source: target.Source})
	if err != nil {
		return target, webinterface.Endpoint{}, state("unavailable", err.Error()), err
	}
	e := result.Endpoint
	if !e.ValidFor(target.Address) {
		return target, e, state("unavailable", result.Message), nil
	}
	if target.CertificateSHA256 != "" && (e.Scheme != "https" || !strings.EqualFold(target.CertificateSHA256, e.CertificateSHA256)) {
		s := state("certificate_required", "The camera's HTTPS certificate changed or HTTPS is unavailable. Check the connection and certificate again.")
		s.Endpoint = e
		return target, e, s, nil
	}
	if e.Scheme == "https" && !e.CertificateTrusted && target.CertificateSHA256 == "" {
		s := state("certificate_required", "HTTPS is available, but this computer does not trust the camera's certificate. Confirm this camera certificate to use security-question recovery, or use its web interface.")
		s.Endpoint = e
		return target, e, s, nil
	}
	target.Scheme, target.Port = e.Scheme, e.Port
	return target, e, Status{}, nil
}
func check(ctx context.Context, client *http.Client, target Target) (Status, error) {
	code, data, err := request(ctx, client, target, http.MethodGet, capabilitiesPath, nil)
	defer clear(data)
	if err != nil {
		return state("unavailable", err.Error()), err
	}
	if code == 401 || code == 403 || code == 404 || code == 405 {
		return state("unsupported", "This firmware does not expose security-question recovery through the supported local interface. Use its web interface or authorised support recovery."), nil
	}
	var caps struct {
		XMLName xml.Name `xml:"externSecurityCap"`
		Reset   struct {
			Supported *bool `xml:"isSupportWithSecurityQuestion"`
		} `xml:"RestAdminPassWord"`
	}
	if code != 200 || xml.Unmarshal(data, &caps) != nil {
		return state("unknown", "Camera recovery capabilities could not be verified."), nil
	}
	if caps.Reset.Supported == nil {
		return state("unknown", "Camera did not report whether security-question recovery is supported."), nil
	}
	if !*caps.Reset.Supported {
		return state("unsupported", "This camera reports that security-question recovery is unsupported."), nil
	}
	code, data, err = request(ctx, client, target, http.MethodGet, questionsPath, nil)
	defer clear(data)
	if err != nil {
		return state("unavailable", err.Error()), err
	}
	var list struct {
		XMLName   xml.Name `xml:"SecurityQuestion"`
		Questions []struct {
			ID   int   `xml:"id"`
			Mark *bool `xml:"mark"`
		} `xml:"QuestionList>Question"`
	}
	if code != 200 || xml.Unmarshal(data, &list) != nil || len(list.Questions) == 0 || len(list.Questions) > 128 {
		return state("unknown", "Camera did not return a recognised question list. Use its web interface."), nil
	}
	selected := []Question{}
	seen := map[int]bool{}
	for _, q := range list.Questions {
		if q.ID < 1 || q.ID > 4096 || seen[q.ID] || q.Mark == nil {
			return state("unknown", "Camera returned an incomplete or ambiguous question list."), nil
		}
		seen[q.ID] = true
		if *q.Mark {
			selected = append(selected, Question{ID: q.ID})
		}
	}
	if len(selected) != 3 {
		return state("not_configured", "Camera reports that three security questions are not configured. Use Set security questions with its current admin password, or configure them through its web interface. If the password is unknown, use authorised support recovery."), nil
	}
	// The API supplies IDs, not question text. Read this firmware's own English
	// catalog rather than guessing the meaning of question numbers.
	code, data, err = request(ctx, client, target, http.MethodGet, catalogPath, nil)
	defer clear(data)
	if err != nil {
		return state("unavailable", err.Error()), err
	}
	var catalog struct {
		Questions map[string]string `json:"questionList"`
	}
	if code != 200 || json.Unmarshal(data, &catalog) != nil {
		return state("unsupported", "Configured questions were found, but this firmware's question text format is unsupported. Use its web interface."), nil
	}
	for i := range selected {
		label := cleanLabel(catalog.Questions["number"+strconv.Itoa(selected[i].ID)])
		if !validText(label, 512) {
			return state("unsupported", "Camera did not supply readable text for every configured question. Use its web interface."), nil
		}
		selected[i].Text = label
	}
	return Status{State: "ready", Message: "Three configured security questions are available. Enter the original answers exactly as configured and choose a new admin password.", Questions: selected}, nil
}

func (c *Client) Reset(ctx context.Context, target Target, r Request) (Result, error) {
	if err := target.validate(); err != nil {
		return Result{}, err
	}
	if err := r.Validate(); err != nil {
		return Result{}, err
	}
	target, _, blocked, err := c.resolve(ctx, target)
	if err != nil {
		return Result{}, err
	}
	if blocked.State != "" {
		return Result{}, errors.New(blocked.Message)
	}
	client := c.newHTTP(target)
	defer client.CloseIdleConnections()
	status, err := check(ctx, client, target)
	if err != nil {
		return Result{}, err
	}
	if status.State != "ready" {
		return Result{}, errors.New(status.Message)
	}
	if !SameQuestions(status.Questions, r.Answers) {
		return Result{}, errors.New("configured questions changed; check the camera again before submitting answers")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return Result{}, errors.New("could not generate recovery encryption key")
	}
	defer func() {
		key.D.SetInt64(0)
		for _, p := range key.Primes {
			p.SetInt64(0)
		}
	}()
	code, data, err := request(ctx, client, target, http.MethodPost, challengePath, publicKeyXML(key))
	defer clear(data)
	if err != nil {
		return Result{}, err
	}
	if code != 200 {
		return Result{}, errors.New("camera refused the encryption exchange; no password reset was sent")
	}
	challenge, err := decryptChallenge(data, key)
	if err != nil {
		return Result{}, err
	}
	defer clear(challenge)
	body, err := resetXML(r, challenge)
	if err != nil {
		return Result{}, err
	}
	defer clear(body)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result := Result{Sent: true, Outcome: "unconfirmed", Message: "No conclusive reset reply. The password may have changed; verify through the camera login before trying again."}
	code, data, err = request(ctx, client, target, http.MethodPut, resetPath, body)
	defer clear(data)
	if err != nil {
		return result, errors.Join(configuration.ErrUnconfirmed, err)
	}
	var reply struct {
		XMLName xml.Name `xml:"ResponseStatus"`
		Code    *int     `xml:"statusCode"`
		Lock    string   `xml:"lockStatus"`
		Retries *int     `xml:"retryTimes"`
		Seconds *int     `xml:"resLockTime"`
	}
	if xml.Unmarshal(data, &reply) != nil || reply.Code == nil {
		return result, configuration.ErrUnconfirmed
	}
	if code == 200 && *reply.Code == 1 {
		result.Outcome = "acknowledged"
		result.Message = "Camera acknowledged the password reset. Verify the new admin password in its web interface."
		return result, nil
	}
	if *reply.Code >= 2 && *reply.Code <= 6 {
		result.Outcome = "rejected"
		result.Message = "Camera rejected the reset. Check the answers, password requirements and firmware support before another attempt."
		if reply.Retries != nil && *reply.Retries >= 0 && *reply.Retries <= 1000 {
			result.RetryTimes = reply.Retries
			result.Message += fmt.Sprintf(" Reported attempts remaining: %d.", *reply.Retries)
		}
		if reply.Lock == "locked" {
			result.Message = "Camera reports that recovery is locked. Do not submit another reset until the lock expires."
			if reply.Seconds != nil && *reply.Seconds >= 0 && *reply.Seconds <= 86400 {
				result.LockSeconds = reply.Seconds
				result.Message += fmt.Sprintf(" Reported wait: %d seconds.", *reply.Seconds)
			}
		}
		return result, configuration.ErrRejected
	}
	return result, configuration.ErrUnconfirmed
}
