package handlers

import (
	"encoding/json"
	"net/http"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// HealthHandler handles health check requests
type HealthHandler struct {
	version string
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{version: version}
}

// Health handles GET /health
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{
		Status:  "ok",
		Version: h.version,
	})
}
