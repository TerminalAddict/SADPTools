package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// All payloads are synthetic. Tests never contact GitHub or execute installers.
var payload = []byte("MZ synthetic installer fixture; never executable")

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(body []byte) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
}
func fixture(tag string, prerelease bool) release {
	name := "SADPTools-" + strings.TrimPrefix(tag, "v") + "-Setup-windows-amd64.exe"
	hash := sha256.Sum256(payload)
	base := ReleasesURL + "/download/" + tag + "/"
	return release{Tag: tag, Prerelease: prerelease, Assets: []asset{
		{Name: name, URL: base + name, State: "uploaded", Size: int64(len(payload)), Digest: "sha256:" + hex.EncodeToString(hash[:])},
		{Name: "SHA256SUMS", URL: base + "SHA256SUMS", State: "uploaded", Size: 200},
	}}
}
func syntheticClient(releases []release, edit func(*http.Request, *http.Response) (*http.Response, error)) *Client {
	return &Client{http: &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case req.URL.Host == "api.github.com":
			body, _ = json.Marshal(releases)
		case strings.HasSuffix(req.URL.Path, "/SHA256SUMS"):
			hash := sha256.Sum256(payload)
			for _, r := range releases {
				for _, a := range r.Assets {
					if strings.HasSuffix(a.Name, ".exe") {
						body = append(body, []byte(hex.EncodeToString(hash[:])+"  "+a.Name+"\n")...)
					}
				}
			}
		default:
			body = payload
		}
		r := response(body)
		if edit != nil {
			return edit(req, r)
		}
		return r, nil
	})}}
}
func available(t *testing.T, c *Client) Offer {
	t.Helper()
	o, err := c.Check(context.Background(), "0.2.0-beta.1", "beta")
	if err != nil || o == nil {
		t.Fatalf("offer=%v err=%v", o, err)
	}
	return *o
}

func TestVersionOrdering(t *testing.T) {
	values := []string{"0.2.0-dev", "0.2.0-beta.2", "0.2.0-beta.10", "0.2.0-rc.1", "0.2.0", "0.2.1-beta.1", "0.10.0", "1.0.0"}
	for i, value := range values {
		v, err := parseVersion(value)
		if err != nil {
			t.Fatal(err)
		}
		if v.newer(v) {
			t.Fatal("equal version treated as new")
		}
		for _, older := range values[:i] {
			old, _ := parseVersion(older)
			if !v.newer(old) || old.newer(v) {
				t.Fatalf("bad ordering %s %s", value, older)
			}
		}
	}
	for _, value := range []string{"1.0", "01.0.0", "0.2.0-beta", "0.2.0-beta.0", "0.2.0-beta.20000", "0.2.0-dev.1", "65536.0.0", "1.0.0+other", "../../file"} {
		if _, err := parseVersion(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}

func TestReleaseSelectionAndChannels(t *testing.T) {
	draft := fixture("v9.0.0", false)
	draft.Draft = true
	client := syntheticClient([]release{fixture("v0.2.0-beta.10", true), draft, fixture("v0.2.0-beta.2", true), fixture("v0.1.9", false)}, nil)
	if got := available(t, client).Version; got != "0.2.0-beta.10" {
		t.Fatal(got)
	}
	for _, current := range []string{"0.2.0-beta.10", "0.2.0", "1.0.0"} {
		if o, err := client.Check(context.Background(), current, "beta"); err != nil || o != nil {
			t.Fatalf("offered downgrade/equal: %v %v", o, err)
		}
	}
	if o, err := client.Check(context.Background(), "0.1.0", "stable"); err != nil || o == nil || o.Version != "0.1.9" {
		t.Fatalf("%v %v", o, err)
	}
	if _, err := client.Check(context.Background(), "0.2.0-dev", "beta"); err == nil {
		t.Fatal("development build allowed")
	}
	if _, err := client.Check(context.Background(), "0.2.0-beta.1", "other"); err == nil {
		t.Fatal("unknown channel allowed")
	}
}

func TestStableReplacesBetaAndMalformedReleasesAreSkipped(t *testing.T) {
	client := syntheticClient([]release{{Tag: "latest"}, fixture("v9.0.0", true), fixture("v0.2.0", false)}, nil)
	if got := available(t, client).Version; got != "0.2.0" {
		t.Fatal(got)
	}
}

func TestPagination(t *testing.T) {
	page := make([]release, 100)
	for i := range page {
		page[i] = release{Tag: "unrelated"}
	}
	var calls int
	c := syntheticClient(page, func(req *http.Request, r *http.Response) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			calls++
			if req.URL.Query().Get("page") == "2" {
				b, _ := json.Marshal([]release{fixture("v0.2.0-beta.2", true)})
				return response(b), nil
			}
		} else if strings.HasSuffix(req.URL.Path, "SHA256SUMS") {
			a := fixture("v0.2.0-beta.2", true).Assets[0]
			return response([]byte(strings.TrimPrefix(a.Digest, "sha256:") + "  " + a.Name)), nil
		}
		return r, nil
	})
	available(t, c)
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestBadAssetsAndChecksumsRejected(t *testing.T) {
	cases := map[string]func(*release){
		"missing":          func(r *release) { r.Assets = nil },
		"wrong repository": func(r *release) { r.Assets[0].URL = strings.Replace(r.Assets[0].URL, repository, "other/project", 1) },
		"oversized":        func(r *release) { r.Assets[0].Size = maxInstallerSize + 1 },
		"duplicate":        func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) },
		"not uploaded":     func(r *release) { r.Assets[0].State = "new" },
		"digest mismatch":  func(r *release) { r.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := fixture("v0.2.0-beta.2", true)
			change(&r)
			if _, err := syntheticClient([]release{r}, nil).Check(context.Background(), "0.2.0-beta.1", "beta"); err == nil {
				t.Fatal("accepted bad release")
			}
		})
	}
	for _, data := range []string{"", "invalid  setup.exe", strings.Repeat("a", 64) + "  setup.exe\n" + strings.Repeat("a", 64) + "  setup.exe"} {
		if _, err := checksum([]byte(data), "setup.exe"); err == nil {
			t.Fatal("accepted missing/invalid/duplicate hash")
		}
	}
}

func TestRedirectPolicy(t *testing.T) {
	for _, address := range []string{"http://github.com/file", "https://github.com.evil.invalid/file", "https://evil.invalid/file", "https://user@github.com/file", "https://github.com:443/file"} {
		u, _ := url.Parse(address)
		if err := checkRedirect(&http.Request{URL: u}, nil); err == nil {
			t.Fatalf("allowed %s", address)
		}
	}
	u, _ := url.Parse("https://release-assets.githubusercontent.com/file?token=public-asset")
	if err := checkRedirect(&http.Request{URL: u}, nil); err != nil {
		t.Fatal(err)
	}
	if err := checkRedirect(&http.Request{URL: u}, make([]*http.Request, 5)); err == nil {
		t.Fatal("unbounded redirects")
	}
}

func TestHTTPFailuresAndCancellation(t *testing.T) {
	for _, code := range []int{403, 404, 429, 500} {
		c := syntheticClient(nil, func(_ *http.Request, r *http.Response) (*http.Response, error) { r.StatusCode = code; return r, nil })
		if _, err := c.Check(context.Background(), "0.1.0", "stable"); err == nil {
			t.Fatal(code)
		}
	}
	c := syntheticClient(nil, func(req *http.Request, _ *http.Response) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := c.Check(ctx, "0.1.0", "stable"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestDownloadVerificationAndCleanup(t *testing.T) {
	for _, kind := range []string{"good", "truncated", "oversized", "corrupt", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			c := syntheticClient([]release{fixture("v0.2.0-beta.2", true)}, nil)
			o := available(t, c)
			c = syntheticClient(nil, func(req *http.Request, r *http.Response) (*http.Response, error) {
				switch kind {
				case "truncated":
					return response(payload[:4]), nil
				case "oversized":
					return response(append(append([]byte{}, payload...), 0)), nil
				case "corrupt":
					return response(bytes.Repeat([]byte("x"), len(payload))), nil
				case "cancelled":
					<-req.Context().Done()
					return nil, req.Context().Err()
				}
				return r, nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			cache := t.TempDir()
			var progress int64
			path, err := c.Download(ctx, o, cache, func(n int64) { progress = n })
			if kind == "good" {
				if err != nil || VerifyFile(path, o) != nil || progress != o.Size {
					t.Fatalf("%s %v %d", path, err, progress)
				}
			} else {
				if err == nil {
					t.Fatal("bad download accepted")
				}
				entries, _ := os.ReadDir(cache)
				if len(entries) != 0 {
					t.Fatal("partial files left behind")
				}
			}
		})
	}
}

func waitState(t *testing.T, m *Manager, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if m.Snapshot().Status == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("wanted %s; got %+v", want, m.Snapshot())
}
func readyManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager("0.2.0-beta.1", t.TempDir(), nil)
	m.state.Supported = true
	m.client = syntheticClient([]release{fixture("v0.2.0-beta.2", true)}, nil)
	t.Cleanup(m.Close)
	if err := m.Check("beta"); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "available")
	if err := m.Download("beta"); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "ready")
	return m
}

func TestInstallRevalidatesAndNeverLaunchesTamperedFile(t *testing.T) {
	m := readyManager(t)
	var launches int
	m.launch = func(string) error { launches++; return nil }
	if err := m.Install("stable"); err == nil {
		t.Fatal("installed from obsolete channel")
	}
	if err := os.WriteFile(m.path, bytes.Repeat([]byte("x"), len(payload)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Install("beta"); err == nil || launches != 0 {
		t.Fatalf("%v %d", err, launches)
	}
	if m.Snapshot().Status != "available" {
		t.Fatal(m.Snapshot())
	}
}

func TestInstallLaunchFailureAndSuccessfulHandoff(t *testing.T) {
	m := readyManager(t)
	m.launch = func(string) error { return errors.New("synthetic launch failure") }
	if err := m.Install("beta"); err == nil || m.Snapshot().Status != "ready" {
		t.Fatal("launch failure lost ready state")
	}
	path := m.path
	m.launch = func(got string) error {
		if got != path {
			t.Fatal(got)
		}
		return nil
	}
	if err := m.Install("beta"); err != nil {
		t.Fatal(err)
	}
	m.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("handoff deleted running installer", err)
	}
}

func TestManagerCancellationCloseAndConcurrentActions(t *testing.T) {
	m := NewManager("0.2.0-beta.1", t.TempDir(), nil)
	started := make(chan struct{})
	var active atomic.Int32
	m.client = syntheticClient(nil, func(req *http.Request, _ *http.Response) (*http.Response, error) {
		active.Add(1)
		close(started)
		defer active.Add(-1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	if err := m.Check("beta"); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := m.Check("beta"); err == nil {
		t.Fatal("overlapping check allowed")
	}
	m.Cancel()
	m.Close()
	if active.Load() != 0 {
		t.Fatal("request survived shutdown")
	}
	if err := m.Check("beta"); err == nil {
		t.Fatal("check allowed after shutdown")
	}
	ready := readyManager(t)
	path := ready.path
	ready.Cancel()
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("discard left installer behind")
	}
}
