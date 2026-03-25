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
		 WHERE ii.trip_id = $1
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
func (s *ItineraryService) AddPOI(tripID, poiID, userID string, day int, notes string) (*models.ItineraryItem, error) {
	var existing int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM itinerary_items WHERE trip_id = $1 AND poi_id = $2`, tripID, poiID,
	).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrAlreadyOnItinerary
	}

	var position int
	s.db.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM itinerary_items WHERE trip_id = $1 AND day = $2`,
		tripID, day,
	).Scan(&position)

	var addedByName string
	s.db.QueryRow(`SELECT display_name FROM users WHERE id = $1`, userID).Scan(&addedByName)

	id := uuid.New().String()
	now := fmtTime(time.Now())
	if _, err := s.db.Exec(
		`INSERT INTO itinerary_items (id, trip_id, poi_id, added_by_user_id, added_by_name, day, notes, position, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
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

// RemoveItem deletes an itinerary item.
func (s *ItineraryService) RemoveItem(tripID, itemID string) error {
	res, err := s.db.Exec(
		`DELETE FROM itinerary_items WHERE id = $1 AND trip_id = $2`, itemID, tripID,
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
		 WHERE ii.id = $1`, id,
	)
	return scanItemWithPOI(row)
}
