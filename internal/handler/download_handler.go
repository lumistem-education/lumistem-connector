package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/job"
	"lumistem/compiler-services/internal/model"
)

func NewDownloadArtifactHandler(cfg *config.Config, jm *job.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")

		token := r.URL.Query().Get("token")
		verifiedJobID, err := job.VerifyDownloadToken(cfg.JWTSecret, token)
		if err != nil || verifiedJobID != jobID {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid token"})
			return
		}

		jobObj, ok := jm.Get(jobID)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "job not found"})
			return
		}

		fullPath, err := arduino.FindArtifactPath(jobObj.Workspace)
		if err != nil || fullPath == "" {
			writeJSON(w, http.StatusNotFound, model.ErrorResponse{Error: "no compiled artifact"})
			return
		}
		target := filepath.Base(fullPath)

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", target))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, fullPath)
	}
}
