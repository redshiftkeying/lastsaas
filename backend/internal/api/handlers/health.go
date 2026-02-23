package handlers

import (
	"net/http"
	"time"

	"lastsaas/internal/health"
	"lastsaas/internal/models"
)

type HealthHandler struct {
	service *health.Service
}

func NewHealthHandler(service *health.Service) *HealthHandler {
	return &HealthHandler{service: service}
}

// ListNodes handles GET /api/admin/health/nodes
func (h *HealthHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.service.ListNodes(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list nodes")
		return
	}
	if nodes == nil {
		nodes = []models.SystemNode{}
	}
	respondWithJSON(w, http.StatusOK, map[string]interface{}{"nodes": nodes})
}

// GetMetrics handles GET /api/admin/health/metrics?node=<id>&range=<1h|6h|24h|7d|30d>
func (h *HealthHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	nodeID := q.Get("node")
	rangeStr := q.Get("range")

	to := time.Now()
	from := parseTimeRange(rangeStr, to)

	var metrics []models.SystemMetric
	var err error

	if nodeID != "" {
		metrics, err = h.service.GetMetrics(r.Context(), nodeID, from, to)
	} else {
		metrics, err = h.service.GetAggregateMetrics(r.Context(), from, to)
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to query metrics")
		return
	}
	if metrics == nil {
		metrics = []models.SystemMetric{}
	}
	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"from":    from,
		"to":      to,
	})
}

// GetCurrent handles GET /api/admin/health/current
func (h *HealthHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.service.GetCurrentMetrics(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get current metrics")
		return
	}
	if metrics == nil {
		metrics = []models.SystemMetric{}
	}
	respondWithJSON(w, http.StatusOK, map[string]interface{}{"metrics": metrics})
}

// GetIntegrations handles GET /api/admin/health/integrations
func (h *HealthHandler) GetIntegrations(w http.ResponseWriter, r *http.Request) {
	results := h.service.GetIntegrationStatus()
	if results == nil {
		results = []models.IntegrationCheck{}
	}

	// Enrich with 24h call counts
	stripeCalls, resendEmails := h.service.GetIntegrationCounts24h(r.Context())
	for i := range results {
		switch results[i].Name {
		case "stripe":
			results[i].Calls24h = stripeCalls
		case "resend":
			results[i].Calls24h = resendEmails
		}
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{"integrations": results})
}

func parseTimeRange(rangeStr string, to time.Time) time.Time {
	switch rangeStr {
	case "1h":
		return to.Add(-1 * time.Hour)
	case "6h":
		return to.Add(-6 * time.Hour)
	case "24h":
		return to.Add(-24 * time.Hour)
	case "7d":
		return to.Add(-7 * 24 * time.Hour)
	case "30d":
		return to.Add(-30 * 24 * time.Hour)
	default:
		return to.Add(-24 * time.Hour)
	}
}
