package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/gorilla/websocket"

    "backend/docker"
)

type StatsHandler struct {
    dclient *docker.Client
}

func NewStatsHandler(d *docker.Client) *StatsHandler {
    return &StatsHandler{dclient: d}
}

var upgrader = websocket.Upgrader{}

// WS streams metrics for all containers every 2 seconds.
func (s *StatsHandler) WS(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        http.Error(w, "failed to upgrade websocket", http.StatusBadRequest)
        return
    }
    defer conn.Close()

    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
            stats, err := s.dclient.GetAllContainerStats(ctx)
            cancel()
            if err != nil {
                // send an error message and continue
                _ = conn.WriteJSON(map[string]string{"error": err.Error()})
                continue
            }
            conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
            if err := conn.WriteJSON(stats); err != nil {
                return
            }
        default:
            // check for close message
            _, _, err := conn.ReadMessage()
            if err != nil {
                return
            }
        }
    }
}

// One-shot REST endpoint for a single container
func (s *StatsHandler) OneShot(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    // allow up to 10s for an on-demand stats inspect
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
