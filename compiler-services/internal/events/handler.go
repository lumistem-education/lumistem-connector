package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/job"
	"lumistem/compiler-services/internal/logs"
)

const pingInterval = 20 * time.Second

func NewCompileHandler(cfg *config.Config, h *Hub, jm *job.Manager) http.HandlerFunc {
	sem := make(chan struct{}, cfg.MaxConcurrentJobs)

	return func(w http.ResponseWriter, r *http.Request) {
		// expected: /compile/<jobID>
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

		jobObj, ok := jm.Get(jobID)
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}

		// build SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering for SSE
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

		// read meta.json for board/fqbn
		metaPath := filepath.Join(jobObj.Workspace, "meta.json")
		var meta struct {
			Board         string `json:"board"`
			BrowserUpload bool   `json:"browserUpload"`
		}
		if b, err := os.ReadFile(metaPath); err == nil {
			_ = json.Unmarshal(b, &meta)
		}

		// forwards arduino-cli output as events
		sw := NewStreamWriter(h, jobID)
		reqCtx := r.Context()

		go func() {
			slotCtx, cancelSlot := context.WithTimeout(reqCtx, cfg.CompileTimeout)
			defer cancelSlot()

			select {
			case sem <- struct{}{}:
			case <-slotCtx.Done():
				if reqCtx.Err() == nil {
					logs.Error("Compile queue timeout (job=%s)", jobID)
					_ = h.SendEvent(jobID, NewEvent(EventError, "compile queue timeout"))
				}
				_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]interface{}{"success": false}))
				_ = client.Close()
				return
			}
			defer func() { <-sem }()

			// Compilation never installs packages. Setup is an explicit user action.
			cfg.ArduinoMu.RLock()
			defer cfg.ArduinoMu.RUnlock()

			compileCtx, cancelCompile := context.WithTimeout(reqCtx, cfg.CompileTimeout)
			defer cancelCompile()

			// compile
			sketchPathRel := "sketch"
			_, err := arduino.Compile(compileCtx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, jobObj.Workspace, meta.Board, sketchPathRel, sw)
			if err != nil {
				if reqCtx.Err() == nil {
					logs.Error("Compile failed (job=%s)", jobID)
					_ = h.SendEvent(jobID, NewEvent(EventError, "compile failed"))
				}
				_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]interface{}{"success": false}))
				_ = client.Close()
				return
			}

			// get artifact path
			artifactPath, err := arduino.FindArtifactPath(jobObj.Workspace)
			if err != nil {
				logs.Error("Find artifact (job=%s): %v", jobID, err)
				_ = h.SendEvent(jobID, NewEvent(EventError, fmt.Sprintf("find artifact: %v", err)))
				_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]interface{}{"success": false}))
				_ = client.Close()
				return
			}

			if meta.BrowserUpload && !strings.HasSuffix(artifactPath, ".merged.bin") {
				_ = h.SendEvent(jobID, NewEvent(EventError, "Browser upload requires an ESP merged binary. Use the local app or a compatible ESP core."))
				_ = h.SendEvent(jobID, NewEvent(EventDone, map[string]any{"success": false}))
				_ = client.Close()
				return
			}

			// build response
			doneData := map[string]interface{}{"success": true}
			if artifactPath != "" {
				doneData["artifactPath"] = artifactPath
				doneData["artifactName"] = filepath.Base(artifactPath)
				downloadToken, tokenErr := job.GenerateDownloadToken(cfg.JWTSecret, jobID, time.Hour)
				if tokenErr != nil {
					logs.Error("Create download token (job=%s): %v", jobID, tokenErr)
					_ = h.SendEvent(jobID, NewEvent(EventLog, fmt.Sprintf("create download token: %v", tokenErr)))
				} else {
					doneData["downloadToken"] = downloadToken
				}

				workspaceToken, tokenErr := job.GenerateWorkspaceToken(cfg.JWTSecret, jobID, time.Hour)
				if tokenErr != nil {
					logs.Error("Create workspace token (job=%s): %v", jobID, tokenErr)
					_ = h.SendEvent(jobID, NewEvent(EventLog, fmt.Sprintf("create workspace token: %v", tokenErr)))
				} else {
					doneData["workspaceToken"] = workspaceToken
				}
			}

			_ = h.SendEvent(jobID, NewEvent(EventDone, doneData))

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
