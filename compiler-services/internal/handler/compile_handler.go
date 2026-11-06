package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/job"
	"lumistem/compiler-services/internal/model"
)

func NewCompileHandler(cfg *config.Config, jm *job.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.CreateJobRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024)).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid request"})
			return
		}

		if !arduino.ValidFQBN(req.Board) || req.Code == "" {
			writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "code and a valid FQBN are required"})
			return
		}
		if len(req.Code) > 200*1024 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "code too large"})
			return
		}

		if cfg.Mode.IsLocal() {
			cfg.ArduinoMu.RLock()
			installed, err := arduino.IsCoreInstalled(r.Context(), cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, arduino.CoreFromFQBN(req.Board))
			cfg.ArduinoMu.RUnlock()
			if err != nil || !installed {
				writeJSON(w, http.StatusConflict, model.ErrorResponse{Error: "Install board package " + arduino.CoreFromFQBN(req.Board) + " in Settings before compiling."})
				return
			}
		}
		jobObj, err := jm.CreateJob()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "unable to create job"})
			return
		}

		sketchDir := filepath.Join(jobObj.Workspace, "sketch")
		if err := os.MkdirAll(sketchDir, 0o755); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "unable to create sketch dir"})
			return
		}
		sketchPath := filepath.Join(sketchDir, "sketch.ino")
		if err := os.WriteFile(sketchPath, []byte(req.Code), 0o644); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "unable to write sketch"})
			return
		}

		// write metadata so sse handler knows which fqbn/libs to use
		meta := map[string]interface{}{"board": req.Board, "libs": req.Libs, "browserUpload": req.BrowserUpload}
		metaBytes, _ := json.Marshal(meta)
		_ = os.WriteFile(filepath.Join(jobObj.Workspace, "meta.json"), metaBytes, 0o644)

		// Generate token valid for cfg.CompileTimeout
		ttl := cfg.CompileTimeout
		if ttl <= 0 {
			ttl = 60 * time.Second
		}
		token, err := job.GenerateSSEToken(cfg.JWTSecret, jobObj.ID, ttl)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "unable to generate token"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.CreateJobResponse{
			JobID: jobObj.ID,
			Token: token,
		})
	}
}
