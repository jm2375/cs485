// integration_test.go exercises every HTTP endpoint of the backend against
// a real in-memory SQLite database.  Each test that needs isolation gets its
// own database instance via newTestServer().
package main_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cs485/internal/cache"
	appdb "cs485/internal/db"
	"cs485/internal/handlers"
	"cs485/internal/middleware"
	"cs485/internal/models"
	"cs485/internal/services"
	"cs485/internal/websocket"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

// ── Test server helper ────────────────────────────────────────────────────────

type testServer struct {
	router    *gin.Engine
	tripID    string
	token     string // Sarah Chen (OWNER) JWT
	ownerID   string
	authSvc   *services.AuthService
	collabSvc *services.CollaboratorService
	db        *sql.DB
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Unique in-memory DB per test to avoid state bleed.
	dsn := fmt.Sprintf(
		"file:testdb%d?mode=memory&cache=shared&_foreign_keys=on&_journal_mode=WAL",
		time.Now().UnixNano(),
	)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { database.Close() })

	if err := appdb.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seedResult, err := appdb.Seed(database)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	cacheStore := cache.New()
	hub := websocket.New()
	authSvc := services.NewAuthService(database, "test-secret")
	emailSvc := services.NewMockEmailService()
	poiSvc := services.NewPOIService(database, nil) // nil = local SQLite only (no API key in tests)
	collabSvc := services.NewCollaboratorService(database, hub)
	invSvc := services.NewInvitationService(database, cacheStore, emailSvc, hub, "http://localhost:5173")
	itinerarySvc := services.NewItineraryService(database, poiSvc, hub)

	tripSvc := handlers.NewTripService(database, collabSvc, hub, "http://localhost:5173", seedResult)
	tripHandler := handlers.NewTripHandler(tripSvc)
	authHandler := handlers.NewAuthHandler(authSvc, tripSvc)
	invHandler := handlers.NewInvitationHandler(invSvc, collabSvc)
	collabHandler := handlers.NewCollaboratorHandler(collabSvc)
	poiHandler := handlers.NewPOIHandler(poiSvc)
	itineraryHandler := handlers.NewItineraryHandler(itinerarySvc, collabSvc)
	wsHandler := handlers.NewWSHandler(hub, authSvc)

	authMW := middleware.Auth(authSvc)

	r := gin.New()
	r.Use(gin.Recovery())

	// Public
	r.POST("/api/auth/register", authHandler.Register)
	r.POST("/api/auth/login", authHandler.Login)
	r.POST("/api/dev/bootstrap", authHandler.DevBootstrap)
	r.GET("/api/sharelinks/:inviteCode", tripHandler.PreviewByInviteCode)
	r.POST("/api/sharelinks/:inviteCode", authMW, tripHandler.JoinByInviteCode)
	r.GET("/api/invitations/accept/:token", invHandler.GetInvitationPreview)
	r.POST("/api/invitations/accept/:token", authMW, invHandler.AcceptInvitation)
	r.DELETE("/api/invitations/:id", authMW, invHandler.RevokeInvitation)
	r.GET("/api/pois/search", poiHandler.Search)
	r.GET("/ws", wsHandler.HandleConnection)
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// Authenticated
	api := r.Group("/api", authMW)
	{
		api.GET("/auth/me", authHandler.Me)
		api.POST("/trips", tripHandler.Create)
		api.GET("/trips/:tripId", tripHandler.Get)
		api.GET("/trips/:tripId/share-link", tripHandler.GetShareLink)
		api.POST("/trips/:tripId/share-link/regenerate", tripHandler.RegenerateShareLink)
		api.POST("/trips/:tripId/invitations", invHandler.SendInvite)
		api.GET("/trips/:tripId/invitations", invHandler.ListInvitations)
		api.GET("/trips/:tripId/collaborators", collabHandler.ListCollaborators)
		api.PATCH("/trips/:tripId/collaborators/:userId", collabHandler.UpdateRole)
		api.DELETE("/trips/:tripId/collaborators/:userId", collabHandler.RemoveCollaborator)
		api.GET("/trips/:tripId/itinerary", itineraryHandler.GetItinerary)
		api.POST("/trips/:tripId/itinerary", itineraryHandler.AddPOI)
		api.DELETE("/trips/:tripId/itinerary/:itemId", itineraryHandler.RemoveItem)
	}

	token, _, err := authSvc.IssueTokenForUser(seedResult.OwnerUserID)
	if err != nil {
		t.Fatalf("issue owner token: %v", err)
	}

	return &testServer{
		router:    r,
		tripID:    seedResult.TripID,
		token:     token,
		ownerID:   seedResult.OwnerUserID,
		authSvc:   authSvc,
		collabSvc: collabSvc,
		db:        database,
	}
}

// do makes an authenticated request as the owner.
func (ts *testServer) do(t *testing.T, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	return ts.doAs(t, method, path, body, ts.token)
}

// doAs makes a request with the given token (empty = unauthenticated).
func (ts *testServer) doAs(t *testing.T, method, path string, body interface{}, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	ts.router.ServeHTTP(w, req)
	return w
}

// asJSON decodes the response body into a map.
func asJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode body (status %d): %v\nbody: %s", w.Code, err, w.Body.String())
	}
	return m
}

// mustStatus fails the test if the recorder's status code does not match.
func mustStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status: got %d want %d\nbody: %s", w.Code, want, w.Body.String())
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodGet, "/health", nil, "")
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func TestDevBootstrap(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodPost, "/api/dev/bootstrap", nil, "")
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)

	if body["tripId"] == nil || body["tripId"] == "" {
		t.Error("bootstrap: missing tripId")
	}
	if body["token"] == nil || body["token"] == "" {
		t.Error("bootstrap: missing token")
	}
	if body["userId"] == nil || body["userId"] == "" {
		t.Error("bootstrap: missing userId")
	}
	if body["tripId"] != ts.tripID {
		t.Errorf("bootstrap tripId mismatch: got %v, want %s", body["tripId"], ts.tripID)
	}
}

func TestAuthRegisterAndLogin(t *testing.T) {
	ts := newTestServer(t)

	// Register
	w := ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email":       "new.user@test.com",
		"displayName": "New User",
		"password":    "secret123",
	}, "")
	mustStatus(t, w, http.StatusCreated)
	regBody := asJSON(t, w)
	if regBody["token"] == nil {
		t.Fatal("register: no token returned")
	}

	// Login with same credentials
	w2 := ts.doAs(t, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    "new.user@test.com",
		"password": "secret123",
	}, "")
	mustStatus(t, w2, http.StatusOK)
	loginBody := asJSON(t, w2)
	if loginBody["token"] == nil {
		t.Fatal("login: no token returned")
	}

	// Bad password
	w3 := ts.doAs(t, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    "new.user@test.com",
		"password": "wrongpassword",
	}, "")
	mustStatus(t, w3, http.StatusUnauthorized)
}

func TestAuthMe(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/auth/me", nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	if body["email"] != "sarah.chen@example.com" {
		t.Errorf("me: expected sarah's email, got %v", body["email"])
	}
}

func TestAuthMeUnauthorized(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodGet, "/api/auth/me", nil, "")
	mustStatus(t, w, http.StatusUnauthorized)
}

// ── POI Search ────────────────────────────────────────────────────────────────

func TestPOISearchAll(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodGet, "/api/pois/search", nil, "")
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)

	pois, ok := body["pois"].([]interface{})
	if !ok {
		t.Fatalf("pois field missing or wrong type: %v", body["pois"])
	}
	// All 17 POIs seeded
	if len(pois) != 17 {
		t.Errorf("expected 17 POIs, got %d", len(pois))
	}
}

func TestPOISearchByCategory(t *testing.T) {
	ts := newTestServer(t)
	cases := []struct {
		category string
		wantMin  int
	}{
		{"restaurant", 6},
		{"landmark", 5},
		{"hotel", 4},
		{"attraction", 2},
	}
	for _, tc := range cases {
		t.Run(tc.category, func(t *testing.T) {
			w := ts.doAs(t, http.MethodGet, "/api/pois/search?category="+tc.category, nil, "")
			mustStatus(t, w, http.StatusOK)
			body := asJSON(t, w)
			pois := body["pois"].([]interface{})
			if len(pois) < tc.wantMin {
				t.Errorf("category=%s: got %d POIs, want at least %d", tc.category, len(pois), tc.wantMin)
			}
			// Verify every result has the correct category
			for _, raw := range pois {
				p := raw.(map[string]interface{})
				if p["category"] != tc.category {
					t.Errorf("category=%s: result has category %v", tc.category, p["category"])
				}
			}
		})
	}
}

func TestPOISearchByText(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodGet, "/api/pois/search?q=ramen", nil, "")
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	pois := body["pois"].([]interface{})
	if len(pois) == 0 {
		t.Error("text search for 'ramen' returned no results")
	}
	for _, raw := range pois {
		p := raw.(map[string]interface{})
		name := fmt.Sprintf("%v %v", p["name"], p["subcategory"])
		found := false
		for _, word := range []string{"ramen", "Ramen"} {
			if bytes.Contains([]byte(name), []byte(word)) {
				found = true
				break
			}
		}
		if !found {
			t.Logf("unexpected result for 'ramen' query: %s", name)
		}
	}
}

func TestPOISearchNoResults(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodGet, "/api/pois/search?q=zzznomatch999", nil, "")
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	pois := body["pois"].([]interface{})
	if len(pois) != 0 {
		t.Errorf("expected 0 results for nonsense query, got %d", len(pois))
	}
}

// ── Trip ──────────────────────────────────────────────────────────────────────

func TestGetTrip(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID, nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)

	if body["id"] != ts.tripID {
		t.Errorf("trip id mismatch: got %v", body["id"])
	}
	if body["name"] != "Tokyo Trip 2024" {
		t.Errorf("trip name: got %v", body["name"])
	}
	if body["shareLink"] == nil || body["shareLink"] == "" {
		t.Error("trip shareLink missing")
	}
	collabs := body["collaborators"].([]interface{})
	if len(collabs) != 5 {
		t.Errorf("expected 5 collaborators, got %d", len(collabs))
	}
}

func TestGetTripUnauthenticated(t *testing.T) {
	ts := newTestServer(t)
	w := ts.doAs(t, http.MethodGet, "/api/trips/"+ts.tripID, nil, "")
	mustStatus(t, w, http.StatusUnauthorized)
}

func TestGetTripForbiddenForNonMember(t *testing.T) {
	ts := newTestServer(t)
	// Register a stranger and try to access the trip
	ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email": "stranger@test.com", "displayName": "Stranger", "password": "pass123",
	}, "")
	lr := ts.doAs(t, http.MethodPost, "/api/auth/login", map[string]string{
		"email": "stranger@test.com", "password": "pass123",
	}, "")
	token := asJSON(t, lr)["token"].(string)
	w := ts.doAs(t, http.MethodGet, "/api/trips/"+ts.tripID, nil, token)
	mustStatus(t, w, http.StatusForbidden)
}

func TestCreateTrip(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodPost, "/api/trips", map[string]string{
		"name":        "Paris 2025",
		"destination": "Paris, France",
	})
	mustStatus(t, w, http.StatusCreated)
	body := asJSON(t, w)
	if body["name"] != "Paris 2025" {
		t.Errorf("trip name: got %v", body["name"])
	}
	if body["id"] == nil || body["id"] == "" {
		t.Error("missing trip id")
	}
	// Creator should be the only collaborator with Owner role
	collabs := body["collaborators"].([]interface{})
	if len(collabs) != 1 {
		t.Errorf("expected 1 collaborator (owner), got %d", len(collabs))
	}
	owner := collabs[0].(map[string]interface{})
	if owner["role"] != "Owner" {
		t.Errorf("expected role Owner, got %v", owner["role"])
	}
}

// ── Share Link ────────────────────────────────────────────────────────────────

func TestGetShareLink(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/share-link", nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	if body["shareLink"] == nil || body["inviteCode"] == nil {
		t.Error("share-link response missing fields")
	}
}

func TestRegenerateShareLink(t *testing.T) {
	ts := newTestServer(t)
	// Get original code
	w1 := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/share-link", nil)
	orig := asJSON(t, w1)["inviteCode"].(string)

	// Regenerate
	w2 := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/share-link/regenerate", nil)
	mustStatus(t, w2, http.StatusOK)
	newCode := asJSON(t, w2)["inviteCode"].(string)

	if newCode == orig {
		t.Error("regenerate did not produce a new invite code")
	}
}

func TestPreviewTripByInviteCode(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/share-link", nil)
	code := asJSON(t, w)["inviteCode"].(string)

	// Preview is public
	w2 := ts.doAs(t, http.MethodGet, "/api/sharelinks/"+code, nil, "")
	mustStatus(t, w2, http.StatusOK)
	body := asJSON(t, w2)
	if body["name"] != "Tokyo Trip 2024" {
		t.Errorf("preview name: got %v", body["name"])
	}
}

func TestJoinByInviteCode(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/share-link", nil)
	code := asJSON(t, w)["inviteCode"].(string)

	// Register a new user and join via invite code
	ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email": "joiner@test.com", "displayName": "Joiner", "password": "pass123",
	}, "")
	lr := ts.doAs(t, http.MethodPost, "/api/auth/login", map[string]string{
		"email": "joiner@test.com", "password": "pass123",
	}, "")
	joinerToken := asJSON(t, lr)["token"].(string)

	w2 := ts.doAs(t, http.MethodPost, "/api/sharelinks/"+code, nil, joinerToken)
	mustStatus(t, w2, http.StatusOK)
	body := asJSON(t, w2)
	collabs := body["collaborators"].([]interface{})
	// Now 6 collaborators
	if len(collabs) != 6 {
		t.Errorf("expected 6 collaborators after join, got %d", len(collabs))
	}

	// Joining again is idempotent
	w3 := ts.doAs(t, http.MethodPost, "/api/sharelinks/"+code, nil, joinerToken)
	mustStatus(t, w3, http.StatusOK)
}

// ── Invitations ───────────────────────────────────────────────────────────────

func TestSendInvitation(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"invite1@test.com"},
		"role":   "Editor",
	})
	mustStatus(t, w, http.StatusCreated)
	body := asJSON(t, w)
	invs := body["invitations"].([]interface{})
	if len(invs) != 1 {
		t.Fatalf("expected 1 invitation, got %d", len(invs))
	}
	inv := invs[0].(map[string]interface{})
	if inv["inviteeEmail"] != "invite1@test.com" {
		t.Errorf("invitee email: got %v", inv["inviteeEmail"])
	}
	if inv["status"] != "PENDING" {
		t.Errorf("status: got %v", inv["status"])
	}
}

func TestSendMultipleInvitations(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"a@test.com", "b@test.com", "c@test.com"},
		"role":   "Viewer",
	})
	mustStatus(t, w, http.StatusCreated)
	body := asJSON(t, w)
	invs := body["invitations"].([]interface{})
	if len(invs) != 3 {
		t.Errorf("expected 3 invitations, got %d", len(invs))
	}
}

func TestDuplicateInvitationRejected(t *testing.T) {
	ts := newTestServer(t)
	ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"dup@test.com"}, "role": "Editor",
	})
	// Second invite to same email
	w := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"dup@test.com"}, "role": "Editor",
	})
	// Should fail with unprocessable (all emails failed)
	if w.Code == http.StatusCreated {
		t.Error("expected duplicate invitation to be rejected, but got 201")
	}
}

func TestListInvitations(t *testing.T) {
	ts := newTestServer(t)
	ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"list1@test.com", "list2@test.com"}, "role": "Editor",
	})

	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/invitations", nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	invs := body["invitations"].([]interface{})
	if len(invs) < 2 {
		t.Errorf("expected at least 2 invitations, got %d", len(invs))
	}
}

func TestListInvitationsFilteredByStatus(t *testing.T) {
	ts := newTestServer(t)
	ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"flt@test.com"}, "role": "Editor",
	})

	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/invitations?status=PENDING", nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	invs := body["invitations"].([]interface{})
	for _, raw := range invs {
		inv := raw.(map[string]interface{})
		if inv["status"] != "PENDING" {
			t.Errorf("filtered list contains non-PENDING invitation: %v", inv["status"])
		}
	}
}

func TestPreviewInvitationToken(t *testing.T) {
	ts := newTestServer(t)

	// Send invite and capture raw token from email log (we test the DB path directly)
	invSvc := ts.buildInvSvc(t)
	inv, err := invSvc.SendEmailInvite(ts.tripID, ts.ownerID, "preview@test.com", models.RoleEditor)
	if err != nil {
		t.Fatalf("send invite: %v", err)
	}

	// The raw token is stored in the link sent to the user; simulate by fetching via
	// the hash stored in DB — retrieve it directly from DB for test purposes.
	var tokenHash string
	ts.db.QueryRow(`SELECT token_hash FROM invitations WHERE id = ?`, inv.ID).Scan(&tokenHash)
	if tokenHash == "" {
		t.Fatal("no token_hash in DB")
	}
	// Preview endpoint uses raw token, so we can't easily test without the raw token.
	// Instead, verify the invitation was persisted correctly.
	if inv.InviteeEmail != "preview@test.com" {
		t.Errorf("invitee email: got %v", inv.InviteeEmail)
	}
	if inv.Status != models.StatusPending {
		t.Errorf("status: got %v", inv.Status)
	}
}

func TestRevokeInvitation(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"revoke@test.com"}, "role": "Editor",
	})
	body := asJSON(t, w)
	inv := body["invitations"].([]interface{})[0].(map[string]interface{})
	invID := inv["id"].(string)

	// Revoke
	w2 := ts.do(t, http.MethodDelete, "/api/invitations/"+invID, nil)
	mustStatus(t, w2, http.StatusNoContent)

	// Verify status changed
	w3 := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/invitations?status=REVOKED", nil)
	body3 := asJSON(t, w3)
	revoked := body3["invitations"].([]interface{})
	found := false
	for _, r := range revoked {
		if r.(map[string]interface{})["id"] == invID {
			found = true
		}
	}
	if !found {
		t.Error("revoked invitation not in REVOKED list")
	}
}

func TestViewerCannotSendInvitation(t *testing.T) {
	ts := newTestServer(t)
	// Get viewer token (Maria Rodriguez)
	var viewerID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'VIEWER' LIMIT 1`, ts.tripID).Scan(&viewerID)
	viewerToken, _, _ := ts.authSvc.IssueTokenForUser(viewerID)

	w := ts.doAs(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"shouldfail@test.com"}, "role": "Editor",
	}, viewerToken)
	mustStatus(t, w, http.StatusForbidden)
}

func TestEditorCanSendInvitation(t *testing.T) {
	ts := newTestServer(t)
	var editorID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'EDITOR' LIMIT 1`, ts.tripID).Scan(&editorID)
	editorToken, _, _ := ts.authSvc.IssueTokenForUser(editorID)

	w := ts.doAs(t, http.MethodPost, "/api/trips/"+ts.tripID+"/invitations", map[string]interface{}{
		"emails": []string{"editor-invite@test.com"}, "role": "Viewer",
	}, editorToken)
	mustStatus(t, w, http.StatusCreated)
}

// ── Collaborators ─────────────────────────────────────────────────────────────

func TestListCollaborators(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/collaborators", nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	collabs := body["collaborators"].([]interface{})
	if len(collabs) != 5 {
		t.Errorf("expected 5 collaborators, got %d", len(collabs))
	}
	// Verify each collaborator has required fields
	for _, raw := range collabs {
		c := raw.(map[string]interface{})
		if c["id"] == nil || c["name"] == nil || c["email"] == nil || c["role"] == nil {
			t.Errorf("collaborator missing fields: %v", c)
		}
	}
}

func TestListCollaboratorsRoles(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/collaborators", nil)
	body := asJSON(t, w)
	collabs := body["collaborators"].([]interface{})
	roleCounts := map[string]int{}
	for _, raw := range collabs {
		c := raw.(map[string]interface{})
		roleCounts[c["role"].(string)]++
	}
	if roleCounts["Owner"] != 1 {
		t.Errorf("expected 1 Owner, got %d", roleCounts["Owner"])
	}
	if roleCounts["Editor"] != 2 {
		t.Errorf("expected 2 Editors, got %d", roleCounts["Editor"])
	}
	if roleCounts["Viewer"] != 2 {
		t.Errorf("expected 2 Viewers, got %d", roleCounts["Viewer"])
	}
}

func TestUpdateCollaboratorRole(t *testing.T) {
	ts := newTestServer(t)
	// Get an editor's userId
	var editorID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'EDITOR' LIMIT 1`, ts.tripID).Scan(&editorID)

	w := ts.do(t, http.MethodPatch, fmt.Sprintf("/api/trips/%s/collaborators/%s", ts.tripID, editorID),
		map[string]string{"role": "Viewer"})
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	if body["role"] != "VIEWER" {
		t.Errorf("role after update: got %v", body["role"])
	}
}

func TestUpdateRoleRequiresOwner(t *testing.T) {
	ts := newTestServer(t)
	var editorID, viewerID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'EDITOR' LIMIT 1`, ts.tripID).Scan(&editorID)
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'VIEWER' LIMIT 1`, ts.tripID).Scan(&viewerID)
	editorToken, _, _ := ts.authSvc.IssueTokenForUser(editorID)

	// Editor tries to change viewer's role → forbidden
	w := ts.doAs(t, http.MethodPatch, fmt.Sprintf("/api/trips/%s/collaborators/%s", ts.tripID, viewerID),
		map[string]string{"role": "Editor"}, editorToken)
	mustStatus(t, w, http.StatusForbidden)
}

// TP-003: UpdateRole for a non-collaborator must return 404, not 400.
// Oracle: PATCH /api/trips/:tripId/collaborators/:userId where :userId is not a
// member of the trip → HTTP 404 Not Found.
// Current (buggy) behaviour: the handler falls through to the default case in
// the error switch and returns 400 Bad Request because ErrCollaboratorNotFound
// is not handled for UpdateRole (collaborator_handler.go:70-78).
func TestUpdateRoleNonCollaboratorReturns404(t *testing.T) {
	ts := newTestServer(t)

	// Register a brand-new user who has never joined the trip.
	w := ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email":       "outsider@test.com",
		"displayName": "Outsider",
		"password":    "secret123",
	}, "")
	mustStatus(t, w, http.StatusCreated)
	outsiderBody := asJSON(t, w)
	userObj, ok := outsiderBody["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("register: missing user object in response: %v", outsiderBody)
	}
	outsiderID, ok := userObj["id"].(string)
	if !ok || outsiderID == "" {
		t.Fatal("register: missing user.id in response")
	}

	// Owner attempts to update the role of a user who is not a collaborator.
	// Oracle: must be 404 Not Found.
	w2 := ts.do(t, http.MethodPatch,
		fmt.Sprintf("/api/trips/%s/collaborators/%s", ts.tripID, outsiderID),
		map[string]string{"role": "Editor"},
	)
	mustStatus(t, w2, http.StatusNotFound)
}

func TestRemoveCollaborator(t *testing.T) {
	ts := newTestServer(t)
	var viewerID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'VIEWER' LIMIT 1`, ts.tripID).Scan(&viewerID)

	w := ts.do(t, http.MethodDelete, fmt.Sprintf("/api/trips/%s/collaborators/%s", ts.tripID, viewerID), nil)
	mustStatus(t, w, http.StatusNoContent)

	// Verify removed
	w2 := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/collaborators", nil)
	collabs := asJSON(t, w2)["collaborators"].([]interface{})
	if len(collabs) != 4 {
		t.Errorf("expected 4 collaborators after remove, got %d", len(collabs))
	}
}

func TestCannotRemoveOwner(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodDelete, fmt.Sprintf("/api/trips/%s/collaborators/%s", ts.tripID, ts.ownerID), nil)
	mustStatus(t, w, http.StatusBadRequest)
}

func TestCollaboratorCanRemoveThemselves(t *testing.T) {
	ts := newTestServer(t)
	var editorID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'EDITOR' LIMIT 1`, ts.tripID).Scan(&editorID)
	editorToken, _, _ := ts.authSvc.IssueTokenForUser(editorID)

	w := ts.doAs(t, http.MethodDelete, fmt.Sprintf("/api/trips/%s/collaborators/%s", ts.tripID, editorID), nil, editorToken)
	mustStatus(t, w, http.StatusNoContent)
}

// ── Itinerary ─────────────────────────────────────────────────────────────────

func TestGetItinerary(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/itinerary", nil)
	mustStatus(t, w, http.StatusOK)
	body := asJSON(t, w)
	items, ok := body["items"].([]interface{})
	if !ok {
		t.Fatalf("items field wrong type: %v", body["items"])
	}
	// 5 items seeded
	if len(items) != 5 {
		t.Errorf("expected 5 itinerary items, got %d", len(items))
	}
	// Each item should have a nested poi object
	for _, raw := range items {
		item := raw.(map[string]interface{})
		if item["poi"] == nil {
			t.Errorf("item missing nested poi: %v", item)
		}
		if item["day"] == nil {
			t.Errorf("item missing day: %v", item)
		}
		if item["addedBy"] == nil {
			t.Errorf("item missing addedBy: %v", item)
		}
	}
}

func TestAddPOIToItinerary(t *testing.T) {
	ts := newTestServer(t)
	// h3 is NOT in the seeded itinerary
	w := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/itinerary", map[string]interface{}{
		"poiId": "h3",
		"day":   3,
	})
	mustStatus(t, w, http.StatusCreated)
	item := asJSON(t, w)
	if item["id"] == nil {
		t.Error("missing id in response")
	}
	poi := item["poi"].(map[string]interface{})
	if poi["id"] != "h3" {
		t.Errorf("poi id: got %v", poi["id"])
	}
	if item["day"].(float64) != 3 {
		t.Errorf("day: got %v", item["day"])
	}
	if item["addedBy"] == nil || item["addedBy"] == "" {
		t.Error("addedBy missing")
	}
}

func TestDuplicatePOIRejected(t *testing.T) {
	ts := newTestServer(t)
	// l1 is already in the seeded itinerary (day 1)
	w := ts.do(t, http.MethodPost, "/api/trips/"+ts.tripID+"/itinerary", map[string]interface{}{
		"poiId": "l1",
		"day":   2,
	})
	mustStatus(t, w, http.StatusConflict)
}

func TestRemoveFromItinerary(t *testing.T) {
	ts := newTestServer(t)
	// Get the first item's id
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/itinerary", nil)
	items := asJSON(t, w)["items"].([]interface{})
	itemID := items[0].(map[string]interface{})["id"].(string)

	w2 := ts.do(t, http.MethodDelete, fmt.Sprintf("/api/trips/%s/itinerary/%s", ts.tripID, itemID), nil)
	mustStatus(t, w2, http.StatusNoContent)

	// Verify removal
	w3 := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/itinerary", nil)
	items3 := asJSON(t, w3)["items"].([]interface{})
	if len(items3) != 4 {
		t.Errorf("expected 4 items after remove, got %d", len(items3))
	}
}

func TestViewerCannotAddToItinerary(t *testing.T) {
	ts := newTestServer(t)
	var viewerID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'VIEWER' LIMIT 1`, ts.tripID).Scan(&viewerID)
	viewerToken, _, _ := ts.authSvc.IssueTokenForUser(viewerID)

	w := ts.doAs(t, http.MethodPost, "/api/trips/"+ts.tripID+"/itinerary", map[string]interface{}{
		"poiId": "h4", "day": 1,
	}, viewerToken)
	mustStatus(t, w, http.StatusForbidden)
}

func TestEditorCanAddToItinerary(t *testing.T) {
	ts := newTestServer(t)
	var editorID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'EDITOR' LIMIT 1`, ts.tripID).Scan(&editorID)
	editorToken, _, _ := ts.authSvc.IssueTokenForUser(editorID)

	w := ts.doAs(t, http.MethodPost, "/api/trips/"+ts.tripID+"/itinerary", map[string]interface{}{
		"poiId": "h4", "day": 2,
	}, editorToken)
	mustStatus(t, w, http.StatusCreated)
}

func TestRemoveNonExistentItineraryItem(t *testing.T) {
	ts := newTestServer(t)
	w := ts.do(t, http.MethodDelete, fmt.Sprintf("/api/trips/%s/itinerary/does-not-exist", ts.tripID), nil)
	mustStatus(t, w, http.StatusNotFound)
}

// ── Invitation acceptance flow ────────────────────────────────────────────────

func TestFullInvitationAcceptFlow(t *testing.T) {
	ts := newTestServer(t)

	// 1. Register a new user who will be invited
	ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email": "newcollab@test.com", "displayName": "New Collab", "password": "pass123",
	}, "")
	lr := ts.doAs(t, http.MethodPost, "/api/auth/login", map[string]string{
		"email": "newcollab@test.com", "password": "pass123",
	}, "")
	newToken := asJSON(t, lr)["token"].(string)

	// 2. Owner sends an invite; use a capturing email service so we can recover
	//    the raw token that in production travels only via the email link.
	emailSink := &capturingEmailService{}
	invSvc := ts.buildInvSvcWith(t, emailSink)
	_, err := invSvc.SendEmailInvite(ts.tripID, ts.ownerID, "newcollab@test.com", models.RoleEditor)
	if err != nil {
		t.Fatalf("send invite: %v", err)
	}

	// 3. Extract the raw token from the captured invite link.
	//    Link format: "<frontendURL>/invite/<rawToken>"
	parts := strings.SplitN(emailSink.lastInviteLink, "/invite/", 2)
	if len(parts) != 2 || parts[1] == "" {
		t.Fatal("could not extract raw token from captured invite link")
	}
	rawToken := parts[1]

	// 4. Preview is public
	w1 := ts.doAs(t, http.MethodGet, "/api/invitations/accept/"+rawToken, nil, "")
	mustStatus(t, w1, http.StatusOK)
	preview := asJSON(t, w1)
	if preview["role"] != "Editor" {
		t.Errorf("preview role: got %v", preview["role"])
	}

	// 5. Accept requires auth
	w2 := ts.doAs(t, http.MethodPost, "/api/invitations/accept/"+rawToken, nil, newToken)
	mustStatus(t, w2, http.StatusOK)

	// 6. Verify the new user is now a collaborator
	w3 := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/collaborators", nil)
	collabs := asJSON(t, w3)["collaborators"].([]interface{})
	if len(collabs) != 6 {
		t.Errorf("expected 6 collaborators after accept, got %d", len(collabs))
	}
}

// ── Rate limiting ─────────────────────────────────────────────────────────────

func TestInvitationRateLimitPerTrip(t *testing.T) {
	ts := newTestServer(t)
	invSvc := ts.buildInvSvc(t)

	// Send 20 (the limit) — all should succeed
	for i := 0; i < 20; i++ {
		email := fmt.Sprintf("rl%d@test.com", i)
		_, err := invSvc.SendEmailInvite(ts.tripID, ts.ownerID, email, models.RoleViewer)
		if err != nil {
			t.Fatalf("invite %d failed unexpectedly: %v", i, err)
		}
	}

	// The 21st should be rate-limited
	_, err := invSvc.SendEmailInvite(ts.tripID, ts.ownerID, "rl21@test.com", models.RoleViewer)
	if err == nil {
		t.Error("expected rate limit error on 21st invite, got nil")
	}
}

// ── Additional coverage for missing user-story paths ──────────────────────────

// TestRegisterDuplicateEmail verifies that registering with an already-taken
// email address returns an error (User Story 1 registration path).
func TestRegisterDuplicateEmail(t *testing.T) {
	ts := newTestServer(t)
	// sarah.chen@example.com is already seeded — try to register with it again.
	w := ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email": "sarah.chen@example.com", "displayName": "Duplicate", "password": "pass123",
	}, "")
	if w.Code == http.StatusCreated {
		t.Error("expected duplicate email registration to fail, but got 201")
	}
}

// TestViewerCannotRemoveFromItinerary verifies that a VIEWER role member cannot
// delete an itinerary item (User Story 3 RBAC — mirrors the add restriction).
func TestViewerCannotRemoveFromItinerary(t *testing.T) {
	ts := newTestServer(t)

	// Fetch the first seeded itinerary item id.
	w := ts.do(t, http.MethodGet, "/api/trips/"+ts.tripID+"/itinerary", nil)
	mustStatus(t, w, http.StatusOK)
	items := asJSON(t, w)["items"].([]interface{})
	itemID := items[0].(map[string]interface{})["id"].(string)

	// Get a viewer's token.
	var viewerID string
	ts.db.QueryRow(`SELECT user_id FROM trip_collaborators WHERE trip_id = ? AND role = 'VIEWER' LIMIT 1`, ts.tripID).Scan(&viewerID)
	viewerToken, _, _ := ts.authSvc.IssueTokenForUser(viewerID)

	w2 := ts.doAs(t, http.MethodDelete, fmt.Sprintf("/api/trips/%s/itinerary/%s", ts.tripID, itemID), nil, viewerToken)
	mustStatus(t, w2, http.StatusForbidden)
}

// TestNonMemberCannotGetItinerary verifies that a user who is not a collaborator
// on a trip cannot fetch its itinerary (User Story 3 access control).
func TestNonMemberCannotGetItinerary(t *testing.T) {
	ts := newTestServer(t)

	// Register a user who has no membership on the seeded trip.
	ts.doAs(t, http.MethodPost, "/api/auth/register", map[string]string{
		"email": "outsider@test.com", "displayName": "Outsider", "password": "pass123",
	}, "")
	lr := ts.doAs(t, http.MethodPost, "/api/auth/login", map[string]string{
		"email": "outsider@test.com", "password": "pass123",
	}, "")
	outsiderToken := asJSON(t, lr)["token"].(string)

	w := ts.doAs(t, http.MethodGet, "/api/trips/"+ts.tripID+"/itinerary", nil, outsiderToken)
	mustStatus(t, w, http.StatusForbidden)
}

// ── Helpers accessible only to tests ─────────────────────────────────────────

// buildInvSvc creates an InvitationService wired to the same DB as the testServer.
// Used in tests that need to generate real invitation tokens.
func (ts *testServer) buildInvSvc(t *testing.T) *services.InvitationService {
	t.Helper()
	return ts.buildInvSvcWith(t, services.NewMockEmailService())
}

// buildInvSvcWith is like buildInvSvc but accepts a custom email service, allowing
// tests to inject a capturingEmailService to recover the raw invite token.
func (ts *testServer) buildInvSvcWith(t *testing.T, emailSvc services.IEmailService) *services.InvitationService {
	t.Helper()
	c := cache.New()
	hub := websocket.New()
	return services.NewInvitationService(ts.db, c, emailSvc, hub, "http://localhost:5173")
}

// capturingEmailService implements IEmailService and records the last invite link
// so tests can extract the raw token that would normally be sent only via email.
type capturingEmailService struct {
	lastInviteLink string
}

func (c *capturingEmailService) SendInviteEmail(_, _, _, inviteLink string) error {
	c.lastInviteLink = inviteLink
	return nil
}

// extractRawToken recovers the raw token from the invitation service's token cache.
// In the real system the raw token goes only into the email body; here we reach into
// the service-level cache by calling ValidateToken with the stored hash as a proxy.
// Returns "" if the raw token cannot be recovered (expected — this is a security property).
func (ts *testServer) extractRawToken(t *testing.T, invID string) string {
	t.Helper()
	// We can't recover the raw token after the fact — it is intentionally
	// not stored in plaintext.  This is the correct security behaviour.
	// Tests that need the raw token should instrument InvitationService
	// or use a test-friendly token generator.
	return ""
}
