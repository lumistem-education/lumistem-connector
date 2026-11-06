package job

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusTimeout Status = "timeout"
)

type Job struct {
	ID        string
	Status    Status
	Workspace string
	CreatedAt time.Time
}

type Manager struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	baseDir string
}

func NewManager(baseDir string) *Manager {
	return &Manager{
		jobs:    make(map[string]*Job),
		baseDir: baseDir,
	}
}

func (m *Manager) CreateJob() (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	ws := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	job := &Job{
		ID:        id,
		Status:    StatusQueued,
		Workspace: ws,
		CreatedAt: time.Now(),
	}
	m.jobs[id] = job
	return job, nil
}

func (m *Manager) Get(id string) (*Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return nil
	}

	if err := os.RemoveAll(j.Workspace); err != nil {
		return fmt.Errorf("remove workspace: %w", err)
	}
	delete(m.jobs, id)
	return nil
}
