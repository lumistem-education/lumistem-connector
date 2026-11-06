package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/job"
	"lumistem/compiler-services/internal/model"
)

func NewDeleteWorkspaceHandler(cfg *config.Config, jm *job.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := r.PathValue("jobID")

		token := r.URL.Query().Get("token")
		verifiedJobID, err := job.VerifyWorkspaceToken(cfg.JWTSecret, token)
		if err != nil || verifiedJobID != jobID {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid token"})
			return
		}

		err = jm.Delete(jobID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: fmt.Sprintf("delete workspace: %v", err)})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(model.SuccessResponse{Message: "workspace deleted"})
	}
}
