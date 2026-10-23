package ws

import (
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/cv-projects/arena-io/internal/game"
	"github.com/cv-projects/arena-io/internal/netproto"
)

type Room struct {
	world         *game.World
	clients       map[*Client]bool
	register      chan *Client
	unregister    chan *Client
	mu            sync.RWMutex
	nextID        uint32
	activePlayers int64
}

func NewRoom(w *game.World) *Room {
	return &Room{
		world:      w,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
	}
}

func (r *Room) ActivePlayers() int64 {
	return atomic.LoadInt64(&r.activePlayers)
}

func (r *Room) NextPlayerID() uint32 {
	return atomic.AddUint32(&r.nextID, 1)
}

func (r *Room) Register(c *Client) {
	r.register <- c
}

func (r *Room) Unregister(c *Client) {
	r.unregister <- c
}

func (r *Room) Run() {
	for {
		select {
		case client := <-r.register:
			r.mu.Lock()
			r.clients[client] = true
			atomic.AddInt64(&r.activePlayers, 1)
			r.mu.Unlock()

			// Add to physics world at random position
			spawnX := 200.0 + rand.Float64()*(game.WorldWidth-400.0)
			spawnY := 200.0 + rand.Float64()*(game.WorldHeight-400.0)
			r.world.AddPlayer(client.playerID, game.Vec2{X: spawnX, Y: spawnY})

		case client := <-r.unregister:
			r.mu.Lock()
			if _, ok := r.clients[client]; ok {
				delete(r.clients, client)
				close(client.send)
				atomic.AddInt64(&r.activePlayers, -1)
			}
			r.mu.Unlock()

			r.world.RemovePlayer(client.playerID)
		}
	}
}

var neighborBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]uint32, 0, game.MaxEntitiesPerCell*9)
		return &b
	},
}

var entityBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]netproto.EntitySnapshot, 0, 64)
		return &b
	},
}

// BroadcastSnapshots is called at fixed tick rate to send AoI snapshots to all clients
func (r *Room) BroadcastSnapshots(serverTick uint32) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.clients) == 0 {
		return
	}

	ptrNeighbor := neighborBufPool.Get().(*[]uint32)
	ptrEntity := entityBufPool.Get().(*[]netproto.EntitySnapshot)

	neighborBuf := (*ptrNeighbor)[:0]
	entityBuf := (*ptrEntity)[:0]

	for client := range r.clients {
		visible := r.world.GetVisibleEntities(client.playerID, neighborBuf, entityBuf)
		ackSeq := r.world.LastAckSeq(client.playerID)

		bufPtr := netproto.BufferPool.Get().(*[]byte)
		encoded, err := netproto.EncodeSnapshot(*bufPtr, serverTick, ackSeq, visible)
		if err != nil {
			netproto.BufferPool.Put(bufPtr)
			continue
		}

		// Non-blocking write to client channel
		payload := make([]byte, len(encoded))
		copy(payload, encoded)
		netproto.BufferPool.Put(bufPtr)

		select {
		case client.send <- payload:
		default:
			// Client lagging: drop packet to maintain real-time p99
		}
	}

	neighborBufPool.Put(&neighborBuf)
	entityBufPool.Put(&entityBuf)
}
