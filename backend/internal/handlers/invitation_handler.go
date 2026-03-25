package handlers

import (
	"errors"
	"net/http"

	"cs485/internal/middleware"
	"cs485/internal/models"
	"cs485/internal/services"

	"github.com/gin-gonic/gin"
)

// InvitationHandler handles email invite creation, listing, preview, acceptance, and revocation.
type InvitationHandler struct {
	invSvc    *services.InvitationService
	collabSvc *services.CollaboratorService
}

func NewInvitationHandler(invSvc *services.InvitationService, collabSvc *services.CollaboratorService) *InvitationHandler {
	return &InvitationHandler{invSvc: invSvc, collabSvc: collabSvc}
}

// SendInvite godoc  POST /api/trips/:tripId/invitations
// Body: { emails: string[], role: "Editor"|"Viewer" }
func (h *InvitationHandler) SendInvite(c *gin.Context) {
	tripID := c.Param("tripId")
	requesterID := middleware.MustGetUserID(c)

	ok, _ := h.collabSvc.HasPermission(tripID, requesterID, "invite")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "editor or owner role required"})
		return
	}

	var body struct {
		Emails []string `json:"emails" binding:"required,min=1"`
		Role   string   `json:"role"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, ok := models.ParseRole(body.Role)
	if !ok {
		role = models.RoleEditor
	}

	var sent []*models.Invitation
	var errs []string
	for _, email := range body.Emails {
		inv, err := h.invSvc.SendEmailInvite(tripID, requesterID, email, role)
		if err != nil {
			errs = append(errs, email+": "+err.Error())
			continue
		}
		sent = append(sent, inv)
	}

	if len(sent) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": errs})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"invitations": sent, "errors": errs})
}

// ListInvitations godoc  GET /api/trips/:tripId/invitations?status=PENDING
func (h *InvitationHandler) ListInvitations(c *gin.Context) {
	tripID := c.Param("tripId")
	requesterID := middleware.MustGetUserID(c)

	ok, _ := h.collabSvc.HasPermission(tripID, requesterID, "invite")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	var statusPtr *models.InvitationStatus
	if s := c.Query("status"); s != "" {
		st := models.InvitationStatus(s)
		statusPtr = &st
	}

	invs, err := h.invSvc.ListInvitations(tripID, statusPtr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if invs == nil {
		invs = []*models.Invitation{}
	}
	c.JSON(http.StatusOK, gin.H{"invitations": invs})
}

// GetInvitationPreview godoc  GET /api/invitations/accept/:token  (public)
// Returns trip name and role so the user can see what they are accepting.
func (h *InvitationHandler) GetInvitationPreview(c *gin.Context) {
	token := c.Param("token")
	inv, err := h.invSvc.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found or expired"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"invitationId": inv.ID,
		"tripId":       inv.TripID,
		"role":         models.FormatRole(inv.Role),
		"expiresAt":    inv.ExpiresAt,
	})
}

// AcceptInvitation godoc  POST /api/invitations/accept/:token  (requires auth)
func (h *InvitationHandler) AcceptInvitation(c *gin.Context) {
	token := c.Param("token")
	userID := middleware.MustGetUserID(c)

	collab, err := h.invSvc.AcceptInvitation(token, userID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInviteNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrInviteExpiredOrUsed):
			c.JSON(http.StatusGone, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"collaborator": collab})
}

// RevokeInvitation godoc  DELETE /api/invitations/:id
func (h *InvitationHandler) RevokeInvitation(c *gin.Context) {
	invID := c.Param("id")
	requesterID := middleware.MustGetUserID(c)

	if err := h.invSvc.RevokeInvitation(invID, requesterID); err != nil {
		switch {
		case errors.Is(err, services.ErrInviteNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrNotOwner):
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
