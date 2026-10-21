package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
)

// Synthetic password/MAC only. execute is always replaced; no packet leaves these tests.
func sendArgs() []string {
	return []string{"0.0.0.0", "activate", "--mac", "02:00:00:00:00:01", "--interface", "Ethernet Test", "--password-stdin"}
}
func TestSendStdinDryRunAndNoSecrets(t *testing.T) {
	args := append(sendArgs(), "--dry-run", "--json")
	var out, stderr bytes.Buffer
	called := false
	code := runSend(context.Background(), args, strings.NewReader("Synthetic-pass1\n"), &out, &stderr, func(context.Context, configuration.Request) (configuration.Result, error) {
		called = true
		return configuration.Result{}, nil
	})
	if code != 0 || called || !strings.Contains(out.String(), `"sent":false`) || strings.Contains(out.String()+stderr.String(), "Synthetic-pass1") {
		t.Fatal(code, out.String(), stderr.String())
	}
}
func TestSendExitCodesAndExactRequest(t *testing.T) {
	for _, tc := range []struct {
		err  error
		exit int
	}{{nil, 0}, {configuration.ErrRejected, 4}, {configuration.ErrUnconfirmed, 5}, {errors.Join(configuration.ErrUnconfirmed, context.Canceled), 130}, {errors.New("port conflict"), 1}} {
		var out, stderr bytes.Buffer
		code := runSend(context.Background(), sendArgs(), strings.NewReader("Synthetic-pass1\r\n"), &out, &stderr, func(_ context.Context, r configuration.Request) (configuration.Result, error) {
			if r.Password != "Synthetic-pass1" || r.Interface != "Ethernet Test" || r.MAC != "02:00:00:00:00:01" {
				t.Fatal("wrong request")
			}
			return configuration.Result{Outcome: "unconfirmed", Message: "Synthetic outcome"}, tc.err
		})
		if code != tc.exit || strings.Contains(out.String()+stderr.String(), "Synthetic-pass1") {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
}
func TestSendRejectsBadInputBeforeEngine(t *testing.T) {
	for _, extra := range [][]string{{"--password", "Another-pass1"}, {"--timeout", "1.5s"}, {"--mac", "bad"}, {"--interface", ""}, {"--surprise", "secret"}, {"extra-positional"}} {
		var out bytes.Buffer
		called := false
		code := runSend(context.Background(), append(sendArgs(), extra...), strings.NewReader("Synthetic-pass1"), &out, &out, func(context.Context, configuration.Request) (configuration.Result, error) {
			called = true
			return configuration.Result{}, nil
		})
		if code != 2 || called || strings.Contains(out.String(), "Synthetic-pass1") {
			t.Fatal(code, called, out.String())
		}
	}
	for _, password := range []string{"short", strings.Repeat("A", 10000), "Synthetic-pass1\nExtra-line"} {
		if got := runSend(context.Background(), sendArgs(), strings.NewReader(password), io.Discard, io.Discard, func(context.Context, configuration.Request) (configuration.Result, error) {
			t.Fatal("bad input sent")
			return configuration.Result{}, nil
		}); got != 2 {
			t.Fatal(got)
		}
	}
}
func TestCodeFileAndEnvironmentSources(t *testing.T) {
	t.Setenv("SADPTOOLS_TEST_PASSWORD", "Synthetic-pass1")
	path := filepath.Join(t.TempDir(), "code.txt")
	if err := os.WriteFile(path, []byte("SYNTHETIC-SUPPORT-CODE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"0.0.0.0", "resetpassword", "--mac", "02:00:00:00:00:01", "--interface", "Ethernet Test", "--password-env", "SADPTOOLS_TEST_PASSWORD", "--code-file", path}
	called := false
	got := runSend(context.Background(), args, nil, io.Discard, io.Discard, func(_ context.Context, r configuration.Request) (configuration.Result, error) {
		called = true
		if r.Password != "Synthetic-pass1" || r.Code != "SYNTHETIC-SUPPORT-CODE" {
			t.Fatal("secret source changed")
		}
		return configuration.Result{}, nil
	})
	if got != 0 || !called {
		t.Fatal(got)
	}
	if err := os.WriteFile(path, []byte("<VendorXML>Unsupported</VendorXML>"), 0600); err != nil {
		t.Fatal(err)
	}
	got = runSend(context.Background(), args, nil, io.Discard, io.Discard, func(context.Context, configuration.Request) (configuration.Result, error) {
		t.Fatal("unsupported XML sent")
		return configuration.Result{}, nil
	})
	if got != 2 {
		t.Fatal(got)
	}
}
