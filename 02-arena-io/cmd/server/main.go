package main

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/cv-projects/arena-io/internal/game"
	"github.com/cv-projects/arena-io/internal/ws"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type MetricsResponse struct {
	ActivePlayers      int64   `json:"active_players"`
	TotalTicks         uint64  `json:"total_ticks"`
	LastTickDurationNs int64   `json:"last_tick_duration_ns"`
	NumGoroutines      int     `json:"num_goroutines"`
	AllocMB            float64 `json:"alloc_mb"`
	TotalAllocMB       float64 `json:"total_alloc_mb"`
	SysMB              float64 `json:"sys_mb"`
	NumGC              uint32  `json:"num_gc"`
}

func main() {
	world := game.NewWorld()
	room := ws.NewRoom(world)
	go room.Run()

	engine := game.NewEngine(world, func(serverTick uint32) {
		room.BroadcastSnapshots(serverTick)
	})
	go engine.RunLoop()

	log.Printf("[Arena Engine] Started authoritative 30 TPS engine for 3000x3000 world")

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		playerID := room.NextPlayerID()
		client := ws.NewClient(room, conn, playerID)
		room.Register(client)

		go client.WritePump()
		go client.ReadPump(world)
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		resp := MetricsResponse{
			ActivePlayers:      room.ActivePlayers(),
			TotalTicks:         engine.TotalTicks(),
			LastTickDurationNs: engine.LastTickDurationNs(),
			NumGoroutines:      runtime.NumGoroutine(),
			AllocMB:            float64(m.Alloc) / 1024 / 1024,
			TotalAllocMB:       float64(m.TotalAlloc) / 1024 / 1024,
			SysMB:              float64(m.Sys) / 1024 / 1024,
			NumGC:              m.NumGC,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Favicon no-content handler
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Serve visual game client
	http.Handle("/", http.FileServer(http.Dir("./web")))

	port := ":8081"
	log.Printf("[Server] Arena .io game server listening on %s", port)
	srv := &http.Server{
		Addr:              port,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
