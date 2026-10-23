package ws

import (
	"sync/atomic"

	"github.com/cv-projects/pixel-war/internal/canvas"
)

// Hub maintains the set of active clients and broadcasts delta updates.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound delta updates to broadcast.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// In-memory striped canvas engine.
	canvas *canvas.MemoryCanvas

	// Atomic counters for telemetry.
	activeClients   int64
	totalBroadcasts uint64
}

func NewHub(c *canvas.MemoryCanvas) *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 4096),
		register:   make(chan *Client, 1024),
		unregister: make(chan *Client, 1024),
		clients:    make(map[*Client]bool),
		canvas:     c,
	}
}

// Register queues a client for registration.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister queues a client for unregistration.
func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

// ActiveClients returns current active connected clients.
func (h *Hub) ActiveClients() int64 {
	return atomic.LoadInt64(&h.activeClients)
}

// TotalBroadcasts returns total deltas broadcasted.
func (h *Hub) TotalBroadcasts() uint64 {
	return atomic.LoadUint64(&h.totalBroadcasts)
}

// Broadcast queues a message for broadcast.
func (h *Hub) Broadcast(msg []byte) {
	// Make an immutable copy for safety across goroutines
	payload := make([]byte, len(msg))
	copy(payload, msg)

	select {
	case h.broadcast <- payload:
	default:
		// Queue full under extreme load; drop to prevent system freeze
	}
}

// Run executes the main Hub event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			atomic.AddInt64(&h.activeClients, 1)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				atomic.AddInt64(&h.activeClients, -1)
			}

		case message := <-h.broadcast:
			atomic.AddUint64(&h.totalBroadcasts, 1)
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Slow client detected! Drop connection to protect the cluster
					close(client.send)
					delete(h.clients, client)
					atomic.AddInt64(&h.activeClients, -1)
				}
			}
		}
	}
}
