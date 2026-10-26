package cameraip

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRTSPPresenceUsesOnlyOPTIONS(t *testing.T) {
	for _, tc := range []struct {
		reply string
		want  bool
	}{{"RTSP/1.0 200 OK\r\n", true}, {"RTSP/1.0 401 Unauthorized\r\n", true}, {"RTSP/1.0 302 Redirect\r\n", true}, {"HTTP/1.1 200 OK\r\n", false}, {"RTSP/1.0 abc Bad\r\n", false}, {strings.Repeat("x", 5000) + "\n", false}} {
		client, server := net.Pipe()
		done := make(chan string, 1)
		go func() {
			defer server.Close()
			r := bufio.NewReader(server)
			var request strings.Builder
			for {
				line, e := r.ReadString('\n')
				if e != nil {
					done <- request.String()
					return
				}
				request.WriteString(line)
				if line == "\r\n" {
					break
				}
			}
			done <- request.String()
			server.Write([]byte(tc.reply))
		}()
		got := rtspReply(context.Background(), client, "192.0.2.10", 8554)
		client.Close()
		request := <-done
		if got != tc.want {
			t.Errorf("reply %.40q: got %v", tc.reply, got)
		}
		if request != "OPTIONS rtsp://192.0.2.10:8554/ RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: SADPTools\r\n\r\n" {
			t.Errorf("unexpected request %q", request)
		}
	}
}
func TestRTSPCancellationUnblocksIO(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- rtspReply(ctx, client, "192.0.2.10", 554) }()
	cancel()
	select {
	case got := <-done:
		if got {
			t.Fatal("cancelled probe reported response")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close socket")
	}
}
