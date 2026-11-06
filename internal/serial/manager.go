package serial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/google/uuid"
)

const readBufferSize = 4096

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	cliPath  string
	cfgFile  string
}

func NewManager(cliPath, cfgFile string) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		cliPath:  cliPath,
		cfgFile:  cfgFile,
	}
}

func (m *Manager) Open(port string, baudRate int) (*Session, error) {
	cliPath := m.cliPath
	if cliPath == "" {
		cliPath = "arduino-cli"
	}
	if baudRate <= 0 {
		baudRate = 9600
	}

	args := []string{}
	if m.cfgFile != "" {
		args = append(args, "--config-file", m.cfgFile)
	}
	args = append(args,
		"monitor",
		"-p", port,
		"--config", fmt.Sprintf("baudrate=%d", baudRate),
		"--quiet",
		"--raw",
		"--no-color",
	)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, cliPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	session := &Session{
		id:     uuid.New().String(),
		cmd:    cmd,
		stdin:  stdin,
		cancel: cancel,
		done:   make(chan struct{}),
		subs:   make(map[int]chan []byte),
	}

	go session.readLoop(stdout)

	m.mu.Lock()
	m.sessions[session.id] = session
	m.mu.Unlock()

	return session, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if !ok {
		return errors.New("session not found")
	}

	return session.Close()
}

type Session struct {
	id     string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	subs   map[int]chan []byte
	nextID int
	closed bool
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Subscribe() (<-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++
	ch := make(chan []byte, 64)
	s.subs[id] = ch

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if existing, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(existing)
		}
	}

	return ch, unsubscribe
}

func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return errors.New("session closed")
	}

	_, err := s.stdin.Write(data)
	return err
}

func (s *Session) Close() error {
	s.cancel()
	_ = s.stdin.Close()
	<-s.done
	_ = s.cmd.Wait()
	return nil
}

func (s *Session) readLoop(stdout io.Reader) {
	defer func() {
		s.finish()
		close(s.done)
	}()

	buf := make([]byte, readBufferSize)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.broadcast(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) broadcast(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ch := range s.subs {
		select {
		case ch <- chunk:
		default:
		}
	}
}

func (s *Session) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
	}
}
