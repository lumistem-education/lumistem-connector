package events

import (
    "errors"
    "sync"
)

type HubClient interface {
    Send(Event) error
    Close() error
}

type Hub struct {
    mu      sync.Mutex
    clients map[string]HubClient // jobID -> client
}

func NewHub() *Hub {
    return &Hub{clients: make(map[string]HubClient)}
}

// Register registers a client for a jobID
func (h *Hub) Register(jobID string, c HubClient) error {
    h.mu.Lock()
    defer h.mu.Unlock()
    if _, ok := h.clients[jobID]; ok {
        return errors.New("client already connected for this job")
    }
    h.clients[jobID] = c
    return nil
}

func (h *Hub) Unregister(jobID string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if c, ok := h.clients[jobID]; ok {
        _ = c.Close()
        delete(h.clients, jobID)
    }
}

func (h *Hub) SendEvent(jobID string, ev Event) error {
    h.mu.Lock()
    c, ok := h.clients[jobID]
    h.mu.Unlock()
    if !ok {
        return errors.New("no client for job")
    }
    return c.Send(ev)
}
