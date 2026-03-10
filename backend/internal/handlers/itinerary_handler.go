package handlers

import (
	"errors"
	"net/http"

	"cs485/internal/middleware"
	"cs485/internal/models"
	"cs485/internal/services"

	"github.com/gin-gonic/gin"
)

// ItineraryHandler manages the shared trip itinerary (add, list, remove).
type ItineraryHandler struct {
	itinerarySvc *services.ItineraryService
	collabSvc    *services.CollaboratorService
}

func NewItineraryHandler(itinerarySvc *services.ItineraryService, collabSvc *services.CollaboratorService) *ItineraryHandler {
	return &ItineraryHandler{itinerarySvc: itinerarySvc, collabSvc: collabSvc}
}

// GetItinerary godoc  GET /api/trips/:tripId/itinerary
func (h *ItineraryHandler) GetItinerary(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)

	ok, _ := h.collabSvc.IsCollaborator(tripID, userID)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this trip"})
		return
	}

	items, err := h.itinerarySvc.GetItinerary(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*models.ItineraryItem{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AddPOI godoc  POST /api/trips/:tripId/itinerary
// Body: { poiId: string, day: number, notes?: string }
func (h *ItineraryHandler) AddPOI(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)

	ok, _ := h.collabSvc.HasPermission(tripID, userID, "write")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "editor or owner role required"})
		return
	}

	var body struct {
		POIID string `json:"poiId" binding:"required"`
		Day   int    `json:"day" binding:"required,min=1"`
		Notes string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.itinerarySvc.AddPOI(tripID, body.POIID, userID, body.Day, body.Notes)
	if err != nil {
		if errors.Is(err, services.ErrAlreadyOnItinerary) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// RemoveItem godoc  DELETE /api/trips/:tripId/itinerary/:itemId
func (h *ItineraryHandler) RemoveItem(c *gin.Context) {
	tripID := c.Param("tripId")
	itemID := c.Param("itemId")
	userID := middleware.MustGetUserID(c)

	ok, _ := h.collabSvc.HasPermission(tripID, userID, "write")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "editor or owner role required"})
		return
	}

	if err := h.itinerarySvc.RemoveItem(tripID, itemID); err != nil {
		if errors.Is(err, services.ErrItemNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
