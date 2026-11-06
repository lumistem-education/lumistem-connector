package events

import (
    "bytes"
    "strings"
)

type StreamWriter struct {
    hub   *Hub
    jobID string
}

func NewStreamWriter(h *Hub, jobID string) *StreamWriter {
    return &StreamWriter{hub: h, jobID: jobID}
}

func (s *StreamWriter) Write(p []byte) (n int, err error) {
    // split into lines and send each as log event
    buf := bytes.NewBuffer(p)
    for {
        line, err := buf.ReadString('\n')
        if line != "" {
            line = strings.TrimRight(line, "\r\n")
            _ = s.hub.SendEvent(s.jobID, NewEvent(EventLog, line))
        }
        if err != nil {
            break
        }
    }
    return len(p), nil
}
