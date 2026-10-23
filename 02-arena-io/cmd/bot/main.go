package main

import (
	"flag"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cv-projects/arena-io/internal/netproto"
	"github.com/gorilla/websocket"
)

var (
	serverURL       = flag.String("url", "ws://127.0.0.1:8081/ws", "Arena .io server WS URL")
	numBots         = flag.Int("bots", 500, "Number of concurrent attacking bots")
	durationSec     = flag.Int("duration", 30, "Duration of the chaos attack in seconds")
	attackRateLimit = flag.Bool("flood", true, "Enable input flooding attack")
)

type ChaosMetrics struct {
	ConnectedBots      int64
	InputsSent         uint64
	SnapshotsReceived  uint64
	SpeedHacksAttempted uint64
	SpeedHacksBlocked  uint64
	DroppedPackets     uint64
	ReorderedInputs    uint64
}

func main() {
	flag.Parse()

	log.Printf("[DESTRUCTOR] Launching chaos attack against %s with %d bots for %ds...", *serverURL, *numBots, *durationSec)

	var metrics ChaosMetrics
	var wg sync.WaitGroup
	wg.Add(*numBots)

	stopChan := make(chan struct{})
	timer := time.After(time.Duration(*durationSec) * time.Second)

	for i := 0; i < *numBots; i++ {
		go func(botID int) {
			defer wg.Done()
			runHostileBot(botID, *serverURL, &metrics, stopChan)
		}(i)
		time.Sleep(2 * time.Millisecond) // Staggered aggressive ramp-up
	}

	<-timer
	close(stopChan)
	wg.Wait()

	log.Printf("================== REPORTE DE ATAQUE DEL DESTRUCTOR ==================")
	log.Printf("Bots Conectados en Pico:            %d / %d", atomic.LoadInt64(&metrics.ConnectedBots), *numBots)
	log.Printf("Inputs Enviados:                    %d", atomic.LoadUint64(&metrics.InputsSent))
	log.Printf("Snapshots Recibidos:                %d", atomic.LoadUint64(&metrics.SnapshotsReceived))
	log.Printf("Ataques de Speed-Hack Inyectados:   %d", atomic.LoadUint64(&metrics.SpeedHacksAttempted))
	log.Printf("Ataques Fuera de Orden Inyectados:  %d", atomic.LoadUint64(&metrics.ReorderedInputs))
	log.Printf("=======================================================================")
}

func runHostileBot(id int, url string, m *ChaosMetrics, stop <-chan struct{}) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	atomic.AddInt64(&m.ConnectedBots, 1)
	defer atomic.AddInt64(&m.ConnectedBots, -1)

	// Reader goroutine for snapshots
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if len(msg) >= netproto.HeaderSize {
				atomic.AddUint64(&m.SnapshotsReceived, 1)
			}
		}
	}()

	seq := uint32(1)
	rng := rand.New(rand.NewSource(int64(id) + time.Now().UnixNano()))
	ticker := time.NewTicker(33 * time.Millisecond) // ~30 Hz input rate
	defer ticker.Stop()

	var buf [13]byte

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// Simulación de Jitter artificial (0 a 30ms de retardo)
			if rng.Intn(10) == 0 {
				time.Sleep(time.Duration(rng.Intn(25)) * time.Millisecond)
			}

			// Simulación de Packet Loss artificial (5% de drop)
			if rng.Intn(20) == 0 {
				atomic.AddUint64(&m.DroppedPackets, 1)
				continue
			}

			// Inyección de ataques hostiles:
			attackRoll := rng.Intn(100)

			switch {
			case attackRoll < 15:
				// 1. SPEED HACK: inyección de coordenadas a miles de unidades
				atomic.AddUint64(&m.SpeedHacksAttempted, 1)
				packet := netproto.EncodeClientInput(buf[:], seq, 50000.0, 50000.0)
				_ = conn.WriteMessage(websocket.BinaryMessage, packet)
				atomic.AddUint64(&m.InputsSent, 1)
				seq++

			case attackRoll < 30:
				// 2. INPUT DUPLICATION: enviar el mismo input 5 veces
				for d := 0; d < 5; d++ {
					packet := netproto.EncodeClientInput(buf[:], seq, float32(rng.Intn(3000)), float32(rng.Intn(3000)))
					_ = conn.WriteMessage(websocket.BinaryMessage, packet)
					atomic.AddUint64(&m.InputsSent, 1)
				}
				seq++

			case attackRoll < 45:
				// 3. OUT-OF-ORDER INJECTION: enviar secuencia desfasada
				atomic.AddUint64(&m.ReorderedInputs, 1)
				oldSeq := seq
				if oldSeq > 5 {
					oldSeq -= 5
				}
				packet := netproto.EncodeClientInput(buf[:], oldSeq, 1500.0, 1500.0)
				_ = conn.WriteMessage(websocket.BinaryMessage, packet)
				atomic.AddUint64(&m.InputsSent, 1)

			default:
				// Movimiento normal errático
				targetX := float32(rng.Intn(3000))
				targetY := float32(rng.Intn(3000))
				packet := netproto.EncodeClientInput(buf[:], seq, targetX, targetY)
				_ = conn.WriteMessage(websocket.BinaryMessage, packet)
				atomic.AddUint64(&m.InputsSent, 1)
				seq++
			}
		}
	}
}
