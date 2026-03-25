package models

import "time"

// Role represents a collaborator's permission level.
type Role string

const (
	RoleOwner  Role = "OWNER"
	RoleEditor Role = "EDITOR"
	RoleViewer Role = "VIEWER"
)

// InvitationStatus lifecycle.
type InvitationStatus string

const (
	StatusPending  InvitationStatus = "PENDING"
	StatusAccepted InvitationStatus = "ACCEPTED"
	StatusExpired  InvitationStatus = "EXPIRED"
	StatusRevoked  InvitationStatus = "REVOKED"
)

// User is a registered account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Trip is a collaborative travel plan.
type Trip struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Destination string    `json:"destination"`
	InviteCode  string    `json:"inviteCode"`
	OwnerID     string    `json:"ownerId"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TripCollaborator joins a User to a Trip with a role.
type TripCollaborator struct {
	ID       string    `json:"id"`
	TripID   string    `json:"tripId"`
	UserID   string    `json:"userId"`
	Role     Role      `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

// Invitation is a pending email invite to a trip.
type Invitation struct {
	ID           string           `json:"id"`
	TripID       string           `json:"tripId"`
	InviterID    string           `json:"inviterId"`
	InviteeEmail string           `json:"inviteeEmail"`
	TokenHash    string           `json:"-"`
	Role         Role             `json:"role"`
	Status       InvitationStatus `json:"status"`
	ExpiresAt    time.Time        `json:"expiresAt"`
	CreatedAt    time.Time        `json:"createdAt"`
}

// POI is a point of interest (restaurant, landmark, hotel, attraction).
type POI struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Address     string  `json:"address"`
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"reviewCount"`
	Description string  `json:"description"`
	ImageURL    string  `json:"imageUrl"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	PriceLevel  int     `json:"priceLevel"`
}

// ItineraryItem is a POI added to a trip's shared itinerary.
type ItineraryItem struct {
	ID            string    `json:"id"`
	TripID        string    `json:"tripId"`
	POI           *POI      `json:"poi"`
	AddedByUserID string    `json:"addedByUserId"`
	AddedByName   string    `json:"addedBy"`
	Day           int       `json:"day"`
	Notes         string    `json:"notes,omitempty"`
	Position      int       `json:"position"`
	CreatedAt     time.Time `json:"createdAt"`
}

// CollaboratorView is a read-only projection of a collaborator with online status.
// Role is returned in frontend-friendly capitalised form (Owner/Editor/Viewer).
type CollaboratorView struct {
	ID          string    `json:"id"` // userId
	DisplayName string    `json:"name"`
	Email       string    `json:"email"`
	Role        string    `json:"role"` // "Owner" | "Editor" | "Viewer"
	IsOnline    bool      `json:"isOnline"`
	JoinedAt    time.Time `json:"joinedAt"`
}

// TripDetail is the full trip response including collaborators and share link.
type TripDetail struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Destination   string             `json:"destination"`
	ShareLink     string             `json:"shareLink"`
	Collaborators []CollaboratorView `json:"collaborators"`
}

// WSHub abstracts the WebSocket hub so services can broadcast events
// without a direct package dependency on the websocket layer.
type WSHub interface {
	BroadcastToTrip(tripID, event string, data interface{})
	GetOnlineUsersInTrip(tripID string) []string
}

// FormatRole converts a DB role string to the capitalised form the frontend expects.
func FormatRole(r Role) string {
	switch r {
	case RoleOwner:
		return "Owner"
	case RoleEditor:
		return "Editor"
	case RoleViewer:
		return "Viewer"
	}
	return string(r)
}

// ParseRole converts a frontend role string to the DB Role constant.
func ParseRole(s string) (Role, bool) {
	switch s {
	case "Owner", "OWNER":
		return RoleOwner, true
	case "Editor", "EDITOR":
		return RoleEditor, true
	case "Viewer", "VIEWER":
		return RoleViewer, true
	}
	return "", false
}
