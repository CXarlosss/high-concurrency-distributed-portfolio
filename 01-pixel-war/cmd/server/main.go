package main

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/cv-projects/pixel-war/internal/canvas"
	"github.com/cv-projects/pixel-war/internal/ws"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev/load testing
	},
}

type MetricsResponse struct {
	ActiveClients   int64   `json:"active_clients"`
	TotalBroadcasts uint64  `json:"total_broadcasts"`
	NumGoroutines   int     `json:"num_goroutines"`
	AllocMB         float64 `json:"alloc_mb"`
	TotalAllocMB    float64 `json:"total_alloc_mb"`
	SysMB           float64 `json:"sys_mb"`
	NumGC           uint32  `json:"num_gc"`
}

func main() {
	// Initialize sharded in-memory canvas (1000x1000)
	c := canvas.New(canvas.DefaultWidth, canvas.DefaultHeight)
	log.Printf("[Canvas] Initialized %dx%d canvas (%d bytes in RAM)", c.Width(), c.Height(), c.TotalPixels())

	// Initialize WebSocket broadcast hub
	hub := ws.NewHub(c)
	go hub.Run()

	// WebSocket handler
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WS upgrade failed: %v", err)
			return
		}

		client := ws.NewClient(hub, conn)
		// Register client with hub
		hub.Register(client)

		// Start read and write pumps
		go client.WritePump()
		go client.ReadPump()
	})

	// Full canvas snapshot endpoint
	http.HandleFunc("/canvas", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-cache")
		snapshot := c.Snapshot()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(snapshot)
	})

	// Metrics endpoint for real-time telemetry
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		resp := MetricsResponse{
			ActiveClients:   hub.ActiveClients(),
			TotalBroadcasts: hub.TotalBroadcasts(),
			NumGoroutines:   runtime.NumGoroutine(),
			AllocMB:         float64(m.Alloc) / 1024 / 1024,
			TotalAllocMB:    float64(m.TotalAlloc) / 1024 / 1024,
			SysMB:           float64(m.Sys) / 1024 / 1024,
			NumGC:           m.NumGC,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Favicon no-content handler
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Serve thin client frontend
	http.Handle("/", http.FileServer(http.Dir("./web")))

	port := ":8080"
	log.Printf("[Server] High-concurrency Pixel War server listening on %s", port)
	srv := &http.Server{
		Addr:              port,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
