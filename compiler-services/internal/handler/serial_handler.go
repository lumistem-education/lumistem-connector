package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"lumistem/compiler-services/internal/model"
	"lumistem/compiler-services/internal/serial"
)

func NewOpenSerialHandler(sm *serial.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.OpenSerialRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Port == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid request"})
			return
		}

		session, err := sm.Open(req.Port, req.BaudRate)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "open port: " + err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.OpenSerialResponse{SessionID: session.ID()})
	}
}

func NewWriteSerialHandler(sm *serial.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := sm.Get(r.PathValue("sessionID"))
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "session not found"})
			return
		}

		var req model.WriteSerialRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid request"})
			return
		}

		data, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid data"})
			return
		}

		if err := session.Write(data); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "write port: " + err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SuccessResponse{Message: "data sent"})
	}
}

func NewCloseSerialHandler(sm *serial.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := sm.Close(r.PathValue("sessionID")); err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "session not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SuccessResponse{Message: "session closed"})
	}
}

func NewReadSerialHandler(sm *serial.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := sm.Get(r.PathValue("sessionID"))
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		rc := http.NewResponseController(w)
		if err := rc.Flush(); err != nil {
			return
		}

		chunks, unsubscribe := session.Subscribe()
		defer unsubscribe()

		for {
			select {
			case <-r.Context().Done():
				return
			case chunk, open := <-chunks:
				if !open {
					return
				}
				encoded := base64.StdEncoding.EncodeToString(chunk)
				if _, err := fmt.Fprintf(w, "event: data\ndata: %s\n\n", encoded); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
		}
	}
}
