// Package testcamera provides a loopback-only synthetic RTSP source for tests.
// It is never imported by the desktop application or normal CLI.
package testcamera

import (
	"context"
	"embed"
	"net"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"
)

//go:embed testdata/synthetic.h264 testdata/synthetic.h265
var fixtures embed.FS

func AccessUnits(hevc bool) ([][][]byte, error) {
	name := "testdata/synthetic.h264"
	if hevc {
		name = "testdata/synthetic.h265"
	}
	b, e := fixtures.ReadFile(name)
	if e != nil {
		return nil, e
	}
	var nalus h264.AnnexB
	if e = nalus.Unmarshal(b); e != nil {
		return nil, e
	}
	var out [][][]byte
	var current [][]byte
	for _, n := range nalus {
		aud := n[0]&31 == 9
		if hevc {
			aud = (n[0]>>1)&63 == 35
		}
		if aud {
			if len(current) > 0 {
				out = append(out, current)
				current = nil
			}
			continue
		}
		current = append(current, n)
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out, nil
}

type Camera struct {
	URL         string
	Connections atomic.Int32
	Requests    atomic.Int32
	Redirect    string
	server      *gortsplib.Server
	stream      *gortsplib.ServerStream
	cancel      context.CancelFunc
	done        chan struct{}
	auth        bool
}

func New(hevc, auth bool, redirect string) (*Camera, error) {
	units, e := AccessUnits(hevc)
	if e != nil {
		return nil, e
	}
	var sps, pps, vps []byte
	for _, n := range units[0] {
		if hevc {
			switch (n[0] >> 1) & 63 {
			case 32:
				vps = n
			case 33:
				sps = n
			case 34:
				pps = n
			}
		} else {
			switch n[0] & 31 {
			case 7:
				sps = n
			case 8:
				pps = n
			}
		}
	}
	var f format.Format
	var encode func([][]byte) ([]*rtp.Packet, error)
	if hevc {
		v := &format.H265{PayloadTyp: 96, VPS: vps, SPS: sps, PPS: pps}
		f = v
		enc, e := v.CreateEncoder()
		if e != nil {
			return nil, e
		}
		encode = enc.Encode
	} else {
		v := &format.H264{PayloadTyp: 96, SPS: sps, PPS: pps, PacketizationMode: 1}
		f = v
		enc, e := v.CreateEncoder()
		if e != nil {
			return nil, e
		}
		encode = enc.Encode
	}
	c := &Camera{auth: auth, Redirect: redirect, done: make(chan struct{})}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	c.URL = "rtsp://" + listener.Addr().String() + "/Streaming/channels/102"
	c.server = &gortsplib.Server{RTSPAddress: listener.Addr().String(), Handler: c, Listen: func(string, string) (net.Listener, error) { return listener, nil }}
	if e = c.server.Start(); e != nil {
		listener.Close()
		return nil, e
	}
	media := &description.Media{Type: description.MediaTypeVideo, Formats: []format.Format{f}}
	c.stream = &gortsplib.ServerStream{Server: c.server, Desc: &description.Session{Medias: []*description.Media{media}}}
	if e = c.stream.Initialize(); e != nil {
		c.server.Close()
		return nil, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go func() {
		defer close(c.done)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		var index uint32
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				packets, e := encode(units[int(index)%len(units)])
				if e != nil {
					return
				}
				for _, p := range packets {
					p.Timestamp = index * 9000
					_ = c.stream.WritePacketRTP(media, p)
				}
				index++
			}
		}
	}()
	return c, nil
}
func (c *Camera) Close()                                             { c.cancel(); <-c.done; c.stream.Close(); c.server.Close() }
func (c *Camera) OnConnOpen(*gortsplib.ServerHandlerOnConnOpenCtx)   { c.Connections.Add(1) }
func (c *Camera) OnConnClose(*gortsplib.ServerHandlerOnConnCloseCtx) { c.Connections.Add(-1) }
func (c *Camera) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	c.Requests.Add(1)
	if c.Redirect != "" {
		return &base.Response{StatusCode: 302, Header: base.Header{"Location": base.HeaderValue{c.Redirect}}}, nil, nil
	}
	if c.auth && !ctx.Conn.VerifyCredentials(ctx.Request, "fixture", "synthetic-password") {
		return &base.Response{StatusCode: 401}, nil, liberrors.ErrServerAuth{}
	}
	return &base.Response{StatusCode: 200}, c.stream, nil
}
func (c *Camera) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	if c.auth && !ctx.Conn.VerifyCredentials(ctx.Request, "fixture", "synthetic-password") {
		return &base.Response{StatusCode: 401}, nil, liberrors.ErrServerAuth{}
	}
	return &base.Response{StatusCode: 200}, c.stream, nil
}
func (c *Camera) OnPlay(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return &base.Response{StatusCode: 200}, nil
}
