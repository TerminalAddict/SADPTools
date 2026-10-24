package streaming

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type Status struct {
	ID               string    `json:"id"`
	State            string    `json:"state"`
	Message          string    `json:"message"`
	Info             Info      `json:"info"`
	Profiles         []Profile `json:"profiles"`
	Bytes            uint64    `json:"bytes"`
	Fragments        uint64    `json:"fragments"`
	Started          time.Time `json:"started"`
	CredentialSource string    `json:"credentialSource"`
}
type Resolver func(context.Context, func(string, string), func([]Profile)) (Source, error)
type Ticket struct {
	ID string `json:"id"`
}
type Session struct {
	mu       sync.Mutex
	status   Status
	ctx      context.Context
	cancel   context.CancelFunc
	resolve  Resolver
	dispose  func()
	alive    func() bool
	claimed  bool
	deadline *time.Timer
	lastBeat time.Time
	done     chan struct{}
	started  bool
}
type Manager struct {
	lifecycle sync.Mutex
	mu        sync.Mutex
	active    *Session
	closed    bool
	engine    Engine
}

func NewManager(engine Engine) *Manager {
	if engine == nil {
		engine = NativeEngine{}
	}
	return &Manager{engine: engine}
}

func (m *Manager) Prepare(resolve Resolver, dispose func(), alive func() bool, credentialSource string) (Ticket, error) {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.Stop("")
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		select {
		case <-m.active.done:
		default:
			if dispose != nil {
				dispose()
			}
			return Ticket{}, errors.New("previous preview is still stopping; try again shortly")
		}
	}
	if m.closed {
		if dispose != nil {
			dispose()
		}
		return Ticket{}, errors.New("Live View is closing")
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		if dispose != nil {
			dispose()
		}
		return Ticket{}, errors.New("could not create a private preview session")
	}
	id := hex.EncodeToString(b)
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{ctx: ctx, cancel: cancel, resolve: resolve, dispose: dispose, alive: alive, done: make(chan struct{}), status: Status{ID: id, State: "ready", Message: "Waiting for the player", CredentialSource: credentialSource, Profiles: []Profile{}}}
	m.active = s
	s.deadline = time.AfterFunc(20*time.Second, func() { m.Stop(id) })
	return Ticket{id}, nil
}
func (m *Manager) Status(id string) Status {
	m.mu.Lock()
	s := m.active
	m.mu.Unlock()
	if s == nil {
		return Status{State: "idle", Profiles: []Profile{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" && id != s.status.ID {
		return Status{State: "stopped", Profiles: []Profile{}}
	}
	out := s.status
	out.Profiles = append([]Profile{}, out.Profiles...)
	return out
}
func (m *Manager) Heartbeat(id string, playing bool) {
	m.mu.Lock()
	s := m.active
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.ID == id {
		s.lastBeat = time.Now()
		if playing && s.status.Info.MIME != "" && (s.status.State == "buffering" || s.status.State == "playing") {
			s.status.State = "playing"
			s.status.Message = "Playing video"
		}
	}
}
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s := m.active
	if s == nil || (id != "" && s.status.ID != id) {
		m.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.cancel()
	s.deadline.Stop()
	if !s.started {
		s.started = true
		if s.dispose != nil {
			s.dispose()
			s.dispose = nil
		}
		s.resolve = nil
		close(s.done)
	}
	s.status.State = "stopped"
	s.status.Message = "Preview stopped"
	s.mu.Unlock()
	m.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(4 * time.Second):
	}
}
func (m *Manager) Close() {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.Stop("")
}

// Consume is the only way to claim a ticket. It is single-use and the parent
// context must belong to the owning Wails stream, not to an HTTP listener.
func (m *Manager) Consume(parent context.Context, id string, send func(Packet) error) error {
	m.mu.Lock()
	s := m.active
	if m.closed || s == nil || s.status.ID != id {
		m.mu.Unlock()
		return errors.New("preview session is unavailable")
	}
	s.mu.Lock()
	if s.claimed || s.started || s.ctx.Err() != nil {
		s.mu.Unlock()
		m.mu.Unlock()
		return errors.New("preview session expired or was already claimed")
	}
	s.claimed = true
	s.started = true
	s.lastBeat = time.Now()
	s.status.Started = time.Now()
	s.deadline.Stop()
	s.mu.Unlock()
	m.mu.Unlock()
	stop := context.AfterFunc(parent, s.cancel)
	defer stop()
	defer func() {
		s.cancel()
		s.mu.Lock()
		if s.dispose != nil {
			s.dispose()
			s.dispose = nil
		}
		s.resolve = nil
		s.mu.Unlock()
		close(s.done)
	}()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-tick.C:
				s.mu.Lock()
				expired := time.Since(s.lastBeat) > 7*time.Second
				s.mu.Unlock()
				if expired {
					s.state("stopped", "Player disconnected")
					s.cancel()
					return
				}
				if s.alive != nil && !s.alive() {
					s.state("stopped", "Credential vault locked; unlock it before reopening Live View")
					s.cancel()
					return
				}
			}
		}
	}()
	if s.alive != nil && !s.alive() {
		s.state("stopped", "Credential vault locked; unlock it before reopening Live View")
		return nil
	}
	source, e := s.resolve(s.ctx, s.state, s.profiles)
	if e == nil {
		defer source.secret.Clear()
		s.state("connecting", "Connecting to the selected camera stream")
		sink := &sessionSink{session: s, send: send}
		e = m.engine.Play(s.ctx, source, sink)
	}
	if s.ctx.Err() != nil {
		s.mu.Lock()
		if s.status.State != "stopped" {
			s.status.State = "stopped"
			s.status.Message = "Preview stopped"
		}
		s.mu.Unlock()
		return nil
	}
	if e != nil {
		s.state("error", e.Error())
	}
	return nil
}
func (s *Session) state(state, message string) {
	s.mu.Lock()
	s.status.State = state
	s.status.Message = message
	s.mu.Unlock()
}
func (s *Session) profiles(p []Profile) {
	s.mu.Lock()
	s.status.Profiles = append([]Profile{}, p...)
	s.mu.Unlock()
}

type sessionSink struct {
	session *Session
	send    func(Packet) error
}

func (s *sessionSink) Information(info Info) error {
	s.session.mu.Lock()
	if s.session.status.Info == info {
		s.session.mu.Unlock()
		return nil
	}
	s.session.status.Info = info
	s.session.status.State = "buffering"
	s.session.status.Message = "Receiving video; waiting for the player"
	s.session.mu.Unlock()
	b, _ := json.Marshal(info)
	return s.send(Packet{Kind: 0, Data: b})
}
func (s *sessionSink) Write(p Packet) error {
	if len(p.Data) > MaxFrame+65536 {
		return ErrMedia
	}
	if e := s.send(p); e != nil {
		return e
	}
	s.session.mu.Lock()
	s.session.status.Bytes += uint64(len(p.Data))
	s.session.status.Fragments++
	s.session.mu.Unlock()
	return nil
}
