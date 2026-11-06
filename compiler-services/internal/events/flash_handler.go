package events

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/job"
	"lumistem/compiler-services/internal/logs"
)

func NewFlashHandler(cfg *config.Config, h *Hub, jm *job.Manager) http.HandlerFunc {
	sem := make(chan struct{}, cfg.MaxConcurrentJobs)

	return func(w http.ResponseWriter, r *http.Request) {
		// expected: /flash/<jobID>
		jobID := path.Base(r.URL.Path)

		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		verifiedJobID, err := job.VerifySSEToken(cfg.JWTSecret, token)
		if err != nil || verifiedJobID != jobID {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		port := r.URL.Query().Get("port")
		if port == "" {
			http.Error(w, "missing port", http.StatusBadRequest)
			return
		}

		uploadSpeed := 0
		if raw := r.URL.Query().Get("upload_speed"); raw != "" {
			speed, err := strconv.Atoi(raw)
			if err != nil || speed <= 0 || speed > 4000000 {
				http.Error(w, "invalid upload_speed", http.StatusBadRequest)
				return
			}
			uploadSpeed = speed
		}

		jobObj, ok := jm.Get(jobID)
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
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

		client := NewClient(w)
		if err := h.Register(jobID, client); err != nil {
			return
		}
		defer h.Unregister(jobID)

		metaPath := filepath.Join(jobObj.Workspace, "meta.json")
		var meta struct {
			Board string `json:"board"`
		}
		if b, err := os.ReadFile(metaPath); err == nil {
			_ = json.Unmarshal(b, &meta)
		}

		sw := NewStreamWriter(h, jobID)

		go func() {
			select {
			case sem <- struct{}{}:
			case <-r.Context().Done():
				_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]interface{}{"success": false}))
				_ = client.Close()
				return
			}
			defer func() { <-sem }()

			cfg.ArduinoMu.RLock()
			defer cfg.ArduinoMu.RUnlock()
			ctx, cancel := context.WithTimeout(r.Context(), cfg.UploadTimeout)
			defer cancel()

			if _, err := arduino.Upload(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, jobObj.Workspace, meta.Board, port, "sketch", uploadSpeed, sw); err != nil {
				if r.Context().Err() == nil {
					logs.Error("Upload failed (job=%s)", jobID)
					_ = h.SendEvent(jobID, NewEvent(EventError, "upload failed"))
				}
				_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]interface{}{"success": false}))
				_ = client.Close()
				return
			}

			if err := jm.Delete(jobID); err != nil {
				logs.Error("Delete workspace (job=%s): %v", jobID, err)
			}

			_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]interface{}{"success": true}))
			_ = client.Close()
		}()

		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-client.Done():
				return
			case <-ticker.C:
				if err := client.Ping(); err != nil {
					return
				}
			}
		}
	}
}
