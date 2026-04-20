package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cs485/internal/cache"
	"cs485/internal/config"
	"cs485/internal/db"
	"cs485/internal/handlers"
	"cs485/internal/middleware"
	"cs485/internal/services"
	"cs485/internal/websocket"

	"github.com/gin-gonic/gin"
)

func main() {
	loadDotEnv(".env")
	cfg := config.Load()

	// ── Database (PostgreSQL) ─────────────────────────────────────────────────
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	var seedResult *db.SeedResult
	if cfg.SeedData {
		seedResult, err = db.Seed(database)
		if err != nil {
			log.Fatalf("db seed: %v", err)
		}
		log.Printf("[main] demo trip ID: %s", seedResult.TripID)
	}

	// ── In-memory cache ───────────────────────────────────────────────────────
	cacheStore := cache.New()

	// ── WebSocket hub ─────────────────────────────────────────────────────────
	hub := websocket.New()

	// ── Services ──────────────────────────────────────────────────────────────
	authSvc := services.NewAuthService(database, cfg.JWTSecret)
	emailSvc := services.NewSendGridEmailService(cfg.SendGridAPIKey)

	var googleClient *services.GooglePlacesClient
	if cfg.GoogleAPIKey != "" {
		googleClient = services.NewGooglePlacesClient(cfg.GoogleAPIKey)
		log.Println("[main] Google Places API enabled")
	}
	poiSvc := services.NewPOIService(database, googleClient)
	collabSvc := services.NewCollaboratorService(database, hub)
	invSvc := services.NewInvitationService(database, cacheStore, emailSvc, hub, cfg.FrontendURL)
	itinerarySvc := services.NewItineraryService(database, poiSvc, hub)

	// ── Handlers ──────────────────────────────────────────────────────────────
	tripSvc := handlers.NewTripService(database, collabSvc, hub, cfg.FrontendURL, seedResult)
	tripHandler := handlers.NewTripHandler(tripSvc)
	authHandler := handlers.NewAuthHandler(authSvc, tripSvc)
	invHandler := handlers.NewInvitationHandler(invSvc, collabSvc)
	collabHandler := handlers.NewCollaboratorHandler(collabSvc)
	poiHandler := handlers.NewPOIHandler(poiSvc)
	itineraryHandler := handlers.NewItineraryHandler(itinerarySvc, collabSvc)
	wsHandler := handlers.NewWSHandler(hub, authSvc)

	// ── Router ────────────────────────────────────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(corsMiddleware(cfg.FrontendURL))

	authMW := middleware.Auth(authSvc)

	// Public
	r.POST("/api/auth/register", authHandler.Register)
	r.POST("/api/auth/login", authHandler.Login)
	r.POST("/api/dev/bootstrap", authHandler.DevBootstrap)

	// Share-link preview (public — no auth required)
	r.GET("/api/sharelinks/:inviteCode", tripHandler.PreviewByInviteCode)
	// Share-link join (requires auth)
	r.POST("/api/sharelinks/:inviteCode", authMW, tripHandler.JoinByInviteCode)

	// Spec alias: /api/join/:inviteCode → same handlers as /api/sharelinks/:inviteCode
	r.GET("/api/join/:inviteCode", tripHandler.PreviewByInviteCode)
	r.POST("/api/join/:inviteCode", authMW, tripHandler.JoinByInviteCode)

	// Invitation accept preview (public)
	r.GET("/api/invitations/accept/:token", invHandler.GetInvitationPreview)
	// Invitation accept (requires auth)
	r.POST("/api/invitations/accept/:token", authMW, invHandler.AcceptInvitation)
	// Revoke invitation (requires auth)
	r.DELETE("/api/invitations/:id", authMW, invHandler.RevokeInvitation)

	// POI search (public — anyone can search)
	r.GET("/api/pois/search", poiHandler.Search)

	// Authenticated routes
	api := r.Group("/api", authMW)
	{
		api.GET("/auth/me", authHandler.Me)

		// Trips
		api.GET("/trips", tripHandler.List)
		api.POST("/trips", tripHandler.Create)
		api.GET("/trips/:tripId", tripHandler.Get)
		api.PATCH("/trips/:tripId", tripHandler.Update)
		api.GET("/trips/:tripId/share-link", tripHandler.GetShareLink)
		api.POST("/trips/:tripId/share-link/regenerate", tripHandler.RegenerateShareLink)

		// Invitations
		api.POST("/trips/:tripId/invitations", invHandler.SendInvite)
		api.GET("/trips/:tripId/invitations", invHandler.ListInvitations)

		// Collaborators
		api.GET("/trips/:tripId/collaborators", collabHandler.ListCollaborators)
		api.PATCH("/trips/:tripId/collaborators/:userId", collabHandler.UpdateRole)
		api.DELETE("/trips/:tripId/collaborators/:userId", collabHandler.RemoveCollaborator)

		// Itinerary
		api.GET("/trips/:tripId/itinerary", itineraryHandler.GetItinerary)
		api.POST("/trips/:tripId/itinerary", itineraryHandler.AddPOI)
		api.DELETE("/trips/:tripId/itinerary/:itemId", itineraryHandler.RemoveItem)
	}

	// WebSocket
	r.GET("/ws", wsHandler.HandleConnection)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().Format(time.RFC3339)})
	})

	// Static file serving (production: React build)
	// In dev the Vite dev server handles this; set STATIC_DIR=./dist to enable.
	if cfg.StaticDir != "" {
		r.Static("/assets", cfg.StaticDir+"/assets")
		r.StaticFile("/favicon.ico", cfg.StaticDir+"/favicon.ico")
		// Catch-all: unknown paths → index.html so React Router handles them.
		r.NoRoute(func(c *gin.Context) {
			c.File(cfg.StaticDir + "/index.html")
		})
		log.Printf("[main] serving static files from %s", cfg.StaticDir)
	}

	addr := ":" + cfg.Port
	log.Printf("[main] server listening on %s  (frontend: %s)", addr, cfg.FrontendURL)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// loadDotEnv reads key=value pairs from a .env file into the process environment.
// It is a no-op if the file does not exist.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if os.Getenv(k) == "" { // don't override real env vars
			os.Setenv(k, v)
		}
	}
}

// corsMiddleware sets permissive CORS headers for P4 local development.
func corsMiddleware(frontendURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", frontendURL)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
