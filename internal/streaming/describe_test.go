package streaming

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"testing"
	"time"
)

func TestDescribeReusesTransportAndAuthenticationBoundary(t *testing.T) {
	camera, e := testcamera.New(false, true, "")
	if e != nil {
		t.Fatal(e)
	}
	defer camera.Close()
	for _, password := range []string{"synthetic-password", "wrong"} {
		source, e := NewSource("127.0.0.1", camera.URL, "rtsp", "", vault.Secret{Username: "fixture", Password: password})
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		d, e := Describe(ctx, source)
		cancel()
		if password == "wrong" {
			if !errors.Is(e, ErrAuthentication) {
				t.Fatal(e)
			}
			continue
		}
		if e != nil || d.Code != 200 || len(d.Codecs) == 0 || d.Codecs[0] != "H264" {
			t.Fatal(d, e)
		}
	}
}
