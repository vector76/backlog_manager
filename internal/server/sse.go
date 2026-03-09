package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// --- JSON response types ---

type dashboardDataResponse struct {
	Projects []projectDashData `json:"projects"`
}

type featureDataResponse struct {
	Status       string                     `json:"status"`
	Iterations   []featureIterationPageData `json:"iterations"`
	BeadProgress *beadProgressJSON          `json:"bead_progress,omitempty"`
}

type beadProgressJSON struct {
	Total       int               `json:"total"`
	Closed      int               `json:"closed"`
	Statuses    map[string]string `json:"statuses"`
	Unavailable bool              `json:"unavailable"`
}

// --- SSE and data handlers ---

func handleDashboardSSE(hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := hub.SubscribeDashboard()
		defer hub.UnsubscribeDashboard(ch)

		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush()
		}
		for {
			select {
			case <-ch:
				_, _ = w.Write([]byte("event: update\ndata: \n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

func handleFeatureSSE(hub *NotifyHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")
		key := projectName + ":" + featureID

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := hub.SubscribeFeature(key)
		defer hub.UnsubscribeFeature(key, ch)

		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush()
		}
		for {
			select {
			case <-ch:
				_, _ = w.Write([]byte("event: update\ndata: \n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

func handleDashboardData(st Store, monitor *BeadMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects := buildDashboardData(st, monitor)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dashboardDataResponse{Projects: projects})
	}
}

func handleFeatureData(st Store, monitor *BeadMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectName := chi.URLParam(r, "name")
		featureID := chi.URLParam(r, "id")

		data, err := buildFeatureDetailData(st, monitor, projectName, featureID)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		resp := featureDataResponse{
			Status:     data.Feature.Status,
			Iterations: data.Iterations,
		}
		if data.BeadProgress != nil {
			p := data.BeadProgress
			resp.BeadProgress = &beadProgressJSON{
				Total:       p.Total,
				Closed:      p.Closed,
				Statuses:    p.Statuses,
				Unavailable: p.Unavailable,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
