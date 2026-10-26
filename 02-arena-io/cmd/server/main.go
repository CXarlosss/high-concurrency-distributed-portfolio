package main

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/cv-projects/arena-io/internal/game"
	"github.com/cv-projects/arena-io/internal/netproto"
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

		// 1. Send initial authoritative player ID welcome packet (5 bytes: [0x00, ID:uint32 LittleEndian])
		welcome := make([]byte, 5)
		welcome[0] = netproto.MsgWelcome
		binary.LittleEndian.PutUint32(welcome[1:5], playerID)
		_ = conn.WriteMessage(websocket.BinaryMessage, welcome)

		// 2. Send initial Food Pellets snapshot (MsgFoodInit: 0x03)
		foodPayload := netproto.EncodeFoodInit(nil, world.GetFoodPelletsNet())
		_ = conn.WriteMessage(websocket.BinaryMessage, foodPayload)

		client := ws.NewClient(room, conn, playerID)
		room.Register(client)

		go client.WritePump()
		go client.ReadPump(world)
	})

	http.HandleFunc("/bots/toggle", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		count := world.ToggleBots()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active_bots": count,
		})
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Favicon no-content handler
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Serve visual game client (support root or subfolder execution)
	webDir := "./web"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		webDir = "./02-arena-io/web"
	}
	http.Handle("/", http.FileServer(http.Dir(webDir)))

	log.Printf("[Arena Web] Listening on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
