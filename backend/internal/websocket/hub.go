// Package websocket implements the real-time collaboration layer.
// The Hub tracks one *Client per browser connection, organised by tripID room.
// It satisfies the models.WSHub interface so services can broadcast events
// without importing this package.
package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
	CheckOrigin:      func(r *http.Request) bool { return true }, // CORS handled by Gin
}

// WSMessage is the JSON envelope for all events sent over the wire.
type WSMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// Client represents one connected browser tab.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	UserID   string
	TripID   string
	SocketID string
}

// Hub manages all active WebSocket clients grouped by trip room.
type Hub struct {
	rooms map[string]map[*Client]bool
	mu    sync.RWMutex
}

// New creates an initialised Hub.
func New() *Hub {
	return &Hub{rooms: make(map[string]map[*Client]bool)}
}

// ---- models.WSHub interface ------------------------------------------------

// BroadcastToTrip serialises event+data and sends it to every client in tripID.
func (h *Hub) BroadcastToTrip(tripID, event string, data interface{}) {
	payload, err := json.Marshal(WSMessage{Event: event, Data: data})
	if err != nil {
		log.Printf("[ws] marshal error: %v", err)
		return
	}
	h.mu.RLock()
	clients := h.rooms[tripID]
	h.mu.RUnlock()
	for c := range clients {
		select {
		case c.send <- payload:
		default:
			// Slow client — drop the message rather than blocking.
			log.Printf("[ws] dropped message to client %s (buffer full)", c.SocketID)
		}
	}
}

// GetOnlineUsersInTrip returns the distinct userIDs of connected clients in tripID.
func (h *Hub) GetOnlineUsersInTrip(tripID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[string]bool)
	for c := range h.rooms[tripID] {
		seen[c.UserID] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids
}

// ---- Internal registration -------------------------------------------------

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	if h.rooms[c.TripID] == nil {
		h.rooms[c.TripID] = make(map[*Client]bool)
	}
	h.rooms[c.TripID][c] = true
	h.mu.Unlock()
	h.broadcastPresence(c.TripID)
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	if clients, ok := h.rooms[c.TripID]; ok {
		delete(clients, c)
		if len(clients) == 0 {
			delete(h.rooms, c.TripID)
		}
	}
	h.mu.Unlock()
	close(c.send)
	h.broadcastPresence(c.TripID)
}

func (h *Hub) broadcastPresence(tripID string) {
	ids := h.GetOnlineUsersInTrip(tripID)
	h.BroadcastToTrip(tripID, "presence_update", map[string]interface{}{
		"onlineUserIds": ids,
	})
}

// ---- Pump goroutines -------------------------------------------------------

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 512
)

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[ws] read error: %v", err)
			}
			break
		}
		var envelope WSMessage
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}
		switch envelope.Event {
		case "join_trip":
			// No-op: tripID is set at connection time via query param.
		case "leave_trip":
			// The disconnect will handle cleanup; nothing extra needed.
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)
			// Drain any queued messages into the same frame.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWS upgrades an HTTP connection to WebSocket and starts the pump goroutines.
// userID and tripID must already be resolved by the calling handler.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID, tripID, socketID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}
	c := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 32),
		UserID:   userID,
		TripID:   tripID,
		SocketID: socketID,
	}
	h.register(c)
	go c.writePump()
	go c.readPump()
}
