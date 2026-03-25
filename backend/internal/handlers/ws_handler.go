package handlers

import (
	"net/http"

	"cs485/internal/services"
	"cs485/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// WSHandler upgrades HTTP connections to WebSocket and delegates to the Hub.
type WSHandler struct {
	hub     *websocket.Hub
	authSvc *services.AuthService
}

func NewWSHandler(hub *websocket.Hub, authSvc *services.AuthService) *WSHandler {
	return &WSHandler{hub: hub, authSvc: authSvc}
}

// HandleConnection godoc  GET /ws?token=<jwt>&tripId=<id>
// The client must supply a valid JWT and the trip they want to join.
// Presence updates are broadcast to all members of that trip automatically.
func (h *WSHandler) HandleConnection(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
		return
	}
	claims, err := h.authSvc.ValidateToken(tokenStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	tripID := c.Query("tripId")
	if tripID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tripId required"})
		return
	}

	socketID := uuid.New().String()
	h.hub.ServeWS(c.Writer, c.Request, claims.UserID, tripID, socketID)
}
