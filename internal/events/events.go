package events

import "time"

type EventType string

const (
    EventLog      EventType = "log"
    EventProgress EventType = "progress"
    EventNotice   EventType = "notice"
    EventError    EventType = "error"
    EventDone     EventType = "done"
)

type Event struct {
    Type EventType   `json:"type"`
    Data interface{} `json:"data"`
    TS   int64       `json:"ts"`
}

func NewEvent(t EventType, data interface{}) Event {
    return Event{Type: t, Data: data, TS: time.Now().Unix()}
}
