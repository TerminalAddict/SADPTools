package remote

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"golang.org/x/crypto/ssh"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Synthetic loopback SSH server. Never reaches a real remote host/device.
func fixtureServer(t *testing.T, mode string) (Target, <-chan string) {
	t.Helper()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(key)
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if c.User() != "fixture" || string(p) != "synthetic-login" {
			return nil, errors.New("denied")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var clients []net.Conn
	commands := make(chan string, 20)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			raw, e := ln.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			clients = append(clients, raw)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer raw.Close()
				conn, chans, reqs, e := ssh.NewServerConn(raw, cfg)
				if e != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					if ch.ChannelType() != "session" {
						ch.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, e := ch.Accept()
					if e != nil {
						return
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer channel.Close()
						for req := range requests {
							if req.Type == "signal" {
								return
							}
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							var args struct{ Command string }
							if ssh.Unmarshal(req.Payload, &args) != nil {
								return
							}
							commands <- args.Command
							req.Reply(true, nil)
							code := uint32(0)
							var output any
							if strings.Contains(args.Command, "capabilities") {
								output = Capabilities{Schema: Schema, Version: "synthetic", Discovery: true}
								if mode == "schema" {
									output = Capabilities{Schema: 99, Version: "future", Discovery: true}
								}
								if mode == "missing" {
									code = 127
									output = "missing CLI"
								}
							} else {
								if mode == "hang" {
									continue
								}
								output = Report{Schema: Schema, Version: "synthetic", At: time.Now().UTC().Format(time.RFC3339), Devices: []devices.Device{{ID: "serial:fixture-camera", Serial: "fixture-camera", IPv4: "192.0.2.8", Interfaces: []devices.Interface{{Name: "fixture0"}}}}}
								if mode == "partial" {
									code = 3
									r := output.(Report)
									r.Partial = true
									r.Warnings = []string{"synthetic adapter warning"}
									output = r
								}
								if mode == "malformed" {
									output = "not report"
								}
							}
							json.NewEncoder(channel).Encode(output)
							channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
							return
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		for _, c := range clients {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	return Target{Host: host, Port: p, Username: "fixture", Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()), Executable: "/opt/sadptools", Interface: "fixture0", Protocols: "sadp,onvif", Timeout: 1}, commands
}
func TestSSHReadOnlyProtocolAndHostVerification(t *testing.T) {
	for _, mode := range []string{"ok", "partial", "schema", "missing", "malformed", "wrong-key", "hang"} {
		t.Run(mode, func(t *testing.T) {
			target, commands := fixtureServer(t, mode)
			if mode == "wrong-key" {
				target.Fingerprint = "SHA256:" + strings.Repeat("A", 43)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if mode == "hang" {
				ctx, cancel = context.WithTimeout(context.Background(), 200*time.Millisecond)
			}
			defer cancel()
			started := time.Now()
			ob, e := Run(ctx, target, vault.Secret{Password: "synthetic-login"})
			if mode == "ok" || mode == "partial" {
				if e != nil || len(ob.Report.Devices) != 1 || ob.Report.Partial != (mode == "partial") {
					t.Fatal(ob, e)
				}
				for range 2 {
					command := <-commands
					if strings.Contains(command, "synthetic-login") || strings.Contains(command, " send ") {
						t.Fatal("unsafe command", command)
					}
				}
			} else if e == nil {
				t.Fatal("expected error", mode)
			}
			if time.Since(started) > 2*time.Second {
				t.Fatal("cancellation/connection not bounded")
			}
		})
	}
}
func TestHostileArgumentsAndBoundedSchema(t *testing.T) {
	target, _ := fixtureServer(t, "ok")
	for _, bad := range []string{"eth0;whoami", "$(id)", "eth0\nrm -f x", "`id`", "--debug x"} {
		x := target
		x.Interface = bad
		if _, _, e := Commands(x); e == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	for _, bad := range []string{"{}", "[]", "{\"schema\":2,\"version\":\"x\"}", strings.Repeat("x", MaxOutput+1)} {
		if _, e := Parse([]byte(bad)); e == nil {
			t.Fatal("invalid report accepted")
		}
	}
	good := Report{Schema: 1, Version: "fixture", Devices: []devices.Device{}}
	b, _ := json.Marshal(good)
	if _, e := Parse(append(b, []byte(" {}")...)); e == nil {
		t.Fatal("trailing JSON accepted")
	}
}
