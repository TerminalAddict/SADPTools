package streaming

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	"github.com/bluenviron/gortsplib/v5/pkg/liberrors"
	"github.com/pion/rtp"
)

type NativeEngine struct{}

func (NativeEngine) Play(ctx context.Context, s Source, sink Sink) error {
	if s.mode != "rtsp" {
		return playHTTP(ctx, s, sink)
	}
	return playRTSP(ctx, s, sink)
}

func playRTSP(ctx context.Context, s Source, sink Sink) error {
	parsed, err := ValidateURL(s.address, s.raw)
	if err != nil {
		return err
	}
	expected := net.JoinHostPort(s.address, endpointPort(parsed))
	parsed.Host = expected
	if s.secret.Username != "" {
		parsed.User = url.UserPassword(s.secret.Username, s.secret.Password)
	}
	u, err := base.ParseURL(parsed.String())
	if err != nil {
		return ErrEndpoint
	}
	tlsConfig, err := TLSConfig(s.pin)
	if err != nil {
		return err
	}
	protocol := gortsplib.ProtocolTCP
	var redirected atomic.Bool
	c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &protocol, ReadTimeout: 5 * time.Second, WriteTimeout: 3 * time.Second, TLSConfig: tlsConfig, UserAgent: "SADPTools Live View", WriteQueueSize: 16,
		DialContext: func(inner context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != expected {
				return nil, ErrEndpoint
			}
			dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			stop := context.AfterFunc(inner, cancel)
			defer stop()
			return (&net.Dialer{}).DialContext(dialCtx, "tcp4", expected)
		},
		OnResponse: func(r *base.Response) {
			if r.StatusCode >= 300 && r.StatusCode < 400 {
				redirected.Store(true)
				delete(r.Header, "Location")
			}
		},
		OnTransportSwitch: func(error) {}, OnDecodeError: func(error) {}, OnPacketsLost: func(uint64) {},
	}
	if err = c.Start(); err != nil {
		return connectionError(ctx, err)
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, c.Close)
	defer stop()
	desc, _, err := c.Describe(u)
	if redirected.Load() {
		return ErrEndpoint
	}
	if err != nil {
		return connectionError(ctx, err)
	}
	if len(desc.Medias) > 16 || desc.BaseURL == nil {
		return ErrMedia
	}
	// Even though RTP stays on our one TCP connection, reject untrusted SDP
	// control URLs before generating a SETUP request with credentials.
	validControl := func(raw string) bool {
		v, e := url.Parse(raw)
		if e != nil {
			return false
		}
		if v.IsAbs() {
			v.User = nil
			_, e = ValidateURL(s.address, v.String())
			return e == nil && v.Scheme == parsed.Scheme && net.JoinHostPort(v.Hostname(), endpointPort(v)) == expected
		}
		return len(raw) < 2048 && v.Host == ""
	}
	baseURL := desc.BaseURL.Clone()
	baseURL.User = nil
	if !validControl(baseURL.String()) {
		return ErrEndpoint
	}
	var a *format.H264
	var b *format.H265
	var media *description.Media
	var selected format.Format
	mux := &videoMux{sink: sink}
	var decode func(*rtp.Packet) ([][]byte, error)
	if media = desc.FindFormat(&a); media != nil {
		selected = a
		mux.sps, mux.pps = a.SafeParams()
		dec, e := a.CreateDecoder()
		if e != nil {
			return ErrMedia
		}
		decode = dec.Decode
	} else if media = desc.FindFormat(&b); media != nil {
		selected = b
		mux.hevc = true
		mux.vps, mux.sps, mux.pps = b.SafeParams()
		dec, e := b.CreateDecoder()
		if e != nil {
			return ErrMedia
		}
		decode = dec.Decode
	} else {
		return ErrCodec
	}
	if len(mux.sps) > 4096 || len(mux.pps) > 4096 || len(mux.vps) > 4096 {
		return ErrMedia
	}
	if !validControl(media.Control) {
		return ErrEndpoint
	}
	if _, err = c.Setup(desc.BaseURL, media, 0, 0); err != nil {
		return connectionError(ctx, err)
	}
	failures := make(chan error, 1)
	fail := func(e error) {
		select {
		case failures <- e:
		default:
		}
	}
	var packetTime atomic.Int64
	c.OnPacketRTP(media, selected, func(p *rtp.Packet) {
		defer func() {
			if recover() != nil {
				fail(ErrMedia)
			}
		}()
		if ctx.Err() != nil {
			return
		}
		pts, ok := c.PacketPTS(media, p)
		if !ok {
			return
		}
		au, e := decode(p)
		if e != nil {
			if errors.Is(e, rtph264.ErrMorePacketsNeeded) || errors.Is(e, rtph264.ErrNonStartingPacketAndNoPrevious) || errors.Is(e, rtph265.ErrMorePacketsNeeded) || errors.Is(e, rtph265.ErrNonStartingPacketAndNoPrevious) {
				return
			}
			fail(ErrMedia)
			return
		}
		if e = mux.accessUnit(au, pts); e != nil {
			fail(e)
			return
		}
		if mux.started {
			packetTime.Store(time.Now().UnixNano())
		}
	})
	if _, err = c.Play(nil); err != nil {
		return connectionError(ctx, err)
	}
	ended := make(chan error, 1)
	go func() { ended <- c.Wait() }()
	watchdog := time.NewTicker(time.Second)
	defer watchdog.Stop()
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err = <-ended:
			return connectionError(ctx, err)
		case err = <-failures:
			return err
		case <-watchdog.C:
			last := packetTime.Load()
			if last == 0 && time.Since(started) > 12*time.Second {
				return errors.New("no decodable keyframe arrived within 12 seconds; select another stream or check camera encoding")
			}
			if last != 0 && time.Since(time.Unix(0, last)) > 6*time.Second {
				return errors.New("video stalled; check the network and use Retry")
			}
		}
	}
}

func connectionError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrEndpoint) {
		return ErrEndpoint
	}
	var status liberrors.ErrClientBadStatusCode
	if errors.As(err, &status) {
		switch status.Code {
		case 401, 403:
			return ErrAuthentication
		case 404:
			return errors.New("camera stream was not found; check the profile, channel and RTSP path")
		}
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return errors.New("camera TLS certificate is not trusted; inspect and approve its fingerprint before playback")
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return errors.New("camera connection timed out; check the RTSP port, routing and firewall")
	}
	return errors.New("camera connection closed or was refused; check credentials, stream address and network, then Retry")
}
