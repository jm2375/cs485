package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"cs485/internal/models"

	"github.com/google/uuid"
)

var (
	ErrForbidden          = errors.New("insufficient permissions")
	ErrCollaboratorNotFound = errors.New("collaborator not found")
	ErrCannotRemoveOwner  = errors.New("the trip owner cannot be removed")
)

// CollaboratorService manages trip membership and RBAC enforcement.
// It satisfies the spec's requirement to expose isCollaborator and hasPermission
// for use by other modules.
type CollaboratorService struct {
	db  *sql.DB
	hub models.WSHub
}

func NewCollaboratorService(db *sql.DB, hub models.WSHub) *CollaboratorService {
	return &CollaboratorService{db: db, hub: hub}
}

// AddCollaborator inserts a TripCollaborator record for the given user.
func (s *CollaboratorService) AddCollaborator(tripID, userID string, role models.Role) (*models.TripCollaborator, error) {
	id := uuid.New().String()
	now := fmtTime(time.Now())
	if _, err := s.db.Exec(
		`INSERT INTO trip_collaborators (id, trip_id, user_id, role, joined_at) VALUES (?, ?, ?, ?, ?)`,
		id, tripID, userID, string(role), now,
	); err != nil {
		return nil, fmt.Errorf("add collaborator: %w", err)
	}
	return &models.TripCollaborator{
		ID: id, TripID: tripID, UserID: userID, Role: role, JoinedAt: parseTime(now),
	}, nil
}

// RemoveCollaborator removes a collaborator. The requester must be the owner,
// or the requester is removing themselves.
func (s *CollaboratorService) RemoveCollaborator(tripID, targetUserID, requesterID string) error {
	if targetUserID != requesterID {
		if err := s.assertRole(tripID, requesterID, models.RoleOwner); err != nil {
			return ErrForbidden
		}
	}
	// Owners cannot be removed.
	var role string
	if err := s.db.QueryRow(
		`SELECT role FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`, tripID, targetUserID,
	).Scan(&role); err != nil {
		return ErrCollaboratorNotFound
	}
	if role == string(models.RoleOwner) {
		return ErrCannotRemoveOwner
	}

	if _, err := s.db.Exec(
		`DELETE FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`, tripID, targetUserID,
	); err != nil {
		return err
	}
	s.hub.BroadcastToTrip(tripID, "collaborator_left", map[string]interface{}{
		"userId": targetUserID,
	})
	return nil
}

// UpdateRole changes the role of an existing collaborator.
// Only the trip owner may do this, and the owner's own role cannot be changed.
func (s *CollaboratorService) UpdateRole(tripID, targetUserID string, newRole models.Role, requesterID string) (*models.TripCollaborator, error) {
	if err := s.assertRole(tripID, requesterID, models.RoleOwner); err != nil {
		return nil, ErrForbidden
	}
	if targetUserID == requesterID {
		return nil, errors.New("owner cannot change their own role")
	}
	if _, err := s.db.Exec(
		`UPDATE trip_collaborators SET role = ? WHERE trip_id = ? AND user_id = ?`,
		string(newRole), tripID, targetUserID,
	); err != nil {
		return nil, err
	}
	s.hub.BroadcastToTrip(tripID, "role_updated", map[string]interface{}{
		"userId":  targetUserID,
		"newRole": models.FormatRole(newRole),
	})
	return s.getCollaborator(tripID, targetUserID)
}

// GetCollaborators lists all collaborators for a trip with their online status.
func (s *CollaboratorService) GetCollaborators(tripID string) ([]models.CollaboratorView, error) {
	rows, err := s.db.Query(
		`SELECT tc.user_id, u.display_name, u.email, tc.role, tc.joined_at
		 FROM trip_collaborators tc
		 JOIN users u ON u.id = tc.user_id
		 WHERE tc.trip_id = ?
		 ORDER BY tc.joined_at ASC`,
		tripID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	onlineIDs := make(map[string]bool)
	for _, id := range s.hub.GetOnlineUsersInTrip(tripID) {
		onlineIDs[id] = true
	}

	var result []models.CollaboratorView
	for rows.Next() {
		var cv models.CollaboratorView
		var role, joinedAt string
		if err := rows.Scan(&cv.ID, &cv.DisplayName, &cv.Email, &role, &joinedAt); err != nil {
			return nil, err
		}
		cv.Role = models.FormatRole(models.Role(role))
		cv.JoinedAt = parseTime(joinedAt)
		cv.IsOnline = onlineIDs[cv.ID]
		result = append(result, cv)
	}
	return result, rows.Err()
}

// IsCollaborator returns true if userID is a member of tripID.
func (s *CollaboratorService) IsCollaborator(tripID, userID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`, tripID, userID,
	).Scan(&count)
	return count > 0, err
}

// HasPermission checks whether userID may perform action on tripID.
// Actions: "read", "invite", "write", "admin".
func (s *CollaboratorService) HasPermission(tripID, userID, action string) (bool, error) {
	var role string
	err := s.db.QueryRow(
		`SELECT role FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`, tripID, userID,
	).Scan(&role)
	if err != nil {
		return false, nil
	}
	switch action {
	case "read":
		return true, nil
	case "invite", "write":
		return role == "OWNER" || role == "EDITOR", nil
	case "admin":
		return role == "OWNER", nil
	}
	return false, nil
}

// assertRole aborts with ErrForbidden if userID does not have at least minimumRole.
func (s *CollaboratorService) assertRole(tripID, userID string, minimumRole models.Role) error {
	var role string
	if err := s.db.QueryRow(
		`SELECT role FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`, tripID, userID,
	).Scan(&role); err != nil {
		return ErrForbidden
	}
	order := map[models.Role]int{models.RoleViewer: 0, models.RoleEditor: 1, models.RoleOwner: 2}
	if order[models.Role(role)] < order[minimumRole] {
		return ErrForbidden
	}
	return nil
}

func (s *CollaboratorService) getCollaborator(tripID, userID string) (*models.TripCollaborator, error) {
	var tc models.TripCollaborator
	var role, joinedAt string
	err := s.db.QueryRow(
		`SELECT id, trip_id, user_id, role, joined_at FROM trip_collaborators WHERE trip_id = ? AND user_id = ?`,
		tripID, userID,
	).Scan(&tc.ID, &tc.TripID, &tc.UserID, &role, &joinedAt)
	if err != nil {
		return nil, ErrCollaboratorNotFound
	}
	tc.Role = models.Role(role)
	tc.JoinedAt = parseTime(joinedAt)
	return &tc, nil
}
