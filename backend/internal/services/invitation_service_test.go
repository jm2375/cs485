package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	appdb "cs485/internal/db"
	"cs485/internal/cache"
	"cs485/internal/models"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ── Mocks ────────────────────────────────────────────────────────────────────

type captureEmailService struct {
	mu    sync.Mutex
	calls []struct{ to, inviterName, tripName, inviteLink string }
	err   error
}

func (m *captureEmailService) SendInviteEmail(to, inviterName, tripName, inviteLink string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, struct{ to, inviterName, tripName, inviteLink string }{to, inviterName, tripName, inviteLink})
	return m.err
}

type captureHub struct {
	mu     sync.Mutex
	events []struct{ tripID, event string; data interface{} }
}

func (h *captureHub) BroadcastToTrip(tripID, event string, data interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, struct{ tripID, event string; data interface{} }{tripID, event, data})
}

func (h *captureHub) GetOnlineUsersInTrip(tripID string) []string { return nil }

type errScanner struct{ err error }

func (e *errScanner) Scan(dest ...interface{}) error { return e.err }

// ── Test infrastructure ───────────────────────────────────────────────────────

type testBundle struct {
	svc   *InvitationService
	email *captureEmailService
	hub   *captureHub
	cache *cache.Store
	db    *sql.DB
}

func newTestBundle(t *testing.T) *testBundle {
	t.Helper()
	dsn := fmt.Sprintf("file:invtest%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c := cache.New()
	emailSvc := &captureEmailService{}
	hub := &captureHub{}
	svc := NewInvitationService(db, c, emailSvc, hub, "https://app.example.com")
	return &testBundle{svc, emailSvc, hub, c, db}
}

func insertUser(t *testing.T, db *sql.DB, email, displayName string) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash, created_at) VALUES ($1,$2,$3,'x',$4)`,
		id, email, displayName, fmtTime(time.Now()),
	); err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	return id
}

func insertTrip(t *testing.T, db *sql.DB, name, destination, ownerID string) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO trips (id, name, destination, invite_code, owner_id, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		id, name, destination, uuid.New().String(), ownerID, fmtTime(time.Now()),
	); err != nil {
		t.Fatalf("insertTrip: %v", err)
	}
	return id
}

func insertCollaborator(t *testing.T, db *sql.DB, tripID, userID, role string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO trip_collaborators (id, trip_id, user_id, role, joined_at) VALUES ($1,$2,$3,$4,$5)`,
		uuid.New().String(), tripID, userID, role, fmtTime(time.Now()),
	); err != nil {
		t.Fatalf("insertCollaborator: %v", err)
	}
}

func insertInvitation(t *testing.T, db *sql.DB, tripID, inviterID, email, tokenHash, role string, status models.InvitationStatus, expiresAt time.Time) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO invitations (id, trip_id, inviter_id, invitee_email, token_hash, role, status, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, tripID, inviterID, email, tokenHash, role, string(status), fmtTime(expiresAt), fmtTime(time.Now()),
	); err != nil {
		t.Fatalf("insertInvitation: %v", err)
	}
	return id
}

// ── 1. NewInvitationService ───────────────────────────────────────────────────

// 1.1 — All dependency fields are set correctly on the returned service.
func TestNewInvitationService_WiresAllFields(t *testing.T) {
	db := func() *sql.DB {
		dsn := fmt.Sprintf("file:wire%d?mode=memory&cache=shared", time.Now().UnixNano())
		d, _ := sql.Open("sqlite", dsn)
		t.Cleanup(func() { d.Close() })
		return d
	}()
	c := cache.New()
	emailSvc := &captureEmailService{}
	hub := &captureHub{}
	frontendURL := "https://app.example.com"

	svc := NewInvitationService(db, c, emailSvc, hub, frontendURL)

	if svc == nil {
		t.Fatal("expected non-nil *InvitationService")
	}
	if svc.db != db {
		t.Error("db field not wired correctly")
	}
	if svc.cache != c {
		t.Error("cache field not wired correctly")
	}
	if svc.emailService != emailSvc {
		t.Error("emailService field not wired correctly")
	}
	if svc.hub != hub {
		t.Error("hub field not wired correctly")
	}
	if svc.frontendURL != frontendURL {
		t.Errorf("frontendURL: got %q, want %q", svc.frontendURL, frontendURL)
	}
}

// ── 2. GetTripInfo ────────────────────────────────────────────────────────────

// 2.1 — Returns name and destination for a trip that exists in the database.
func TestGetTripInfo_KnownTrip(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Italy 2026", "Rome", ownerID)

	name, destination, err := b.svc.GetTripInfo(tripID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Italy 2026" {
		t.Errorf("name: got %q, want %q", name, "Italy 2026")
	}
	if destination != "Rome" {
		t.Errorf("destination: got %q, want %q", destination, "Rome")
	}
}

// 2.2 — Returns sql.ErrNoRows for a trip ID not in the database.
func TestGetTripInfo_NotFound(t *testing.T) {
	b := newTestBundle(t)

	_, _, err := b.svc.GetTripInfo("nonexistent-id")

	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

// ── 3. SendEmailInvite ────────────────────────────────────────────────────────

// 3.1 — Creates a PENDING invitation, sends one email, and persists the row.
func TestSendEmailInvite_HappyPath(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Trip Owner")
	tripID := insertTrip(t, b.db, "Tokyo Trip", "Tokyo", ownerID)

	inv, err := b.svc.SendEmailInvite(tripID, ownerID, "friend@example.com", models.RoleViewer)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv == nil {
		t.Fatal("expected non-nil *Invitation")
	}
	if inv.Status != models.StatusPending {
		t.Errorf("status: got %q, want PENDING", inv.Status)
	}
	if inv.Role != models.RoleViewer {
		t.Errorf("role: got %q, want VIEWER", inv.Role)
	}
	if time.Until(inv.ExpiresAt) < 6*24*time.Hour || time.Until(inv.ExpiresAt) > 8*24*time.Hour {
		t.Errorf("expiresAt not ~7 days from now: %v", inv.ExpiresAt)
	}
	if len(b.email.calls) != 1 || b.email.calls[0].to != "friend@example.com" {
		t.Errorf("email not sent correctly; calls: %+v", b.email.calls)
	}
	var count int
	b.db.QueryRow(`SELECT COUNT(*) FROM invitations WHERE id = $1`, inv.ID).Scan(&count)
	if count != 1 {
		t.Error("invitation row missing from DB")
	}
}

// 3.2 — Returns ErrInviteAlreadySent when a PENDING invite already exists for the same email/trip.
func TestSendEmailInvite_AlreadySent(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertInvitation(t, b.db, tripID, ownerID, "dup@example.com", "somehash", "VIEWER", models.StatusPending, time.Now().Add(24*time.Hour))

	inv, err := b.svc.SendEmailInvite(tripID, ownerID, "dup@example.com", models.RoleViewer)

	if inv != nil {
		t.Error("expected nil invitation")
	}
	if !errors.Is(err, ErrInviteAlreadySent) {
		t.Errorf("expected ErrInviteAlreadySent, got %v", err)
	}
}

// 3.3 — Returns ErrInviteRateLimited when the per-trip hourly limit (20) is exceeded.
func TestSendEmailInvite_TripRateLimit(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	tripKey := fmt.Sprintf("ratelimit:trip:%s:%s", tripID, ownerID)
	for i := 0; i < 20; i++ {
		b.cache.Incr(tripKey, time.Hour)
	}

	_, err := b.svc.SendEmailInvite(tripID, ownerID, "new@example.com", models.RoleViewer)

	if !errors.Is(err, ErrInviteRateLimited) {
		t.Errorf("expected ErrInviteRateLimited, got %v", err)
	}
}

// 3.4 — Returns ErrInviteRateLimited when the per-user daily limit (50) is exceeded.
func TestSendEmailInvite_UserRateLimit(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	userKey := fmt.Sprintf("ratelimit:user:%s", ownerID)
	for i := 0; i < 50; i++ {
		b.cache.Incr(userKey, 24*time.Hour)
	}

	_, err := b.svc.SendEmailInvite(tripID, ownerID, "new@example.com", models.RoleViewer)

	if !errors.Is(err, ErrInviteRateLimited) {
		t.Errorf("expected ErrInviteRateLimited, got %v", err)
	}
}

// 3.5 — Invitation is created and returned even when the email service errors.
func TestSendEmailInvite_EmailFailureDoesNotBlockCreation(t *testing.T) {
	b := newTestBundle(t)
	b.email.err = errors.New("SMTP down")
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)

	inv, err := b.svc.SendEmailInvite(tripID, ownerID, "friend@example.com", models.RoleEditor)

	if err != nil {
		t.Errorf("expected no error despite email failure, got %v", err)
	}
	if inv == nil {
		t.Error("expected non-nil invitation despite email failure")
	}
}

// 3.6 — Invite link sent to recipient starts with the configured frontendURL.
func TestSendEmailInvite_InviteLinkUsesRawToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)

	_, err := b.svc.SendEmailInvite(tripID, ownerID, "friend@example.com", models.RoleEditor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b.email.calls) == 0 {
		t.Fatal("no email calls recorded")
	}
	if !strings.HasPrefix(b.email.calls[0].inviteLink, "https://app.example.com/invite/") {
		t.Errorf("invite link %q missing expected prefix", b.email.calls[0].inviteLink)
	}
}

// 3.7 — Token stored in the DB is the SHA-256 hash; the raw token is never persisted.
func TestSendEmailInvite_StoresHashNotRawToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)

	inv, err := b.svc.SendEmailInvite(tripID, ownerID, "friend@example.com", models.RoleEditor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	link := b.email.calls[0].inviteLink
	parts := strings.SplitN(link, "/invite/", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected link format: %s", link)
	}
	rawToken := parts[1]
	expectedHash := hashToken(rawToken)

	if inv.TokenHash != expectedHash {
		t.Errorf("TokenHash %q does not equal sha256(rawToken)", inv.TokenHash)
	}
	if inv.TokenHash == rawToken {
		t.Error("raw token was stored instead of its hash")
	}
}

// ── 4. ValidateToken ──────────────────────────────────────────────────────────

// 4.1 — Returns the invitation for a valid, non-expired PENDING token.
func TestValidateToken_ValidPendingToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "validrawtoken"
	hash := hashToken(raw)
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusPending, time.Now().Add(7*24*time.Hour))

	inv, err := b.svc.ValidateToken(raw)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv == nil || inv.TokenHash != hash {
		t.Errorf("unexpected invitation: %+v", inv)
	}
}

// 4.2 — Returns ErrInviteNotFound for a token with no matching DB record.
func TestValidateToken_UnknownToken(t *testing.T) {
	b := newTestBundle(t)

	_, err := b.svc.ValidateToken("totallyunknowntoken")

	if !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("expected ErrInviteNotFound, got %v", err)
	}
}

// 4.3 — Returns ErrInviteExpiredOrUsed for a PENDING token past its expiry.
func TestValidateToken_ExpiredToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "expiredtoken"
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken(raw), "VIEWER", models.StatusPending, time.Now().Add(-time.Hour))

	_, err := b.svc.ValidateToken(raw)

	if !errors.Is(err, ErrInviteExpiredOrUsed) {
		t.Errorf("expected ErrInviteExpiredOrUsed, got %v", err)
	}
}

// 4.4 — Returns ErrInviteExpiredOrUsed when the invitation is already ACCEPTED.
func TestValidateToken_AcceptedToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "acceptedtoken"
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken(raw), "VIEWER", models.StatusAccepted, time.Now().Add(7*24*time.Hour))

	_, err := b.svc.ValidateToken(raw)

	if !errors.Is(err, ErrInviteExpiredOrUsed) {
		t.Errorf("expected ErrInviteExpiredOrUsed, got %v", err)
	}
}

// 4.5 — Returns ErrInviteExpiredOrUsed for a REVOKED token.
func TestValidateToken_RevokedToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "revokedtoken"
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken(raw), "VIEWER", models.StatusRevoked, time.Now().Add(7*24*time.Hour))

	_, err := b.svc.ValidateToken(raw)

	if !errors.Is(err, ErrInviteExpiredOrUsed) {
		t.Errorf("expected ErrInviteExpiredOrUsed, got %v", err)
	}
}

// ── 5. AcceptInvitation ───────────────────────────────────────────────────────

// 5.1 — Creates a collaborator, marks invitation ACCEPTED, broadcasts WS event, and clears the cache key.
func TestAcceptInvitation_HappyPath(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	inviteeID := insertUser(t, b.db, "invitee@example.com", "New User")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "accepttoken"
	hash := hashToken(raw)
	b.cache.Set("invite:"+hash, `{"invitationId":"x"}`, time.Hour)
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "EDITOR", models.StatusPending, time.Now().Add(7*24*time.Hour))

	collab, err := b.svc.AcceptInvitation(raw, inviteeID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if collab == nil {
		t.Fatal("expected non-nil *TripCollaborator")
	}
	if collab.TripID != tripID {
		t.Errorf("TripID: got %q, want %q", collab.TripID, tripID)
	}
	if collab.UserID != inviteeID {
		t.Errorf("UserID: got %q, want %q", collab.UserID, inviteeID)
	}
	if collab.Role != models.RoleEditor {
		t.Errorf("Role: got %q, want EDITOR", collab.Role)
	}
	var status string
	b.db.QueryRow(`SELECT status FROM invitations WHERE token_hash = $1`, hash).Scan(&status)
	if status != "ACCEPTED" {
		t.Errorf("invitation status: got %q, want ACCEPTED", status)
	}
	if len(b.hub.events) == 0 || b.hub.events[0].event != "collaborator_joined" {
		t.Errorf("expected collaborator_joined WS event; got %+v", b.hub.events)
	}
	if _, ok := b.cache.Get("invite:" + hash); ok {
		t.Error("expected cache key to be deleted after acceptance")
	}
}

// 5.2 — Returns ErrInviteNotFound for an unrecognised token.
func TestAcceptInvitation_TokenNotFound(t *testing.T) {
	b := newTestBundle(t)

	_, err := b.svc.AcceptInvitation("unknowntoken", "someuser")

	if !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("expected ErrInviteNotFound, got %v", err)
	}
}

// 5.3 — Returns ErrInviteExpiredOrUsed for a token whose invitation is past its expiry.
func TestAcceptInvitation_ExpiredToken(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "expiredaccept"
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken(raw), "VIEWER", models.StatusPending, time.Now().Add(-time.Hour))

	_, err := b.svc.AcceptInvitation(raw, "anyuser")

	if !errors.Is(err, ErrInviteExpiredOrUsed) {
		t.Errorf("expected ErrInviteExpiredOrUsed, got %v", err)
	}
}

// 5.4 — Returns ErrInviteExpiredOrUsed when the invitation is already ACCEPTED.
func TestAcceptInvitation_AlreadyAccepted(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "alreadyaccepted"
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken(raw), "VIEWER", models.StatusAccepted, time.Now().Add(7*24*time.Hour))

	_, err := b.svc.AcceptInvitation(raw, "anyuser")

	if !errors.Is(err, ErrInviteExpiredOrUsed) {
		t.Errorf("expected ErrInviteExpiredOrUsed, got %v", err)
	}
}

// 5.5 — A duplicate accept attempt does not insert a second collaborator row.
func TestAcceptInvitation_NoDuplicateCollaboratorOnConflict(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	inviteeID := insertUser(t, b.db, "invitee@example.com", "New User")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "idempotenttoken"
	hash := hashToken(raw)
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "EDITOR", models.StatusPending, time.Now().Add(7*24*time.Hour))

	if _, err := b.svc.AcceptInvitation(raw, inviteeID); err != nil {
		t.Fatalf("first accept failed: %v", err)
	}
	// Status is now ACCEPTED; a second call must not insert a duplicate row.
	b.svc.AcceptInvitation(raw, inviteeID) //nolint — error expected, ignored intentionally

	var count int
	b.db.QueryRow(`SELECT COUNT(*) FROM trip_collaborators WHERE trip_id = $1 AND user_id = $2`, tripID, inviteeID).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 collaborator row, got %d", count)
	}
}

// 5.6 — Transaction rolls back and invitation stays PENDING when the collaborator insert fails.
func TestAcceptInvitation_RollbackOnFKViolation(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "rollbacktoken"
	hash := hashToken(raw)
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "EDITOR", models.StatusPending, time.Now().Add(7*24*time.Hour))

	// non-existent userID violates the FK on trip_collaborators.user_id
	_, err := b.svc.AcceptInvitation(raw, "nonexistent-user-id")

	if err == nil {
		t.Error("expected error for FK violation")
	}
	var status string
	b.db.QueryRow(`SELECT status FROM invitations WHERE token_hash = $1`, hash).Scan(&status)
	if status != "PENDING" {
		t.Errorf("invitation should remain PENDING after rollback, got %q", status)
	}
}

// 5.7 — The collaborator's role matches the role stored on the invitation.
func TestAcceptInvitation_RoleMatchesInvitation(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	inviteeID := insertUser(t, b.db, "invitee@example.com", "New User")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	raw := "editortoken"
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken(raw), "EDITOR", models.StatusPending, time.Now().Add(7*24*time.Hour))

	collab, err := b.svc.AcceptInvitation(raw, inviteeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if collab.Role != models.RoleEditor {
		t.Errorf("returned role: got %q, want EDITOR", collab.Role)
	}
	var dbRole string
	b.db.QueryRow(`SELECT role FROM trip_collaborators WHERE trip_id = $1 AND user_id = $2`, tripID, inviteeID).Scan(&dbRole)
	if dbRole != "EDITOR" {
		t.Errorf("DB role: got %q, want EDITOR", dbRole)
	}
}

// ── 6. RevokeInvitation ───────────────────────────────────────────────────────

// 6.1 — Trip OWNER can revoke a PENDING invitation; status becomes REVOKED and cache key is deleted.
func TestRevokeInvitation_OwnerCanRevoke(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertCollaborator(t, b.db, tripID, ownerID, "OWNER")
	hash := hashToken("revoketoken")
	b.cache.Set("invite:"+hash, "data", time.Hour)
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusPending, time.Now().Add(7*24*time.Hour))

	err := b.svc.RevokeInvitation(invID, ownerID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	b.db.QueryRow(`SELECT status FROM invitations WHERE id = $1`, invID).Scan(&status)
	if status != "REVOKED" {
		t.Errorf("status: got %q, want REVOKED", status)
	}
	if _, ok := b.cache.Get("invite:" + hash); ok {
		t.Error("expected cache key deleted after revocation")
	}
}

// 6.2 — Trip EDITOR can revoke a PENDING invitation.
func TestRevokeInvitation_EditorCanRevoke(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	editorID := insertUser(t, b.db, "editor@example.com", "Editor")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertCollaborator(t, b.db, tripID, editorID, "EDITOR")
	hash := hashToken("editorrevoke")
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusPending, time.Now().Add(7*24*time.Hour))

	if err := b.svc.RevokeInvitation(invID, editorID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var status string
	b.db.QueryRow(`SELECT status FROM invitations WHERE id = $1`, invID).Scan(&status)
	if status != "REVOKED" {
		t.Errorf("status: got %q, want REVOKED", status)
	}
}

// 6.3 — Returns ErrNotOwner and leaves status unchanged when requester is a VIEWER.
func TestRevokeInvitation_ViewerForbidden(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	viewerID := insertUser(t, b.db, "viewer@example.com", "Viewer")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertCollaborator(t, b.db, tripID, viewerID, "VIEWER")
	hash := hashToken("viewerrevoke")
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusPending, time.Now().Add(7*24*time.Hour))

	err := b.svc.RevokeInvitation(invID, viewerID)

	if !errors.Is(err, ErrNotOwner) {
		t.Errorf("expected ErrNotOwner, got %v", err)
	}
	var status string
	b.db.QueryRow(`SELECT status FROM invitations WHERE id = $1`, invID).Scan(&status)
	if status != "PENDING" {
		t.Errorf("status should remain PENDING, got %q", status)
	}
}

// 6.4 — Returns ErrNotOwner when requester is not a collaborator on the trip at all.
func TestRevokeInvitation_NonCollaboratorForbidden(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	strangerID := insertUser(t, b.db, "stranger@example.com", "Stranger")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	hash := hashToken("strangerrevoke")
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusPending, time.Now().Add(7*24*time.Hour))

	err := b.svc.RevokeInvitation(invID, strangerID)

	if !errors.Is(err, ErrNotOwner) {
		t.Errorf("expected ErrNotOwner, got %v", err)
	}
}

// 6.5 — Returns an error containing "no longer pending" for a non-PENDING invitation.
func TestRevokeInvitation_AlreadyAcceptedReturnsError(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertCollaborator(t, b.db, tripID, ownerID, "OWNER")
	hash := hashToken("acceptedrevoke")
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusAccepted, time.Now().Add(7*24*time.Hour))

	err := b.svc.RevokeInvitation(invID, ownerID)

	if err == nil {
		t.Fatal("expected error when revoking a non-PENDING invitation")
	}
	if !strings.Contains(err.Error(), "no longer pending") {
		t.Errorf("expected 'no longer pending' in error message, got: %v", err)
	}
}

// 6.6 — Returns ErrInviteNotFound for a non-existent invitation ID.
func TestRevokeInvitation_NotFound(t *testing.T) {
	b := newTestBundle(t)

	err := b.svc.RevokeInvitation("nonexistent-id", "anyuser")

	if !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("expected ErrInviteNotFound, got %v", err)
	}
}

// ── 7. ListInvitations ────────────────────────────────────────────────────────

// 7.1 — Returns all invitations regardless of status when no filter is supplied.
func TestListInvitations_ReturnsAllStatuses(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertInvitation(t, b.db, tripID, ownerID, "a@example.com", "hash1", "VIEWER", models.StatusPending, time.Now().Add(24*time.Hour))
	insertInvitation(t, b.db, tripID, ownerID, "b@example.com", "hash2", "VIEWER", models.StatusAccepted, time.Now().Add(24*time.Hour))
	insertInvitation(t, b.db, tripID, ownerID, "c@example.com", "hash3", "VIEWER", models.StatusRevoked, time.Now().Add(24*time.Hour))

	invs, err := b.svc.ListInvitations(tripID, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invs) != 3 {
		t.Errorf("expected 3 invitations, got %d", len(invs))
	}
}

// 7.2 — Returns only PENDING invitations when filtered by status.
func TestListInvitations_FilterByPendingStatus(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	insertInvitation(t, b.db, tripID, ownerID, "a@example.com", "hash1", "VIEWER", models.StatusPending, time.Now().Add(24*time.Hour))
	insertInvitation(t, b.db, tripID, ownerID, "b@example.com", "hash2", "VIEWER", models.StatusAccepted, time.Now().Add(24*time.Hour))

	pending := models.StatusPending
	invs, err := b.svc.ListInvitations(tripID, &pending)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invs) != 1 {
		t.Errorf("expected 1 PENDING invitation, got %d", len(invs))
	}
	if invs[0].Status != models.StatusPending {
		t.Errorf("returned non-PENDING invitation: %q", invs[0].Status)
	}
}

// 7.3 — Returns an empty slice for a trip with no invitations.
func TestListInvitations_EmptyTrip(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)

	invs, err := b.svc.ListInvitations(tripID, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invs) != 0 {
		t.Errorf("expected empty slice, got %d invitations", len(invs))
	}
}

// 7.4 — Returns an error when the database query fails.
func TestListInvitations_DBError(t *testing.T) {
	b := newTestBundle(t)
	b.db.Close()

	_, err := b.svc.ListInvitations("any-trip-id", nil)

	if err == nil {
		t.Error("expected error after DB close, got nil")
	}
}

// ── 8. checkRateLimit ─────────────────────────────────────────────────────────

// 8.1 — Allows the first invite with no prior cache entries.
func TestCheckRateLimit_FirstInviteAllowed(t *testing.T) {
	b := newTestBundle(t)

	err := b.svc.checkRateLimit("user1", "trip1")

	if err != nil {
		t.Errorf("expected no error on first invite, got %v", err)
	}
}

// 8.2 — Returns ErrInviteRateLimited when the trip counter reaches 21 within one hour.
func TestCheckRateLimit_TripLimitBlocks(t *testing.T) {
	b := newTestBundle(t)
	tripKey := fmt.Sprintf("ratelimit:trip:%s:%s", "trip1", "user1")
	for i := 0; i < 20; i++ {
		b.cache.Incr(tripKey, time.Hour)
	}

	err := b.svc.checkRateLimit("user1", "trip1")

	if !errors.Is(err, ErrInviteRateLimited) {
		t.Errorf("expected ErrInviteRateLimited at trip threshold, got %v", err)
	}
}

// 8.3 — Returns ErrInviteRateLimited when the user counter reaches 51 within 24 hours.
func TestCheckRateLimit_UserLimitBlocks(t *testing.T) {
	b := newTestBundle(t)
	userKey := fmt.Sprintf("ratelimit:user:%s", "user1")
	for i := 0; i < 50; i++ {
		b.cache.Incr(userKey, 24*time.Hour)
	}

	err := b.svc.checkRateLimit("user1", "trip1")

	if !errors.Is(err, ErrInviteRateLimited) {
		t.Errorf("expected ErrInviteRateLimited at user threshold, got %v", err)
	}
}

// ── 9. generateToken ──────────────────────────────────────────────────────────

// 9.1a — Returns a non-empty string with no error.
func TestGenerateToken_NonEmpty(t *testing.T) {
	token, err := generateToken()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

// 9.1b — Output contains only URL-safe Base64 characters.
func TestGenerateToken_URLSafeBase64Alphabet(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range token {
		isAlpha := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		isDigit := c >= '0' && c <= '9'
		isURLSafe := c == '-' || c == '_' || c == '='
		if !isAlpha && !isDigit && !isURLSafe {
			t.Errorf("token contains non-URL-safe character %q", c)
		}
	}
}

// 9.2 — Two successive calls return different tokens.
func TestGenerateToken_UniqueOnSuccessiveCalls(t *testing.T) {
	t1, err1 := generateToken()
	t2, err2 := generateToken()

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if t1 == t2 {
		t.Error("two successive calls returned the same token")
	}
}

// 9.3 — Encoded output is exactly 44 characters (Base64 of 32 random bytes).
func TestGenerateToken_Length44(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token) != 44 {
		t.Errorf("expected token length 44 (base64 of 32 bytes), got %d", len(token))
	}
}

// ── 10. hashToken ─────────────────────────────────────────────────────────────

// 10.1 — Produces the correct lowercase hex-encoded SHA-256 digest for a known input.
func TestHashToken_KnownSHA256Value(t *testing.T) {
	// echo -n "hello" | sha256sum
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got := hashToken("hello")
	if got != want {
		t.Errorf("hashToken(%q) = %q, want %q", "hello", got, want)
	}
}

// 10.2 — Same input always yields the same output.
func TestHashToken_Deterministic(t *testing.T) {
	h1 := hashToken("repeatme")
	h2 := hashToken("repeatme")
	if h1 != h2 {
		t.Errorf("hashToken not deterministic: %q != %q", h1, h2)
	}
}

// 10.3 — Different inputs produce different hashes.
func TestHashToken_DifferentInputsDifferentOutputs(t *testing.T) {
	if hashToken("tokenA") == hashToken("tokenB") {
		t.Error("different inputs produced the same hash")
	}
}

// ── 11. scanInvitation ────────────────────────────────────────────────────────

// 11.1 — All nine columns are mapped to the correct Invitation fields, including parsed time and enum types.
func TestScanInvitation_MapsAllFieldsCorrectly(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	hash := hashToken("scantest")
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "EDITOR", models.StatusPending, time.Now().Add(7*24*time.Hour))

	// scanInvitation is exercised indirectly via getByID, which wraps a real DB row.
	inv, err := b.svc.getByID(invID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.ID != invID {
		t.Errorf("ID: got %q, want %q", inv.ID, invID)
	}
	if inv.Role != models.RoleEditor {
		t.Errorf("Role: got %q, want EDITOR", inv.Role)
	}
	if inv.Status != models.StatusPending {
		t.Errorf("Status: got %q, want PENDING", inv.Status)
	}
	if inv.InviteeEmail != "invitee@example.com" {
		t.Errorf("InviteeEmail: got %q", inv.InviteeEmail)
	}
	if inv.ExpiresAt.IsZero() || inv.CreatedAt.IsZero() {
		t.Error("ExpiresAt or CreatedAt not parsed")
	}
}

// 11.2 — Returns nil and the scanner's error when Scan fails.
func TestScanInvitation_PropagatesScanError(t *testing.T) {
	row := &errScanner{err: sql.ErrNoRows}

	inv, err := scanInvitation(row)

	if inv != nil {
		t.Error("expected nil invitation on scan error")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

// ── 12. getByID ───────────────────────────────────────────────────────────────

// 12.1 — Returns the invitation whose ID matches the query.
func TestGetByID_KnownID(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	invID := insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hashToken("tok"), "VIEWER", models.StatusPending, time.Now().Add(24*time.Hour))

	inv, err := b.svc.getByID(invID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.ID != invID {
		t.Errorf("ID: got %q, want %q", inv.ID, invID)
	}
}

// 12.2 — Returns a non-nil error for an ID not in the database.
func TestGetByID_UnknownID(t *testing.T) {
	b := newTestBundle(t)

	inv, err := b.svc.getByID("nonexistent-uuid")

	if inv != nil {
		t.Error("expected nil invitation for unknown ID")
	}
	if err == nil {
		t.Error("expected error for unknown ID")
	}
}

// ── 13. getByTokenHash ────────────────────────────────────────────────────────

// 13.1 — Returns the invitation whose token_hash column matches the supplied hash.
func TestGetByTokenHash_MatchingHash(t *testing.T) {
	b := newTestBundle(t)
	ownerID := insertUser(t, b.db, "owner@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip", "Paris", ownerID)
	hash := hashToken("hashtest")
	insertInvitation(t, b.db, tripID, ownerID, "invitee@example.com", hash, "VIEWER", models.StatusPending, time.Now().Add(24*time.Hour))

	inv, err := b.svc.getByTokenHash(hash)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.TokenHash != hash {
		t.Errorf("TokenHash: got %q, want %q", inv.TokenHash, hash)
	}
}

// 13.2 — Returns a non-nil error when no row matches the hash.
func TestGetByTokenHash_NoMatch(t *testing.T) {
	b := newTestBundle(t)

	inv, err := b.svc.getByTokenHash("nonexistenthash")

	if inv != nil {
		t.Error("expected nil invitation for non-existent hash")
	}
	if err == nil {
		t.Error("expected error for non-existent hash")
	}
}
