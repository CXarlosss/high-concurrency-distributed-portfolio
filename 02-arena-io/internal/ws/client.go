package ws

import (
	"time"

	"github.com/cv-projects/arena-io/internal/game"
	"github.com/cv-projects/arena-io/internal/netproto"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 5 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 128
	sendBufferSize = 64
)

type Client struct {
	room     *Room
	conn     *websocket.Conn
	playerID uint32
	send     chan []byte
}

func NewClient(room *Room, conn *websocket.Conn, playerID uint32) *Client {
	return &Client{
		room:     room,
		conn:     conn,
		playerID: playerID,
		send:     make(chan []byte, sendBufferSize),
	}
}

func (c *Client) ReadPump(w *game.World) {
	defer func() {
		c.room.unregister <- c
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
			break
		}

		if messageType == websocket.BinaryMessage && len(message) >= 13 {
			input, err := netproto.DecodeClientInput(message)
			if err != nil {
				continue
			}
			w.QueueInput(c.playerID, input)
		}
	}
}

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
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.BinaryMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			// Flush any queued packets in same frame
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
