package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"backend/docker"
)

type StatsHandler struct {
	dclient *docker.Client
	buffer  *StatsBuffer
}

func NewStatsHandler(d *docker.Client) *StatsHandler {
	h := &StatsHandler{
		dclient: d,
		buffer:  NewStatsBuffer(),
	}
	go h.collect()
	return h
}

// collect runs a background loop that fetches stats for all containers every
// 2 seconds and stores them in the ring buffer. This ensures history
// accumulates from server start, independent of WebSocket clients.
func (s *StatsHandler) collect() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		metrics, err := s.dclient.GetAllContainerStats(ctx)
		cancel()
		if err != nil {
			log.Printf("stats collector: %v", err)
			continue
		}
		s.buffer.PushAll(metrics)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WS streams metrics for all containers every 2 seconds.
func (s *StatsHandler) WS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "failed to upgrade websocket", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	// Read pump: detect client disconnect
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			stats, err := s.dclient.GetAllContainerStats(ctx)
			cancel()
			if err != nil {
				_ = conn.WriteJSON(map[string]string{"error": err.Error()})
				continue
			}
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(stats); err != nil {
				return
			}
		}
	}
}

// OneShot is a REST endpoint for a single container's current stats.
func (s *StatsHandler) OneShot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stats, err := s.dclient.ContainerStats(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	m, err := docker.ParseStats(stats.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	m.ContainerID = id
	_ = json.NewEncoder(w).Encode(m)
}

// History returns the buffered stats history for all containers.
func (s *StatsHandler) History(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.buffer.All())
}
