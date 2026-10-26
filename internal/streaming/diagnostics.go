package streaming

import (
	"errors"
	"time"
)

// Metrics describe observed RTP packets/access units and delivered media bytes.
// They are never labelled as decoded frames or camera encoder targets.
type Metrics struct {
	Source         string   `json:"source"`
	ElapsedSeconds float64  `json:"elapsedSeconds"`
	RTPPackets     *uint64  `json:"rtpPackets"`
	RTPBytes       *uint64  `json:"rtpBytes"`
	AccessUnits    *uint64  `json:"accessUnits"`
	LostPackets    *uint64  `json:"lostPackets"`
	DecodeErrors   *uint64  `json:"decodeErrors"`
	RTPMbps        *float64 `json:"rtpMbps"`
	EncodedFPS     *float64 `json:"encodedFPS"`
	DeliveredMbps  *float64 `json:"deliveredMbps"`
	FirstMediaMS   *float64 `json:"firstMediaMS"`
	FirstPlayingMS *float64 `json:"firstPlayingMS"`
	NominalFPS     *float64 `json:"nominalFPS"`
	DecodedDrops   *uint64  `json:"decodedDrops"`
}
type Transition struct {
	State     string    `json:"state"`
	Message   string    `json:"message"`
	At        time.Time `json:"at"`
	ElapsedMS float64   `json:"elapsedMS"`
}
type observer interface {
	Observe(packetBytes, units, lost, decodeErrors uint64)
}

func observe(sink Sink, packetBytes, units, lost, decodeErrors uint64) {
	if o, ok := sink.(observer); ok {
		o.Observe(packetBytes, units, lost, decodeErrors)
	}
}
func (s *sessionSink) Observe(packetBytes, units, lost, decodeErrors uint64) {
	x := s.session
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.firstPacket.IsZero() {
		x.firstPacket = time.Now()
	}
	x.rtpAvailable = true
	x.packets += boolCount(packetBytes > 0)
	x.packetBytes += packetBytes
	x.units += units
	x.lost += lost
	x.decodeErrors += decodeErrors
}
func boolCount(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}
func ptr[T any](x T) *T { return &x }
func (s *Session) metricsLocked() Metrics {
	end := s.ended
	if end.IsZero() {
		end = time.Now()
	}
	elapsed := end.Sub(s.created).Seconds()
	m := Metrics{Source: "Go RTP packet/access-unit observations; delivered bytes include remux/container overhead", ElapsedSeconds: elapsed, FirstMediaMS: s.firstMediaMS, FirstPlayingMS: s.firstPlayingMS}
	if !s.status.Started.IsZero() && s.status.Bytes > 0 {
		secs := end.Sub(s.status.Started).Seconds()
		if secs > 0 {
			m.DeliveredMbps = ptr(float64(s.status.Bytes) * 8 / secs / 1e6)
		}
	}
	if s.rtpAvailable {
		m.RTPPackets = ptr(s.packets)
		m.RTPBytes = ptr(s.packetBytes)
		m.AccessUnits = ptr(s.units)
		m.LostPackets = ptr(s.lost)
		m.DecodeErrors = ptr(s.decodeErrors)
		seconds := end.Sub(s.firstPacket).Seconds()
		if seconds > 0 {
			m.RTPMbps = ptr(float64(s.packetBytes) * 8 / seconds / 1e6)
			m.EncodedFPS = ptr(float64(s.units) / seconds)
		}
	}
	return m
}
func (s *Session) stateLocked(state, message string) {
	if s.status.State != state || len(s.status.Timeline) == 0 {
		at := time.Now()
		s.status.Timeline = append(s.status.Timeline, Transition{State: state, Message: message, At: at.UTC(), ElapsedMS: float64(at.Sub(s.created).Microseconds()) / 1000})
		if len(s.status.Timeline) > 128 {
			s.status.Timeline = append([]Transition(nil), s.status.Timeline[len(s.status.Timeline)-128:]...)
		}
	}
	s.status.State = state
	s.status.Message = message
	if state == "stopped" || state == "error" {
		if s.ended.IsZero() {
			s.ended = time.Now()
		}
	}
	if state == "playing" && s.firstPlayingMS == nil {
		s.firstPlayingMS = ptr(float64(time.Since(s.created).Microseconds()) / 1000)
	}
}

var ErrTimeout = errors.New("camera connection timed out; check the RTSP port, routing and firewall")
var ErrNoVideo = errors.New("no decodable keyframe arrived within 12 seconds; try an H.264 sub stream and check smart-encoding compatibility")
var ErrTransport = errors.New("camera rejected the requested RTSP transport; this viewer supports interleaved TCP")
var ErrConnection = errors.New("camera connection closed or was refused; check stream address and routing")
var ErrSDP = errors.New("camera returned invalid stream metadata or SDP")

func errorCode(e error) string {
	for _, p := range []struct {
		err  error
		code string
	}{{ErrAuthentication, "authentication"}, {ErrCodec, "codec"}, {ErrTimeout, "timeout"}, {ErrNoVideo, "no-video"}, {ErrTransport, "transport"}, {ErrConnection, "connection"}, {ErrSDP, "sdp"}, {ErrMedia, "media"}, {ErrEndpoint, "endpoint"}} {
		if errors.Is(e, p.err) {
			return p.code
		}
	}
	return "stream"
}
