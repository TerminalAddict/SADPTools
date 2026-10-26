package cameraip

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

func probeRTSP(parent context.Context, address string, port int) bool {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	conn, e := (&net.Dialer{}).DialContext(ctx, "tcp4", net.JoinHostPort(address, strconv.Itoa(port)))
	if e != nil {
		return false
	}
	defer conn.Close()
	return rtspReply(ctx, conn, address, port)
}
func rtspReply(ctx context.Context, conn net.Conn, address string, port int) bool {
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	_, e := fmt.Fprintf(conn, "OPTIONS rtsp://%s/ RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: SADPTools\r\n\r\n", net.JoinHostPort(address, strconv.Itoa(port)))
	if e != nil {
		return false
	}
	// Read only the bounded status line. Never authenticate, follow a redirect,
	// DESCRIBE, SETUP or PLAY from this service-presence check.
	line, e := bufio.NewReaderSize(conn, 4096).ReadSlice('\n')
	if e != nil {
		return false
	}
	fields := strings.Fields(string(line))
	if len(fields) < 2 || fields[0] != "RTSP/1.0" {
		return false
	}
	status, e := strconv.Atoi(fields[1])
	return e == nil && status >= 100 && status <= 599
}
