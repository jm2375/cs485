package handlers

import (
	"errors"
	"net/http"

	"cs485/internal/middleware"
	"cs485/internal/models"
	"cs485/internal/services"

	"github.com/gin-gonic/gin"
)

// CollaboratorHandler handles listing, role-update, and removal of trip collaborators.
type CollaboratorHandler struct {
	collabSvc *services.CollaboratorService
}

func NewCollaboratorHandler(collabSvc *services.CollaboratorService) *CollaboratorHandler {
	return &CollaboratorHandler{collabSvc: collabSvc}
}

// ListCollaborators godoc  GET /api/trips/:tripId/collaborators
func (h *CollaboratorHandler) ListCollaborators(c *gin.Context) {
	tripID := c.Param("tripId")
	userID := middleware.MustGetUserID(c)

	ok, _ := h.collabSvc.IsCollaborator(tripID, userID)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this trip"})
		return
	}

	collabs, err := h.collabSvc.GetCollaborators(tripID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if collabs == nil {
		collabs = []models.CollaboratorView{}
	}
	c.JSON(http.StatusOK, gin.H{"collaborators": collabs})
}

// UpdateRole godoc  PATCH /api/trips/:tripId/collaborators/:userId
// Body: { role: "Editor"|"Viewer" }
func (h *CollaboratorHandler) UpdateRole(c *gin.Context) {
	tripID := c.Param("tripId")
	targetUserID := c.Param("userId")
	requesterID := middleware.MustGetUserID(c)

	var body struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, ok := models.ParseRole(body.Role)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	if role == models.RoleOwner {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot assign owner role via this endpoint"})
		return
	}

	collab, err := h.collabSvc.UpdateRole(tripID, targetUserID, role, requesterID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, collab)
}

// RemoveCollaborator godoc  DELETE /api/trips/:tripId/collaborators/:userId
func (h *CollaboratorHandler) RemoveCollaborator(c *gin.Context) {
	tripID := c.Param("tripId")
	targetUserID := c.Param("userId")
	requesterID := middleware.MustGetUserID(c)

	if err := h.collabSvc.RemoveCollaborator(tripID, targetUserID, requesterID); err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrCannotRemoveOwner):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrCollaboratorNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
