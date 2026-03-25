package handlers

import (
	"net/http"

	"cs485/internal/middleware"
	"cs485/internal/services"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles registration, login, and the dev bootstrap endpoint.
type AuthHandler struct {
	authSvc *services.AuthService
	tripSvc *TripService // for bootstrap
}

func NewAuthHandler(authSvc *services.AuthService, tripSvc *TripService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc, tripSvc: tripSvc}
}

// Register godoc  POST /api/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var body struct {
		Email       string `json:"email" binding:"required,email"`
		DisplayName string `json:"displayName" binding:"required"`
		Password    string `json:"password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.authSvc.Register(body.Email, body.DisplayName, body.Password)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}
	token, _, err := h.authSvc.IssueTokenForUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "user": user})
}

// Login godoc  POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, user, err := h.authSvc.Login(body.Email, body.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// Me godoc  GET /api/auth/me  (requires auth)
func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	user, err := h.authSvc.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// DevBootstrap godoc  POST /api/dev/bootstrap
// Returns the demo trip ID and a fresh JWT for the demo owner (Sarah Chen).
// This endpoint exists for P4 development only — remove or guard in production.
func (h *AuthHandler) DevBootstrap(c *gin.Context) {
	result := h.tripSvc.SeedResult()
	if result == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "seed data not available"})
		return
	}
	token, user, err := h.authSvc.IssueTokenForUser(result.OwnerUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tripId": result.TripID,
		"token":  token,
		"userId": result.OwnerUserID,
		"user":   user,
	})
}
