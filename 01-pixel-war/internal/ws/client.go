package ws

import (
	"log"
	"time"

	"github.com/cv-projects/pixel-war/internal/canvas"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer (5 bytes expected, allow up to 64 bytes).
	maxMessageSize = 64

	// Channel buffer size for outgoing messages per client.
	sendBufferSize = 512
)

// Client represents a single connected WebSocket client.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, sendBufferSize),
	}
}

// readPump pumps messages from the websocket connection to the hub and canvas.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		// Only accept binary messages of 5 bytes
		if messageType == websocket.BinaryMessage && len(message) == canvas.UpdatePacketSize {
			update, err := canvas.DecodeBinary(message)
			if err != nil {
				continue
			}

			// Apply to in-memory striped canvas
			_, changed, err := c.hub.canvas.SetPixel(update.X, update.Y, update.Color)
			if err != nil {
				continue
			}

			// If the pixel actually changed, broadcast delta to all other clients
			if changed {
				var pkt [canvas.UpdatePacketSize]byte
				encoded := update.EncodeBinary(pkt[:])
				// Broadcast binary payload
				c.hub.Broadcast(encoded)
			}
		}
	}
}

// writePump pumps messages from the send channel to the websocket connection.
// It batches pending messages into a single TCP write frame to maximize network efficiency.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			// Drain queued messages into the same WebSocket frame (batching)
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
