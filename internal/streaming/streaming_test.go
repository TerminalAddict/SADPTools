package streaming

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

type captureSink struct {
	info    Info
	packets []Packet
	cancel  context.CancelFunc
	limit   int
}

func (s *captureSink) Information(i Info) error { s.info = i; return nil }
func (s *captureSink) Write(p Packet) error {
	s.packets = append(s.packets, p)
	if s.cancel != nil && len(s.packets) >= s.limit {
		s.cancel()
	}
	return nil
}

func TestEndpointValidationAndRedaction(t *testing.T) {
	for _, raw := range []string{"rtsp://192.0.2.3:8554/live", "https://192.0.2.3/image?channel=1"} {
		if _, e := ValidateURL("192.0.2.3", raw); e != nil {
			t.Fatal(e)
		}
	}
	for _, raw := range []string{"rtsp://u:p@192.0.2.3/x", "rtsp://192.0.2.4/x", "rtsp://camera.local/x", "file:///x", "exec:foo", "http://192.0.2.3:65536/x", "rtsp://192.0.2.3/x#secret", "rtsp://192.0.2.3/x\n"} {
		if _, e := ValidateURL("192.0.2.3", raw); e == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	s, e := NewSource("192.0.2.3", "rtsp://192.0.2.3/private", "rtsp", "", vault.Secret{Username: "synthetic-user", Password: "synthetic-password"})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(s)
	for _, v := range []string{string(b), fmt.Sprintf("%v %#v %+v", s, s, s)} {
		if strings.Contains(v, "private") || strings.Contains(v, "synthetic") {
			t.Fatal("protected source exposed")
		}
	}
}
func TestMuxAndRTSPAuthenticatedH264H265(t *testing.T) {
	for _, hevc := range []bool{false, true} {
		t.Run(fmt.Sprint(hevc), func(t *testing.T) {
			camera, e := testcamera.New(hevc, true, "")
			if e != nil {
				t.Fatal(e)
			}
			defer camera.Close()
			s, e := NewSource("127.0.0.1", camera.URL, "rtsp", "", vault.Secret{Username: "fixture", Password: "synthetic-password"})
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sink := &captureSink{cancel: cancel, limit: 7}
			e = NativeEngine{}.Play(ctx, s, sink)
			if !errors.Is(e, context.Canceled) {
				t.Fatalf("playback: %v packets=%d", e, len(sink.packets))
			}
			if len(sink.packets) < 7 || sink.info.Width != 320 || sink.info.Height != 180 {
				t.Fatalf("invalid media: %+v fragments=%d", sink.info, len(sink.packets))
			}
			var init fmp4.Init
			if e = init.Unmarshal(bytes.NewReader(sink.packets[0].Data)); e != nil {
				t.Fatal(e)
			}
			if len(init.Tracks) != 1 {
				t.Fatal("unexpected tracks")
			}
			for _, p := range sink.packets[1:] {
				var parts fmp4.Parts
				if e = parts.Unmarshal(p.Data); e != nil {
					t.Fatal(e)
				}
				if len(parts) != 1 || len(parts[0].Tracks) != 1 {
					t.Fatal("invalid fragment")
				}
			}
			deadline := time.Now().Add(time.Second)
			for camera.Connections.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if camera.Connections.Load() != 0 {
				t.Fatal("RTSP socket leaked")
			}
		})
	}
}
func TestRTSPRejectsRedirectAndBadCredentials(t *testing.T) {
	target, e := testcamera.New(false, false, "")
	if e != nil {
		t.Fatal(e)
	}
	defer target.Close()
	for _, redirect := range []string{target.URL, ""} {
		camera, e := testcamera.New(false, true, redirect)
		if e != nil {
			t.Fatal(e)
		}
		s, _ := NewSource("127.0.0.1", camera.URL, "rtsp", "", vault.Secret{Username: "fixture", Password: "wrong-synthetic-password"})
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		e = NativeEngine{}.Play(ctx, s, &captureSink{})
		cancel()
		camera.Close()
		if e == nil || (redirect != "" && !errors.Is(e, ErrEndpoint)) || (redirect == "" && !errors.Is(e, ErrAuthentication)) {
			t.Fatal(e)
		}
	}
	if target.Requests.Load() != 0 {
		t.Fatal("redirect destination was contacted")
	}
}
func TestHTTPImagesAuthBoundsRedirectAndTLS(t *testing.T) {
	var jpegData bytes.Buffer
	_ = jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 32, 18)), nil)
	var rejected atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { rejected.Add(1) }))
	defer other.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, other.URL, 302)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "fixture" || pass != "synthetic-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="synthetic"`)
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		if r.URL.Path == "/large" {
			_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", MaxFrame+1)))
			return
		}
		_, _ = w.Write(jpegData.Bytes())
	}))
	defer server.Close()
	cert, e := InspectCertificate(context.Background(), "127.0.0.1", server.URL)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		path, pin string
		success   bool
	}{{"/image", "", false}, {"/image", strings.Repeat("0", 64), false}, {"/image", cert.SHA256, true}, {"/large", cert.SHA256, false}, {"/redirect", cert.SHA256, false}} {
		s, _ := NewSource("127.0.0.1", server.URL+tc.path, "snapshot", tc.pin, vault.Secret{Username: "fixture", Password: "synthetic-password"})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		sink := &captureSink{cancel: cancel, limit: 1}
		e := NativeEngine{}.Play(ctx, s, sink)
		cancel()
		if tc.success && (len(sink.packets) != 1 || sink.info.Width != 32) {
			t.Fatal("snapshot failed", e)
		}
		if !tc.success && len(sink.packets) > 0 {
			t.Fatal("unsafe image accepted")
		}
	}
	if rejected.Load() != 0 {
		t.Fatal("HTTP redirect followed")
	}
}

type blockingEngine struct{ active, max atomic.Int32 }

func TestManagerAuthenticationStatus(t *testing.T) {
	// The viewer uses this code to ask for credentials, rather than treating
	// network failures as a rejected password or retrying credentials itself.
	for _, failure := range []error{ErrAuthentication, errors.New("synthetic offline camera")} {
		m := NewManager(nil)
		ticket, e := m.Prepare(func(context.Context, func(string, string), func([]Profile)) (Source, error) {
			return Source{}, failure
		}, nil, nil, "synthetic")
		if e != nil {
			t.Fatal(e)
		}
		if e = m.Consume(context.Background(), ticket.ID, func(Packet) error { return nil }); e != nil {
			t.Fatal(e)
		}
		status := m.Status(ticket.ID)
		m.Close()
		if status.State != "error" || (status.ErrorCode == "authentication") != errors.Is(failure, ErrAuthentication) {
			t.Fatalf("incorrect viewer error classification: %+v", status)
		}
	}
}

func (e *blockingEngine) Play(ctx context.Context, _ Source, _ Sink) error {
	n := e.active.Add(1)
	defer e.active.Add(-1)
	for {
		v := e.max.Load()
		if v >= n || e.max.CompareAndSwap(v, n) {
			break
		}
	}
	<-ctx.Done()
	return ctx.Err()
}
func TestManagerRepeatedCancellationAndSingleUse(t *testing.T) {
	engine := &blockingEngine{}
	m := NewManager(engine)
	defer m.Close()
	var disposed atomic.Int32
	resolver := func(context.Context, func(string, string), func([]Profile)) (Source, error) { return Source{}, nil }
	for range 100 {
		ticket, e := m.Prepare(resolver, func() { disposed.Add(1) }, nil, "synthetic")
		if e != nil {
			t.Fatal(e)
		}
		if e = m.Consume(context.Background(), "invalid", func(Packet) error { return nil }); e == nil {
			t.Fatal("invalid capability accepted")
		}
		done := make(chan struct{})
		go func() { _ = m.Consume(context.Background(), ticket.ID, func(Packet) error { return nil }); close(done) }()
		deadline := time.Now().Add(time.Second)
		for engine.active.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if e = m.Consume(context.Background(), ticket.ID, func(Packet) error { return nil }); e == nil {
			t.Fatal("reused capability")
		}
		m.Stop(ticket.ID)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("stop did not drain engine")
		}
	}
	if disposed.Load() != 100 || engine.active.Load() != 0 || engine.max.Load() != 1 {
		t.Fatal("session lifecycle leak")
	}
}
func TestManagerConcurrentPrepareAndVaultLock(t *testing.T) {
	engine := &blockingEngine{}
	m := NewManager(engine)
	defer m.Close()
	var unlocked atomic.Bool
	unlocked.Store(true)
	resolver := func(context.Context, func(string, string), func([]Profile)) (Source, error) { return Source{}, nil }
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			ticket, e := m.Prepare(resolver, nil, func() bool { return unlocked.Load() }, "vault")
			if e == nil {
				_ = m.Consume(context.Background(), ticket.ID, func(Packet) error { return nil })
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	unlocked.Store(false)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("vault lock did not close stream")
	}
	if engine.max.Load() > 1 || engine.active.Load() != 0 {
		t.Fatal("overlapping engines")
	}
}
func TestMuxRejectsMalformedAndOversized(t *testing.T) {
	for _, au := range [][][]byte{{{}}, {make([]byte, MaxFrame+1)}, {{0x67, 1}, {0x68, 1}, {0x65, 1}}} {
		m := videoMux{sink: &captureSink{}}
		if e := m.accessUnit(au, 0); e == nil {
			t.Fatal("bad AU accepted")
		}
	}
}

func TestMJPEGAndCancellationDuringConnect(t *testing.T) {
	var frame bytes.Buffer
	_ = jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 64, 36)), nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			<-r.Context().Done()
			return
		}
		writer := multipart.NewWriter(w)
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+writer.Boundary())
		for range 3 {
			part, _ := writer.CreatePart(textproto.MIMEHeader{"Content-Type": []string{"image/jpeg"}})
			_, _ = part.Write(frame.Bytes())
		}
		_ = writer.Close()
	}))
	defer server.Close()
	s, _ := NewSource("127.0.0.1", server.URL, "mjpeg", "", vault.Secret{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	sink := &captureSink{cancel: cancel, limit: 2}
	_ = NativeEngine{}.Play(ctx, s, sink)
	cancel()
	if len(sink.packets) != 2 || sink.info.Transport != "HTTP multipart MJPEG" {
		t.Fatal("MJPEG frames missing")
	}
	s, _ = NewSource("127.0.0.1", server.URL+"/stall", "snapshot", "", vault.Secret{})
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if e := (NativeEngine{}).Play(ctx, s, &captureSink{}); !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("cancel blocked", e)
	}
}

func TestManagerExpiredHeartbeatAndUnclaimedTicket(t *testing.T) {
	m := NewManager(&blockingEngine{})
	defer m.Close()
	var disposed atomic.Int32
	resolver := func(context.Context, func(string, string), func([]Profile)) (Source, error) { return Source{}, nil }
	ticket, _ := m.Prepare(resolver, func() { disposed.Add(1) }, nil, "synthetic")
	m.active.deadline.Reset(10 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if m.Status(ticket.ID).State != "stopped" || disposed.Load() != 1 {
		t.Fatal("unclaimed credentials retained")
	}
	ticket, _ = m.Prepare(resolver, nil, nil, "synthetic")
	done := make(chan struct{})
	go func() { _ = m.Consume(context.Background(), ticket.ID, func(Packet) error { return nil }); close(done) }()
	for i := 0; i < 100; i++ {
		m.active.mu.Lock()
		started := !m.active.lastBeat.IsZero()
		if started {
			m.active.lastBeat = time.Now().Add(-8 * time.Second)
		}
		m.active.mu.Unlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dead renderer retained session")
	}
}

func TestSnapshotDigestNonceCountAdvances(t *testing.T) {
	var frame bytes.Buffer
	_ = jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="synthetic", nonce="fixture-nonce", algorithm=MD5, qop="auth"`)
			w.WriteHeader(401)
			return
		}
		n := count.Add(1)
		if !strings.Contains(auth, fmt.Sprintf("nc=%08x", n)) {
			t.Errorf("nonce count did not advance")
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(frame.Bytes())
	}))
	defer server.Close()
	s, _ := NewSource("127.0.0.1", server.URL, "snapshot", "", vault.Secret{Username: "fixture", Password: "synthetic-password"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sink := &captureSink{cancel: cancel, limit: 2}
	_ = NativeEngine{}.Play(ctx, s, sink)
	if count.Load() != 2 {
		t.Fatal("snapshot refresh did not complete")
	}
}
