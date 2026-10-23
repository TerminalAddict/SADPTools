package recovery

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

// All identities, questions, answers and challenges in this file are synthetic.
var fixtureTarget = Target{Address: "192.0.2.40", Port: 80, Interface: "Ethernet Fixture", Source: "192.0.2.10"}

const fixtureCapabilities = `<externSecurityCap><RestAdminPassWord><isSupportWithSecurityQuestion>true</isSupportWithSecurityQuestion></RestAdminPassWord></externSecurityCap>`
const fixtureQuestions = `<SecurityQuestion><QuestionList><Question><id>2</id><mark>true</mark></Question><Question><id>7</id><mark>true</mark></Question><Question><id>9</id><mark>true</mark></Question></QuestionList></SecurityQuestion>`
const fixtureCatalog = `{"questionList":{"number2":"Synthetic first question?","number7":"Synthetic second question?","number9":"Synthetic third question?"}}`
const fixtureChallenge = "00112233445566778899aabbccddeeff"

func fixtureRequest() Request {
	return Request{Answers: []Answer{{2, "  SYNTHETIC & <answer>  "}, {7, "SYNTHETIC Māori"}, {9, "SYNTHETIC third"}}, Password: "Synthetic-<&123"}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type fixture struct {
	t           *testing.T
	paths       []string
	puts, posts int
	change      func(*http.Request) (*http.Response, error, bool)
	put         func(*http.Request, []byte) (*http.Response, error)
}

func (f *fixture) client() *Client {
	return &Client{detector: fixtureWeb{}, newHTTP: func(target Target) *http.Client {
		c := localHTTP(target)
		c.Transport = roundTrip(f.serve)
		return c
	}}
}
func (f *fixture) serve(r *http.Request) (*http.Response, error) {
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	if r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		f.t.Fatal("unexpected credentials/query")
	}
	if f.change != nil {
		if response, err, handled := f.change(r); handled {
			return response, err
		}
	}
	switch r.URL.Path {
	case capabilitiesPath:
		return response(200, fixtureCapabilities), nil
	case questionsPath:
		return response(200, fixtureQuestions), nil
	case catalogPath:
		return response(200, fixtureCatalog), nil
	case challengePath:
		f.posts++
		if r.Method != "POST" {
			f.t.Fatal("wrong challenge method")
		}
		var public struct {
			Key string `xml:"key"`
		}
		if xml.NewDecoder(r.Body).Decode(&public) != nil {
			f.t.Fatal("bad public key XML")
		}
		encoded, err := base64.StdEncoding.DecodeString(public.Key)
		if err != nil {
			f.t.Fatal(err)
		}
		n, ok := new(big.Int).SetString(string(encoded), 16)
		if !ok || n.BitLen() != 1024 {
			f.t.Fatal("wrong RSA modulus format")
		}
		ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &rsa.PublicKey{N: n, E: 65537}, []byte(fixtureChallenge))
		if err != nil {
			f.t.Fatal(err)
		}
		return response(200, "<Challenge><key>"+base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(ciphertext)))+"</key></Challenge>"), nil
	case resetPath:
		f.puts++
		if r.Method != "PUT" {
			f.t.Fatal("wrong reset method")
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Fatal(err)
		}
		if f.put != nil {
			return f.put(r, data)
		}
		f.checkEncrypted(data)
		return response(200, `<ResponseStatus><statusCode>1</statusCode></ResponseStatus>`), nil
	default:
		f.t.Fatal("unexpected endpoint", r.URL.Path)
		return nil, errors.New("unknown fixture")
	}
}
func (f *fixture) checkEncrypted(data []byte) {
	r := fixtureRequest()
	for _, secret := range []string{r.Password, r.Answers[0].Answer, r.Answers[1].Answer, r.Answers[2].Answer, fixtureChallenge} {
		if bytes.Contains(data, []byte(secret)) {
			f.t.Fatal("plaintext secret on the wire")
		}
	}
	var payload struct {
		Answers []struct {
			ID    int    `xml:"id"`
			Value string `xml:"answer"`
		} `xml:"QuestionList>Question"`
		Password string `xml:"Resetpassword"`
	}
	if xml.Unmarshal(data, &payload) != nil || len(payload.Answers) != 3 {
		f.t.Fatal("invalid reset XML")
	}
	decrypt := func(value string) string {
		hexBytes, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			f.t.Fatal(err)
		}
		ciphertext, err := hex.DecodeString(string(hexBytes))
		if err != nil || len(ciphertext)%16 != 0 || len(ciphertext) < 32 {
			f.t.Fatal("bad encrypted field")
		}
		key, _ := hex.DecodeString(fixtureChallenge)
		block, _ := aes.NewCipher(key)
		for i := 0; i < len(ciphertext); i += 16 {
			block.Decrypt(ciphertext[i:i+16], ciphertext[i:i+16])
		}
		if string(ciphertext[:16]) != fixtureChallenge[:16] {
			f.t.Fatal("challenge prefix missing")
		}
		plain, err := base64.StdEncoding.DecodeString(string(bytes.TrimRight(ciphertext[16:], "\x00")))
		if err != nil {
			f.t.Fatal(err)
		}
		return string(plain)
	}
	for i, a := range payload.Answers {
		if a.ID != r.Answers[i].ID || decrypt(a.Value) != r.Answers[i].Answer {
			f.t.Fatal("answer not preserved exactly")
		}
	}
	if decrypt(payload.Password) != r.Password {
		f.t.Fatal("password not preserved")
	}
}
func TestReadOnlyCheckAndEncryptedReset(t *testing.T) {
	f := &fixture{t: t}
	c := f.client()
	status, err := c.Check(context.Background(), fixtureTarget)
	if err != nil || status.State != "ready" || len(status.Questions) != 3 || f.posts != 0 || f.puts != 0 {
		t.Fatal(status, err)
	}
	result, err := c.Reset(context.Background(), fixtureTarget, fixtureRequest())
	if err != nil || result.Outcome != "acknowledged" || !result.Sent || f.posts != 1 || f.puts != 1 {
		t.Fatal(result, err)
	}
}
func TestCheckStatesAndNoWritesWhenUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, state string
		code                    int
	}{
		{"unsupported", capabilitiesPath, strings.Replace(fixtureCapabilities, "true", "false", 1), "unsupported", 200},
		{"missing capability", capabilitiesPath, "<externSecurityCap/>", "unknown", 200},
		{"login required", capabilitiesPath, "", "unsupported", 401},
		{"redirect", capabilitiesPath, "", "unknown", 302},
		{"html instead of XML", capabilitiesPath, "<html>Login</html>", "unknown", 200},
		{"no selected questions", questionsPath, strings.ReplaceAll(fixtureQuestions, "true", "false"), "not_configured", 200},
		{"partial setup", questionsPath, strings.Replace(fixtureQuestions, "true", "false", 1), "not_configured", 200},
		{"duplicate IDs", questionsPath, strings.Replace(fixtureQuestions, "<id>7</id>", "<id>2</id>", 1), "unknown", 200},
		{"missing mark", questionsPath, strings.Replace(fixtureQuestions, "<mark>true</mark>", "", 1), "unknown", 200},
		{"unknown question text", catalogPath, `{"questionList":{}}`, "unsupported", 200},
		{"bad catalog", catalogPath, `{"questionList":`, "unsupported", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixture{t: t, change: func(r *http.Request) (*http.Response, error, bool) {
				if r.URL.Path == tc.path {
					return response(tc.code, tc.body), nil, true
				}
				return nil, nil, false
			}}
			status, err := f.client().Check(context.Background(), fixtureTarget)
			if err != nil || status.State != tc.state {
				t.Fatal(status, err)
			}
			result, err := f.client().Reset(context.Background(), fixtureTarget, fixtureRequest())
			if err == nil || result.Sent || f.posts != 0 || f.puts != 0 {
				t.Fatal("unavailable camera received a reset")
			}
		})
	}
}
func TestRejectedLockedAndUnconfirmedResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body, outcome string
		code                int
		failure             bool
	}{
		{"wrong answer", `<ResponseStatus><statusCode>4</statusCode><retryTimes>4</retryTimes><statusString>UNTRUSTED-SECRET</statusString></ResponseStatus>`, "rejected", 400, false},
		{"locked", `<ResponseStatus><statusCode>4</statusCode><lockStatus>locked</lockStatus><resLockTime>1800</resLockTime></ResponseStatus>`, "rejected", 403, false},
		{"lost reply", "", "unconfirmed", 0, true},
		{"malformed", "not XML", "unconfirmed", 200, false},
		{"missing result", `<ResponseStatus/>`, "unconfirmed", 200, false},
		{"unknown result", `<ResponseStatus><statusCode>99</statusCode></ResponseStatus>`, "unconfirmed", 200, false},
		{"HTTP failure with success body", `<ResponseStatus><statusCode>1</statusCode></ResponseStatus>`, "unconfirmed", 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixture{t: t, put: func(_ *http.Request, _ []byte) (*http.Response, error) {
				if tc.failure {
					return nil, errors.New("UNTRUSTED-SECRET")
				}
				return response(tc.code, tc.body), nil
			}}
			r, err := f.client().Reset(context.Background(), fixtureTarget, fixtureRequest())
			if err == nil || !r.Sent || r.Outcome != tc.outcome || f.puts != 1 || f.posts != 1 {
				t.Fatal(r, err)
			}
			if strings.Contains(fmt.Sprint(r, err), "UNTRUSTED-SECRET") {
				t.Fatal("raw response/error exposed")
			}
			if tc.name == "locked" && (r.LockSeconds == nil || *r.LockSeconds != 1800) {
				t.Fatal("lock time lost")
			}
			if tc.name == "wrong answer" && (r.RetryTimes == nil || *r.RetryTimes != 4) {
				t.Fatal("remaining attempts lost")
			}
		})
	}
}
func TestCancellationBeforeAndDuringSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fixture{t: t}
	result, err := f.client().Reset(ctx, fixtureTarget, fixtureRequest())
	if err == nil || result.Sent || f.puts != 0 {
		t.Fatal("cancelled request sent")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	f = &fixture{t: t, put: func(r *http.Request, _ []byte) (*http.Response, error) {
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	}}
	result, err = f.client().Reset(ctx, fixtureTarget, fixtureRequest())
	if !errors.Is(err, configuration.ErrUnconfirmed) || !errors.Is(err, context.Canceled) || !result.Sent || f.puts != 1 {
		t.Fatal(result, err)
	}
}
func TestChangedQuestionsInvalidInputAndChallengeDoNotReset(t *testing.T) {
	f := &fixture{t: t}
	r := fixtureRequest()
	r.Answers[0].ID = 11
	result, err := f.client().Reset(context.Background(), fixtureTarget, r)
	if err == nil || result.Sent || f.posts != 0 {
		t.Fatal("stale question sent")
	}
	r = fixtureRequest()
	r.Answers[0].Answer = ""
	f.paths = nil
	if _, err = f.client().Reset(context.Background(), fixtureTarget, r); err == nil || len(f.paths) != 0 {
		t.Fatal("invalid answers opened a connection")
	}
	f.change = func(r *http.Request) (*http.Response, error, bool) {
		if r.URL.Path == challengePath {
			return response(200, "<Challenge><key>bad</key></Challenge>"), nil, true
		}
		return nil, nil, false
	}
	result, err = f.client().Reset(context.Background(), fixtureTarget, fixtureRequest())
	if err == nil || result.Sent || f.puts != 0 {
		t.Fatal("unsupported encryption fell back to reset")
	}
}
func TestBoundedResponsesAndRedirects(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		calls := 0
		f := &fixture{t: t, change: func(r *http.Request) (*http.Response, error, bool) {
			calls++
			if redirect {
				x := response(302, "")
				x.Header.Set("Location", "http://198.51.100.1/")
				return x, nil, true
			}
			return response(200, strings.Repeat("x", (64<<10)+1)), nil, true
		}}
		s, err := f.client().Check(context.Background(), fixtureTarget)
		if s.State == "ready" || calls != 1 || (!redirect && err == nil) {
			t.Fatal("unbounded response or redirect followed")
		}
	}
}
func TestTimeoutIsBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	f := &fixture{t: t, change: func(r *http.Request) (*http.Response, error, bool) {
		<-r.Context().Done()
		return nil, r.Context().Err(), true
	}}
	_, err := f.client().Check(ctx, fixtureTarget)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestCredentialFormattingAndCipherVector(t *testing.T) {
	r := fixtureRequest()
	if strings.Contains(fmt.Sprintf("%v %#v %v", r, r, r.Answers), "SYNTHETIC") {
		t.Fatal("secret formatted")
	}
	// Synthetic compatibility vectors produced independently with the camera's
	// browser AES primitive. Includes UTF-8, exact blocks and preserved spaces.
	for _, tc := range []struct{ value, expected string }{
		{"SYNTHETIC Māori", "ZDZhYjY4MTM4ZGM4ODM2ZWNmZTJhOWY4M2I3OTEwYjE1NGZjNTk4MmJiZDVlYmNkZGQ2ZWNiMzhmMzI1NzRhMDhlZWFlYTk3MTNhY2E0ZTViNDlhODNlNTIwNTY5MTlh"},
		{"123456789012", "ZDZhYjY4MTM4ZGM4ODM2ZWNmZTJhOWY4M2I3OTEwYjE5Mjg0ZDg1YWFiNzc5ZjZmYjFmNWVhMWYwZmZiZDE4Yw=="},
		{"  <&fixture>  ", "ZDZhYjY4MTM4ZGM4ODM2ZWNmZTJhOWY4M2I3OTEwYjEyOTJjNjg2NjUxNTcyMGM5MDNlMjU3YWUxY2M0MTU2Y2NiYzZhNWI5NDZkNmQ0OTViYjU4ODQyYzFjNzE5NzZi"},
	} {
		got, err := encryptValue(tc.value, []byte(fixtureChallenge))
		if err != nil || got != tc.expected {
			t.Fatal("wire encryption vector mismatch", err)
		}
	}
	if _, err := encryptValue("fixture", nil); err == nil {
		t.Fatal("missing challenge accepted")
	}
	data, _ := json.Marshal(Result{Outcome: "acknowledged"})
	if strings.Contains(string(data), r.Password) {
		t.Fatal("result contains secrets")
	}
}

// Synthetic endpoint resolution for protocol tests; never contacts the network.
type fixtureWeb struct{}

func (fixtureWeb) Detect(_ context.Context, t webinterface.Target) (webinterface.Result, error) {
	return webinterface.Result{Endpoint: webinterface.Endpoint{URL: "http://" + t.Address + "/", Scheme: "http", Port: 80}}, nil
}
