package handler

import (
	"encoding/json"
	"net/http"

	"lumistem/compiler-services/internal/arduino"
	"lumistem/compiler-services/internal/config"
	"lumistem/compiler-services/internal/model"
)

type boardListResponse struct {
	DetectedPorts []detectedPort `json:"detected_ports"`
}

type detectedPort struct {
	Port struct {
		Address  string `json:"address"`
		Label    string `json:"label"`
		Protocol string `json:"protocol"`
	} `json:"port"`
	MatchingBoards []struct {
		Name string `json:"name"`
	} `json:"matching_boards"`
}

func NewListPortsHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := arduino.ListPorts(r.Context(), cfg.ArduinoCLIPath, cfg.ArduinoConfigFile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "list ports: " + err.Error()})
			return
		}

		var parsed boardListResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{Error: "invalid port list"})
			return
		}

		ports := make([]model.PortInfo, 0, len(parsed.DetectedPorts))
		for _, p := range parsed.DetectedPorts {
			info := model.PortInfo{
				Address:  p.Port.Address,
				Label:    p.Port.Label,
				Protocol: p.Port.Protocol,
			}
			if len(p.MatchingBoards) > 0 {
				info.Board = p.MatchingBoards[0].Name
			}
			ports = append(ports, info)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ports)
	}
}
