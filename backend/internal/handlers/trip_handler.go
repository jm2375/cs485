package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"cs485/internal/db"
	"cs485/internal/middleware"
	"cs485/internal/models"
	"cs485/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TripService wraps trip-level DB operations and holds the seed result.
type TripService struct {
	database    *sql.DB
	collabSvc   *services.CollaboratorService
	hub         models.WSHub
	frontendURL string
	seedResult  *db.SeedResult
}

func NewTripService(
	database *sql.DB,
	collabSvc *services.CollaboratorService,
	hub models.WSHub,
	frontendURL string,
	seedResult *db.SeedResult,
) *TripService {
	return &TripService{
		database:    database,
		collabSvc:   collabSvc,
		hub:         hub,
		frontendURL: frontendURL,
		seedResult:  seedResult,
	}
}

// SeedResult exposes the seed metadata for the dev bootstrap endpoint.
func (s *TripService) SeedResult() *db.SeedResult { return s.seedResult }

// TripHandler handles trip CRUD and share-link operations.
type TripHandler struct {
	svc *TripService
}

func NewTripHandler(svc *TripService) *TripHandler { return &TripHandler{svc: svc} }

// Create godoc  POST /api/trips
func (h *TripHandler) Create(c *gin.Context) {
	var body struct {
		Name        string `json:"name" binding:"required"`
		Destination string `json:"destination"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ownerID := middleware.MustGetUserID(c)
	tripID := uuid.New().String()
	inviteCode := uuid.New().String()
	now := fmtTime(time.Now())

	if _, err := h.svc.database.Exec(
		`INSERT INTO trips (id, name, destination, invite_code, owner_id, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		tripID, body.Name, body.Destination, inviteCode, ownerID, now,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create trip failed"})
		return
	}
	if _, err := h.svc.collabSvc.AddCollaborator(tripID, ownerID, models.RoleOwner); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "add owner failed"})
		return
	}
	detail, err := h.buildTripDetail(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, detail)
}

// List godoc  GET /api/trips
func (h *TripHandler) List(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	rows, err := h.svc.database.Query(
		`SELECT trip_id FROM trip_collaborators WHERE user_id = $1 ORDER BY joined_at ASC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list trips failed"})
		return
	}
	defer rows.Close()

	type tripSummary struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Destination string `json:"destination"`
	}

	trips := []tripSummary{}
	for rows.Next() {
		var tripID string
		if err := rows.Scan(&tripID); err != nil {
			continue
		}
		var name, destination string
		if err := h.svc.database.QueryRow(
			`SELECT name, destination FROM trips WHERE id = $1`, tripID,
		).Scan(&name, &destination); err != nil {
			continue
		}
		trips = append(trips, tripSummary{ID: tripID, Name: name, Destination: destination})
	}

	c.JSON(http.StatusOK, gin.H{"trips": trips})
}

// Update godoc  PATCH /api/trips/:tripId
func (h *TripHandler) Update(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)

	ok, _ := h.svc.collabSvc.HasPermission(tripID, userID, "admin")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can rename a trip"})
		return
	}

	var body struct {
		Name        *string `json:"name"`
		Destination *string `json:"destination"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Name != nil {
		if _, err := h.svc.database.Exec(
			`UPDATE trips SET name = $1 WHERE id = $2`, *body.Name, tripID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
			return
		}
	}
	if body.Destination != nil {
		if _, err := h.svc.database.Exec(
			`UPDATE trips SET destination = $1 WHERE id = $2`, *body.Destination, tripID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
			return
		}
	}

	detail, err := h.buildTripDetail(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

// Get godoc  GET /api/trips/:tripId
func (h *TripHandler) Get(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)

	ok, _ := h.svc.collabSvc.IsCollaborator(tripID, userID)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this trip"})
		return
	}
	detail, err := h.buildTripDetail(tripID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trip not found"})
		return
	}
	c.JSON(http.StatusOK, detail)
}

// GetShareLink godoc  GET /api/trips/:tripId/share-link
func (h *TripHandler) GetShareLink(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)
	ok, _ := h.svc.collabSvc.IsCollaborator(tripID, userID)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member"})
		return
	}
	var inviteCode string
	if err := h.svc.database.QueryRow(`SELECT invite_code FROM trips WHERE id = $1`, tripID).Scan(&inviteCode); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trip not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"inviteCode": inviteCode,
		"shareLink":  fmt.Sprintf("%s/invite/%s", h.svc.frontendURL, inviteCode),
	})
}

// RegenerateShareLink godoc  POST /api/trips/:tripId/share-link/regenerate
func (h *TripHandler) RegenerateShareLink(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)
	ok, _ := h.svc.collabSvc.HasPermission(tripID, userID, "admin")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "owner only"})
		return
	}
	newCode := uuid.New().String()
	if _, err := h.svc.database.Exec(`UPDATE trips SET invite_code = $1 WHERE id = $2`, newCode, tripID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"inviteCode": newCode,
		"shareLink":  fmt.Sprintf("%s/invite/%s", h.svc.frontendURL, newCode),
	})
}

// PreviewByInviteCode godoc  GET /api/sharelinks/:inviteCode
func (h *TripHandler) PreviewByInviteCode(c *gin.Context) {
	code := c.Param("inviteCode")
	var tripID, name, destination string
	if err := h.svc.database.QueryRow(
		`SELECT id, name, destination FROM trips WHERE invite_code = $1`, code,
	).Scan(&tripID, &name, &destination); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite link not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tripId": tripID, "name": name, "destination": destination})
}

// JoinByInviteCode godoc  POST /api/sharelinks/:inviteCode  (requires auth)
func (h *TripHandler) JoinByInviteCode(c *gin.Context) {
	code := c.Param("inviteCode")
	userID := middleware.MustGetUserID(c)

	var tripID string
	if err := h.svc.database.QueryRow(
		`SELECT id FROM trips WHERE invite_code = $1`, code,
	).Scan(&tripID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite link not found"})
		return
	}

	already, _ := h.svc.collabSvc.IsCollaborator(tripID, userID)
	if already {
		detail, _ := h.buildTripDetail(tripID)
		c.JSON(http.StatusOK, detail)
		return
	}

	if _, err := h.svc.collabSvc.AddCollaborator(tripID, userID, models.RoleEditor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "join failed"})
		return
	}

	var displayName string
	h.svc.database.QueryRow(`SELECT display_name FROM users WHERE id = $1`, userID).Scan(&displayName)
	h.svc.hub.BroadcastToTrip(tripID, "collaborator_joined", map[string]interface{}{
		"userId":      userID,
		"displayName": displayName,
		"role":        "Editor",
	})

	detail, err := h.buildTripDetail(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

// buildTripDetail assembles the full TripDetail response.
func (h *TripHandler) buildTripDetail(tripID string) (*models.TripDetail, error) {
	var trip models.Trip
	var createdAt string
	if err := h.svc.database.QueryRow(
		`SELECT id, name, destination, invite_code, owner_id, created_at FROM trips WHERE id = $1`, tripID,
	).Scan(&trip.ID, &trip.Name, &trip.Destination, &trip.InviteCode, &trip.OwnerID, &createdAt); err != nil {
		return nil, err
	}
	trip.CreatedAt = parseTime(createdAt)

	collabs, err := h.svc.collabSvc.GetCollaborators(tripID)
	if err != nil {
		return nil, err
	}

	return &models.TripDetail{
		ID:            trip.ID,
		Name:          trip.Name,
		Destination:   trip.Destination,
		ShareLink:     fmt.Sprintf("%s/invite/%s", h.svc.frontendURL, trip.InviteCode),
		Collaborators: collabs,
	}, nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
