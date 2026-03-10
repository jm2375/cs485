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
	ErrAlreadyOnItinerary = errors.New("this place is already on the itinerary")
	ErrItemNotFound       = errors.New("itinerary item not found")
)

// ItineraryService manages a trip's shared itinerary and broadcasts real-time updates.
type ItineraryService struct {
	db  *sql.DB
	hub models.WSHub
}

func NewItineraryService(db *sql.DB, _ *POIService, hub models.WSHub) *ItineraryService {
	return &ItineraryService{db: db, hub: hub}
}

// GetItinerary returns all itinerary items for a trip, sorted by day then position.
func (s *ItineraryService) GetItinerary(tripID string) ([]*models.ItineraryItem, error) {
	rows, err := s.db.Query(
		`SELECT ii.id, ii.trip_id, ii.added_by_user_id, ii.added_by_name, ii.day, ii.notes, ii.position, ii.created_at,
		        p.id, p.name, p.category, p.subcategory, p.address, p.rating, p.review_count,
		        p.description, p.image_url, p.lat, p.lng, p.price_level
		 FROM itinerary_items ii
		 JOIN points_of_interest p ON p.id = ii.poi_id
		 WHERE ii.trip_id = ?
		 ORDER BY ii.day ASC, ii.position ASC`, tripID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*models.ItineraryItem
	for rows.Next() {
		item, err := scanItemWithPOI(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// AddPOI adds a point of interest to the trip itinerary.
// Returns ErrAlreadyOnItinerary if the POI is already present on this trip.
func (s *ItineraryService) AddPOI(tripID, poiID, userID string, day int, notes string) (*models.ItineraryItem, error) {
	// Duplicate check (enforced by UNIQUE constraint + early check for a better error).
	var existing int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM itinerary_items WHERE trip_id = ? AND poi_id = ?`, tripID, poiID,
	).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrAlreadyOnItinerary
	}

	var position int
	s.db.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM itinerary_items WHERE trip_id = ? AND day = ?`,
		tripID, day,
	).Scan(&position)

	// Fetch user name.
	var addedByName string
	s.db.QueryRow(`SELECT display_name FROM users WHERE id = ?`, userID).Scan(&addedByName)

	id := uuid.New().String()
	now := fmtTime(time.Now())
	if _, err := s.db.Exec(
		`INSERT INTO itinerary_items (id, trip_id, poi_id, added_by_user_id, added_by_name, day, notes, position, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, tripID, poiID, userID, addedByName, day, notes, position, now,
	); err != nil {
		return nil, fmt.Errorf("insert itinerary item: %w", err)
	}

	item, err := s.getByID(id)
	if err != nil {
		return nil, err
	}

	s.hub.BroadcastToTrip(tripID, "itinerary_updated", map[string]interface{}{
		"action": "added",
		"item":   item,
	})
	return item, nil
}

// RemoveItem deletes an itinerary item. Any trip collaborator with write permission may remove items.
func (s *ItineraryService) RemoveItem(tripID, itemID string) error {
	res, err := s.db.Exec(
		`DELETE FROM itinerary_items WHERE id = ? AND trip_id = ?`, itemID, tripID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrItemNotFound
	}
	s.hub.BroadcastToTrip(tripID, "itinerary_updated", map[string]interface{}{
		"action": "removed",
		"itemId": itemID,
	})
	return nil
}

// ── Private helpers ──────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanItemWithPOI scans a row that was produced by a JOIN between itinerary_items and points_of_interest.
// This avoids the N+1 query deadlock that occurs when GetByID is called while a rows cursor is open
// on the single SQLite connection.
func scanItemWithPOI(row rowScanner) (*models.ItineraryItem, error) {
	var item models.ItineraryItem
	var poi models.POI
	var createdAt string
	if err := row.Scan(
		&item.ID, &item.TripID, &item.AddedByUserID, &item.AddedByName,
		&item.Day, &item.Notes, &item.Position, &createdAt,
		&poi.ID, &poi.Name, &poi.Category, &poi.Subcategory, &poi.Address,
		&poi.Rating, &poi.ReviewCount, &poi.Description, &poi.ImageURL,
		&poi.Lat, &poi.Lng, &poi.PriceLevel,
	); err != nil {
		return nil, err
	}
	item.CreatedAt = parseTime(createdAt)
	item.POI = &poi
	return &item, nil
}

func (s *ItineraryService) getByID(id string) (*models.ItineraryItem, error) {
	row := s.db.QueryRow(
		`SELECT ii.id, ii.trip_id, ii.added_by_user_id, ii.added_by_name, ii.day, ii.notes, ii.position, ii.created_at,
		        p.id, p.name, p.category, p.subcategory, p.address, p.rating, p.review_count,
		        p.description, p.image_url, p.lat, p.lng, p.price_level
		 FROM itinerary_items ii
		 JOIN points_of_interest p ON p.id = ii.poi_id
		 WHERE ii.id = ?`, id,
	)
	return scanItemWithPOI(row)
}
