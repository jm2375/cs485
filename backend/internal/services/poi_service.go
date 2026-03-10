package services

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"cs485/internal/models"
)

// POIService handles searching and fetching points of interest.
// When a GooglePlacesClient is provided, live results are fetched from Google
// Places and cached into SQLite. Without a client the service falls back to the
// local database only (seeded demo data or previously cached results).
type POIService struct {
	db     *sql.DB
	google *GooglePlacesClient // nil → local-only mode
}

func NewPOIService(db *sql.DB, google *GooglePlacesClient) *POIService {
	return &POIService{db: db, google: google}
}

// Search returns POIs matching the optional text query, category, and location.
// When Google Places is configured the live API is used and results are cached.
func (s *POIService) Search(query, category, near string) ([]*models.POI, error) {
	if s.google != nil {
		pois, err := s.google.Search(query, category, near)
		if err != nil {
			log.Printf("[poi] google places error, falling back to local: %v", err)
		} else {
			s.cacheAll(pois)
			return pois, nil
		}
	}
	return s.searchLocal(query, category)
}

// GetByID fetches a single POI by ID from the local cache.
func (s *POIService) GetByID(id string) (*models.POI, error) {
	p := &models.POI{}
	err := s.db.QueryRow(
		`SELECT id, name, category, subcategory, address, rating, review_count,
		        description, image_url, lat, lng, price_level
		 FROM points_of_interest WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Category, &p.Subcategory, &p.Address,
		&p.Rating, &p.ReviewCount, &p.Description, &p.ImageURL, &p.Lat, &p.Lng, &p.PriceLevel)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ── Private helpers ───────────────────────────────────────────────────────────

// searchLocal queries only the local SQLite database.
func (s *POIService) searchLocal(query, category string) ([]*models.POI, error) {
	var conditions []string
	var args []interface{}

	if category != "" && category != "all" {
		conditions = append(conditions, "category = ?")
		args = append(args, category)
	}
	if query != "" {
		q := "%" + strings.ToLower(query) + "%"
		conditions = append(conditions,
			"(LOWER(name) LIKE ? OR LOWER(subcategory) LIKE ? OR LOWER(address) LIKE ? OR LOWER(description) LIKE ?)")
		args = append(args, q, q, q, q)
	}

	sqlStr := `SELECT id, name, category, subcategory, address, rating, review_count,
	                  description, image_url, lat, lng, price_level
	           FROM points_of_interest`
	if len(conditions) > 0 {
		sqlStr += " WHERE " + strings.Join(conditions, " AND ")
	}
	sqlStr += " ORDER BY rating DESC"

	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("poi search: %w", err)
	}
	defer rows.Close()

	var pois []*models.POI
	for rows.Next() {
		p := &models.POI{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Category, &p.Subcategory, &p.Address,
			&p.Rating, &p.ReviewCount, &p.Description, &p.ImageURL, &p.Lat, &p.Lng, &p.PriceLevel); err != nil {
			return nil, err
		}
		pois = append(pois, p)
	}
	return pois, rows.Err()
}

// cacheAll upserts a slice of POIs into the local database so they can be
// referenced by ID when a user adds one to an itinerary.
func (s *POIService) cacheAll(pois []*models.POI) {
	for _, p := range pois {
		if _, err := s.db.Exec(
			`INSERT OR REPLACE INTO points_of_interest
			 (id, name, category, subcategory, address, rating, review_count, description, image_url, lat, lng, price_level)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.Category, p.Subcategory, p.Address,
			p.Rating, p.ReviewCount, p.Description, p.ImageURL, p.Lat, p.Lng, p.PriceLevel,
		); err != nil {
			log.Printf("[poi] cache write for %s: %v", p.ID, err)
		}
	}
}
