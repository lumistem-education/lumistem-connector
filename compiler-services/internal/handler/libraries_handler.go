package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/model"
)

func NewListLibrariesHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg.ArduinoMu.RLock()
		defer cfg.ArduinoMu.RUnlock()
		raw, err := arduino.ListLibraries(r.Context(), cfg.ArduinoCLIPath, cfg.ArduinoConfigFile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "list libraries: " + err.Error()})
			return
		}

		var parsed struct {
			InstalledLibraries []struct {
				Library struct {
					Name    string `json:"name"`
					Version string `json:"version"`
					Author  string `json:"author"`
				} `json:"library"`
			} `json:"installed_libraries"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid library list"})
			return
		}

		result := make([]model.LibraryInfo, 0, len(parsed.InstalledLibraries))
		for _, entry := range parsed.InstalledLibraries {
			result = append(result, model.LibraryInfo{
				Name:    entry.Library.Name,
				Version: entry.Library.Version,
				Author:  entry.Library.Author,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func NewInstallLibraryHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.InstallLibraryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !arduino.ValidLibrary(req.Name) || !arduino.ValidVersion(req.Version) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid request"})
			return
		}

		cfg.ArduinoMu.Lock()
		defer cfg.ArduinoMu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), cfg.LibraryOpTimeout)
		defer cancel()

		if _, err := arduino.InstallLibrary(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, req.Name, req.Version, nil); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "install library: " + err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SuccessResponse{Message: "library installed"})
	}
}

func NewUninstallLibraryHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !arduino.ValidLibrary(name) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid request"})
			return
		}

		cfg.ArduinoMu.Lock()
		defer cfg.ArduinoMu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), cfg.LibraryOpTimeout)
		defer cancel()

		if _, err := arduino.UninstallLibrary(ctx, cfg.ArduinoCLIPath, cfg.ArduinoConfigFile, name, nil); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "uninstall library: " + err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SuccessResponse{Message: "library uninstalled"})
	}
}
