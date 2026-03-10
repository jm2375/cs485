package services

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cs485/internal/cache"
	"cs485/internal/models"

	"github.com/google/uuid"
)

var (
	ErrInviteRateLimited  = errors.New("rate limit exceeded: too many invitations")
	ErrInviteAlreadySent  = errors.New("an active invitation already exists for this email")
	ErrInviteNotFound     = errors.New("invitation not found")
	ErrInviteExpiredOrUsed = errors.New("invitation token is expired or already used")
	ErrNotOwner           = errors.New("only the trip owner or an editor can perform this action")
)

type inviteTokenData struct {
	InvitationID string `json:"invitationId"`
	TripID       string `json:"tripId"`
}

// InvitationService manages email invitations: generation, validation, acceptance, revocation.
type InvitationService struct {
	db           *sql.DB
	cache        *cache.Store
	emailService IEmailService
	hub          models.WSHub
	frontendURL  string
}

func NewInvitationService(
	db *sql.DB,
	c *cache.Store,
	email IEmailService,
	hub models.WSHub,
	frontendURL string,
) *InvitationService {
	return &InvitationService{
		db:          db,
		cache:       c,
		emailService: email,
		hub:         hub,
		frontendURL: frontendURL,
	}
}

// SendEmailInvite generates a secure token, stores the invitation, and dispatches
// the invite email via EmailService.
func (s *InvitationService) SendEmailInvite(tripID, inviterID, email string, role models.Role) (*models.Invitation, error) {
	if err := s.checkRateLimit(inviterID, tripID); err != nil {
		return nil, err
	}

	// Prevent duplicate pending invitations to the same address.
	var existing int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM invitations WHERE trip_id = ? AND invitee_email = ? AND status = 'PENDING'`,
		tripID, email,
	).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrInviteAlreadySent
	}

	rawToken, err := generateToken()
	if err != nil {
		return nil, err
	}
	tokenHash := hashToken(rawToken)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	id := uuid.New().String()
	now := fmtTime(time.Now())
	if _, err := s.db.Exec(
		`INSERT INTO invitations (id, trip_id, inviter_id, invitee_email, token_hash, role, status, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
		id, tripID, inviterID, email, tokenHash, string(role), fmtTime(expiresAt), now,
	); err != nil {
		return nil, fmt.Errorf("insert invitation: %w", err)
	}

	// Cache the raw token → { invitationId, tripId } for fast lookup.
	data, _ := json.Marshal(inviteTokenData{InvitationID: id, TripID: tripID})
	s.cache.Set("invite:"+tokenHash, string(data), 7*24*time.Hour)

	// Look up trip + inviter for the email body.
	var tripName, inviterName string
	s.db.QueryRow(`SELECT name FROM trips WHERE id = ?`, tripID).Scan(&tripName)
	s.db.QueryRow(`SELECT display_name FROM users WHERE id = ?`, inviterID).Scan(&inviterName)

	inviteLink := fmt.Sprintf("%s/invite/%s", s.frontendURL, rawToken)
	if err := s.emailService.SendInviteEmail(email, inviterName, tripName, inviteLink); err != nil {
		// Non-fatal — invitation is already persisted.
		fmt.Printf("[invitation] email send failed for %s: %v\n", email, err)
	}

	return s.getByID(id)
}

// ValidateToken hashes the raw token and looks up the invitation in the DB.
func (s *InvitationService) ValidateToken(rawToken string) (*models.Invitation, error) {
	hash := hashToken(rawToken)
	inv, err := s.getByTokenHash(hash)
	if err != nil {
		return nil, ErrInviteNotFound
	}
	if inv.Status != models.StatusPending || time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteExpiredOrUsed
	}
	return inv, nil
}

// AcceptInvitation converts a pending invitation into a TripCollaborator in a single transaction.
func (s *InvitationService) AcceptInvitation(rawToken, userID string) (*models.TripCollaborator, error) {
	hash := hashToken(rawToken)
	inv, err := s.getByTokenHash(hash)
	if err != nil {
		return nil, ErrInviteNotFound
	}
	if inv.Status != models.StatusPending || time.Now().After(inv.ExpiresAt) {
		return nil, ErrInviteExpiredOrUsed
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	collabID := uuid.New().String()
	now := fmtTime(time.Now())

	// Create collaborator (INSERT OR IGNORE so accepting twice is idempotent).
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO trip_collaborators (id, trip_id, user_id, role, joined_at)
		 VALUES (?, ?, ?, ?, ?)`,
		collabID, inv.TripID, userID, string(inv.Role), now,
	); err != nil {
		return nil, fmt.Errorf("insert collaborator: %w", err)
	}

	// Mark invitation as accepted.
	if _, err := tx.Exec(
		`UPDATE invitations SET status = 'ACCEPTED' WHERE id = ?`, inv.ID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Evict the token from the cache.
	s.cache.Del("invite:" + hash)

	// Broadcast to trip room.
	var displayName string
	s.db.QueryRow(`SELECT display_name FROM users WHERE id = ?`, userID).Scan(&displayName)
	s.hub.BroadcastToTrip(inv.TripID, "collaborator_joined", map[string]interface{}{
		"userId":      userID,
		"displayName": displayName,
		"role":        models.FormatRole(inv.Role),
	})

	return &models.TripCollaborator{
		ID:       collabID,
		TripID:   inv.TripID,
		UserID:   userID,
		Role:     inv.Role,
		JoinedAt: parseTime(now),
	}, nil
}

// RevokeInvitation cancels a pending invitation; only the trip owner or an editor may do this.
func (s *InvitationService) RevokeInvitation(invitationID, requesterID string) error {
	inv, err := s.getByID(invitationID)
	if err != nil {
		return ErrInviteNotFound
	}
	if inv.Status != models.StatusPending {
		return errors.New("invitation is no longer pending")
	}

	// Verify requester has at least EDITOR role.
	var role string
	if err := s.db.QueryRow(
		`SELECT role FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`,
		inv.TripID, requesterID,
	).Scan(&role); err != nil || (role != "OWNER" && role != "EDITOR") {
		return ErrNotOwner
	}

	if _, err := s.db.Exec(
		`UPDATE invitations SET status = 'REVOKED' WHERE id = ?`, invitationID,
	); err != nil {
		return err
	}
	s.cache.Del("invite:" + inv.TokenHash)
	return nil
}

// ListInvitations returns all invitations for a trip, optionally filtered by status.
func (s *InvitationService) ListInvitations(tripID string, status *models.InvitationStatus) ([]*models.Invitation, error) {
	query := `SELECT id, trip_id, inviter_id, invitee_email, token_hash, role, status, expires_at, created_at
	          FROM invitations WHERE trip_id = ?`
	args := []interface{}{tripID}
	if status != nil {
		query += ` AND status = ?`
		args = append(args, string(*status))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invs []*models.Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invs = append(invs, inv)
	}
	return invs, rows.Err()
}

// ── Private helpers ──────────────────────────────────────────────────────────

func (s *InvitationService) getByID(id string) (*models.Invitation, error) {
	row := s.db.QueryRow(
		`SELECT id, trip_id, inviter_id, invitee_email, token_hash, role, status, expires_at, created_at
		 FROM invitations WHERE id = ?`, id,
	)
	return scanInvitation(row)
}

func (s *InvitationService) getByTokenHash(hash string) (*models.Invitation, error) {
	row := s.db.QueryRow(
		`SELECT id, trip_id, inviter_id, invitee_email, token_hash, role, status, expires_at, created_at
		 FROM invitations WHERE token_hash = ?`, hash,
	)
	return scanInvitation(row)
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanInvitation(row scanner) (*models.Invitation, error) {
	var inv models.Invitation
	var role, status, expiresAt, createdAt string
	err := row.Scan(&inv.ID, &inv.TripID, &inv.InviterID, &inv.InviteeEmail,
		&inv.TokenHash, &role, &status, &expiresAt, &createdAt)
	if err != nil {
		return nil, err
	}
	inv.Role = models.Role(role)
	inv.Status = models.InvitationStatus(status)
	inv.ExpiresAt = parseTime(expiresAt)
	inv.CreatedAt = parseTime(createdAt)
	return &inv, nil
}

// checkRateLimit enforces: 20 invites per trip per hour, 50 per user per day.
func (s *InvitationService) checkRateLimit(userID, tripID string) error {
	tripKey := fmt.Sprintf("ratelimit:trip:%s:%s", tripID, userID)
	if s.cache.Incr(tripKey, time.Hour) > 20 {
		return ErrInviteRateLimited
	}
	userKey := fmt.Sprintf("ratelimit:user:%s", userID)
	if s.cache.Incr(userKey, 24*time.Hour) > 50 {
		return ErrInviteRateLimited
	}
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
