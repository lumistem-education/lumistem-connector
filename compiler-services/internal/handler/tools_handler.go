package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func apiError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func NewHealthHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		cfg.ArduinoMu.RLock()
		defer cfg.ArduinoMu.RUnlock()
		version, err := arduino.Command(ctx, cfg.ArduinoCLIPath, "", "version", "--json")
		initialized := true
		for _, index := range []string{"package_index.json", "library_index.json"} {
			if _, err := os.Stat(filepath.Join(cfg.ArduinoDataDir, index)); err != nil {
				initialized = false
			}
		}
		core := arduino.CoreFromFQBN(r.URL.Query().Get("fqbn"))
		installed := false
		if err == nil && initialized && core != "" {
			installed, _ = arduino.IsCoreInstalled(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, core)
		}
		writeJSON(w, 200, map[string]any{"service": "lumistem-compiler", "apiVersion": 1, "mode": cfg.Mode, "port": config.LocalPort, "cliInstalled": err == nil, "cliVersion": strings.TrimSpace(string(version)), "initialized": initialized, "requiredCore": core, "coreInstalled": installed, "ready": err == nil && initialized && (core == "" || installed)})
	}
}
func RegisterToolsRoutes(mux *http.ServeMux, cfg *config.Config) {
	mux.HandleFunc("POST /api/setup", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.CoreInstallTimeout)
		defer cancel()
		cfg.ArduinoMu.Lock()
		defer cfg.ArduinoMu.Unlock()
		if err := arduino.InstallCLI(ctx, cfg.ArduinoCLIPath); err != nil {
			apiError(w, 500, err)
			return
		}
		if _, err := os.Stat(cfg.ArduinoConfigFile); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(cfg.ArduinoConfigFile), 0755); err != nil {
				apiError(w, 500, err)
				return
			}
			contents, _ := json.Marshal(map[string]any{"directories": map[string]string{"data": cfg.ArduinoDataDir, "downloads": filepath.Join(cfg.ArduinoDataDir, "staging"), "user": filepath.Join(cfg.ArduinoDataDir, "user")}, "board_manager": map[string]any{"additional_urls": []string{"https://espressif.github.io/arduino-esp32/package_esp32_index.json"}}})
			if err := os.WriteFile(cfg.ArduinoConfigFile, contents, 0600); err != nil {
				apiError(w, 500, err)
				return
			}
		}
		for _, args := range [][]string{{"core", "update-index"}, {"lib", "update-index"}} {
			if _, err := arduino.Command(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, args...); err != nil {
				apiError(w, 500, err)
				return
			}
		}
		writeJSON(w, 200, map[string]string{"message": "Arduino tools ready."})
	})
	mux.HandleFunc("GET /api/cores", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		cfg.ArduinoMu.RLock()
		defer cfg.ArduinoMu.RUnlock()
		raw, err := arduino.ListCores(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile)
		if err != nil {
			apiError(w, 500, err)
			return
		}
		// CLI releases have used both a top-level array and a platforms envelope.
		var cores []map[string]any
		if err := json.Unmarshal(raw, &cores); err != nil {
			var wrapped struct {
				Platforms []map[string]any `json:"platforms"`
			}
			if err := json.Unmarshal(raw, &wrapped); err != nil {
				apiError(w, 500, err)
				return
			}
			cores = wrapped.Platforms
		}
		if cores == nil {
			cores = []map[string]any{}
		}
		writeJSON(w, 200, cores)
	})
	mux.HandleFunc("POST /api/cores/install", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || !arduino.ValidCore(req.ID) || !arduino.ValidVersion(req.Version) {
			apiError(w, 400, fmt.Errorf("invalid core ID or version"))
			return
		}
		spec := req.ID
		if req.Version != "" {
			spec += "@" + req.Version
		}
		toolsMutation(w, r, cfg, "core", "install", spec)
	})
	mux.HandleFunc("DELETE /api/cores/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !arduino.ValidCore(id) {
			apiError(w, 400, fmt.Errorf("invalid core ID"))
			return
		}
		toolsMutation(w, r, cfg, "core", "uninstall", id)
	})
	mux.HandleFunc("GET /api/boards", func(w http.ResponseWriter, r *http.Request) {
		toolsJSON(w, r, cfg, "board", "listall", "--json")
	})
	mux.HandleFunc("GET /api/boards/details", func(w http.ResponseWriter, r *http.Request) {
		fqbn := r.URL.Query().Get("fqbn")
		if !arduino.ValidFQBN(fqbn) {
			apiError(w, 400, fmt.Errorf("invalid FQBN"))
			return
		}
		toolsJSON(w, r, cfg, "board", "details", "--fqbn", fqbn, "--json")
	})
	mux.HandleFunc("POST /api/board-indexes", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
			apiError(w, 400, fmt.Errorf("invalid index URL"))
			return
		}
		u, err := url.Parse(req.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			apiError(w, 400, fmt.Errorf("an HTTPS package index URL is required"))
			return
		}
		toolsMutation(w, r, cfg, "config", "add", "board_manager.additional_urls", req.URL)
	})
	mux.HandleFunc("POST /api/indexes/update", func(w http.ResponseWriter, r *http.Request) {
		toolsMutation(w, r, cfg, "core", "update-index")
	})
	mux.HandleFunc("GET /api/cores/search", func(w http.ResponseWriter, r *http.Request) {
		toolsJSON(w, r, cfg, "core", "search", "--all", "--json", "--", r.URL.Query().Get("q"))
	})
	mux.HandleFunc("GET /api/libs/search", func(w http.ResponseWriter, r *http.Request) {
		toolsJSON(w, r, cfg, "lib", "search", "--json", "--", r.URL.Query().Get("q"))
	})
}
func toolsMutation(w http.ResponseWriter, r *http.Request, cfg *config.Config, args ...string) {
	ctx, cancel := context.WithTimeout(r.Context(), cfg.CoreInstallTimeout)
	defer cancel()
	cfg.ArduinoMu.Lock()
	defer cfg.ArduinoMu.Unlock()
	raw, err := arduino.Command(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, args...)
	if err != nil {
		apiError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": strings.TrimSpace(string(raw))})
}
func toolsJSON(w http.ResponseWriter, r *http.Request, cfg *config.Config, args ...string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cfg.ArduinoMu.RLock()
	defer cfg.ArduinoMu.RUnlock()
	raw, err := arduino.Command(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, args...)
	if err != nil {
		apiError(w, 500, err)
		return
	}
	if !json.Valid(raw) {
		apiError(w, 500, fmt.Errorf("invalid Arduino CLI response"))
		return
	}
	writeJSON(w, 200, json.RawMessage(raw))
}
