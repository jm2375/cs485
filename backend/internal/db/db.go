package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "github.com/lib/pq"
)

// Connect opens a PostgreSQL database at dsn, configures the connection pool,
// and verifies connectivity.
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// Migrate runs the DDL statements to create all tables and indexes.
func Migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    display_name  TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS trips (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    destination  TEXT NOT NULL DEFAULT '',
    invite_code  TEXT UNIQUE NOT NULL,
    owner_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS trip_collaborators (
    id        TEXT PRIMARY KEY,
    trip_id   TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role      TEXT NOT NULL DEFAULT 'EDITOR',
    joined_at TEXT NOT NULL,
    UNIQUE(trip_id, user_id)
);

CREATE TABLE IF NOT EXISTS invitations (
    id            TEXT PRIMARY KEY,
    trip_id       TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    inviter_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_email TEXT NOT NULL,
    token_hash    TEXT UNIQUE NOT NULL,
    role          TEXT NOT NULL DEFAULT 'EDITOR',
    status        TEXT NOT NULL DEFAULT 'PENDING',
    expires_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS points_of_interest (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    category     TEXT NOT NULL,
    subcategory  TEXT NOT NULL,
    address      TEXT NOT NULL,
    rating       REAL NOT NULL,
    review_count INTEGER NOT NULL,
    description  TEXT NOT NULL,
    image_url    TEXT NOT NULL,
    lat          REAL NOT NULL,
    lng          REAL NOT NULL,
    price_level  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS itinerary_items (
    id                TEXT PRIMARY KEY,
    trip_id           TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    poi_id            TEXT NOT NULL REFERENCES points_of_interest(id),
    added_by_user_id  TEXT NOT NULL REFERENCES users(id),
    added_by_name     TEXT NOT NULL,
    day               INTEGER NOT NULL,
    notes             TEXT NOT NULL DEFAULT '',
    position          INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    UNIQUE(trip_id, poi_id)
);

CREATE INDEX IF NOT EXISTS idx_inv_trip_status   ON invitations(trip_id, status);
CREATE INDEX IF NOT EXISTS idx_inv_email_status  ON invitations(invitee_email, status);
CREATE INDEX IF NOT EXISTS idx_collab_trip       ON trip_collaborators(trip_id);
CREATE INDEX IF NOT EXISTS idx_itinerary_trip    ON itinerary_items(trip_id);
CREATE INDEX IF NOT EXISTS idx_poi_category      ON points_of_interest(category);
`

// SeedResult holds the IDs returned by the one-time demo data setup.
type SeedResult struct {
	TripID      string
	OwnerUserID string
	OwnerEmail  string
}

// Seed creates demo users, a trip, collaborators, POIs, and itinerary items
// the first time the database is empty. On subsequent starts it returns the
// existing IDs without touching any data.
func Seed(db *sql.DB) (*SeedResult, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = 'sarah.chen@example.com'`).Scan(&count); err != nil {
		return nil, err
	}

	if count > 0 {
		var ownerID, tripID string
		if err := db.QueryRow(`SELECT id FROM users WHERE email = 'sarah.chen@example.com'`).Scan(&ownerID); err != nil {
			return nil, err
		}
		if err := db.QueryRow(
			`SELECT trip_id FROM trip_collaborators WHERE user_id = $1 AND role = 'OWNER' LIMIT 1`, ownerID,
		).Scan(&tripID); err != nil {
			return nil, err
		}
		return &SeedResult{TripID: tripID, OwnerUserID: ownerID, OwnerEmail: "sarah.chen@example.com"}, nil
	}

	now := fmtTime(time.Now())

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	hashStr := string(hash)

	type demoUser struct {
		id, email, displayName, role string
	}
	users := []demoUser{
		{uuid.New().String(), "sarah.chen@example.com", "Sarah Chen", "OWNER"},
		{uuid.New().String(), "david.lee@example.com", "David Lee", "EDITOR"},
		{uuid.New().String(), "emily.sato@example.com", "Emily Sato", "EDITOR"},
		{uuid.New().String(), "kenji.tanaka@example.com", "Kenji Tanaka", "VIEWER"},
		{uuid.New().String(), "maria.r@example.com", "Maria Rodriguez", "VIEWER"},
	}
	for _, u := range users {
		if _, err := db.Exec(
			`INSERT INTO users (id, email, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
			u.id, u.email, u.displayName, hashStr, now,
		); err != nil {
			return nil, fmt.Errorf("insert user %s: %w", u.email, err)
		}
	}

	tripID := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO trips (id, name, destination, invite_code, owner_id, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		tripID, "Tokyo Trip 2024", "Tokyo, Japan", "tok-2024-abc123", users[0].id, now,
	); err != nil {
		return nil, fmt.Errorf("insert trip: %w", err)
	}

	for _, u := range users {
		if _, err := db.Exec(
			`INSERT INTO trip_collaborators (id, trip_id, user_id, role, joined_at) VALUES ($1, $2, $3, $4, $5)`,
			uuid.New().String(), tripID, u.id, u.role, now,
		); err != nil {
			return nil, fmt.Errorf("insert collaborator %s: %w", u.email, err)
		}
	}

	if err := seedPOIs(db); err != nil {
		return nil, fmt.Errorf("seed pois: %w", err)
	}

	type demoItem struct {
		poiID  string
		userID string
		name   string
		day    int
	}
	items := []demoItem{
		{"l1", users[0].id, "Sarah Chen", 1},
		{"l4", users[1].id, "David Lee", 1},
		{"r3", users[0].id, "Sarah Chen", 1},
		{"l2", users[2].id, "Emily Sato", 2},
		{"r1", users[0].id, "Sarah Chen", 2},
	}
	for i, item := range items {
		if _, err := db.Exec(
			`INSERT INTO itinerary_items (id, trip_id, poi_id, added_by_user_id, added_by_name, day, position, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			uuid.New().String(), tripID, item.poiID, item.userID, item.name, item.day, i, now,
		); err != nil {
			return nil, fmt.Errorf("insert itinerary item %d: %w", i, err)
		}
	}

	log.Println("[db] demo seed complete")
	return &SeedResult{TripID: tripID, OwnerUserID: users[0].id, OwnerEmail: users[0].email}, nil
}

func seedPOIs(db *sql.DB) error {
	type poi struct {
		id, name, category, subcategory, address, description, imageURL string
		rating                                                           float64
		reviewCount, priceLevel                                          int
		lat, lng                                                         float64
	}
	pois := []poi{
		{"r1", "Sukiyabashi Jiro Honten", "restaurant", "Sushi · Omakase", "Chuo City, Ginza, Tokyo", "World-famous omakase sushi bar featured in Jiro Dreams of Sushi.", "https://picsum.photos/seed/jiro-sushi/200/140", 4.9, 1240, 4, 35.6714, 139.7659},
		{"r2", "Narisawa", "restaurant", "Japanese-French · Fine Dining", "Minami-Aoyama, Minato, Tokyo", "Two-Michelin-star restaurant blending French technique with Japanese ingredients.", "https://picsum.photos/seed/narisawa-tokyo/200/140", 4.8, 892, 4, 35.6667, 139.7242},
		{"r3", "Ichiran Ramen Shibuya", "restaurant", "Ramen · Solo Dining", "Shibuya, Tokyo", "Iconic solo-booth ramen chain serving rich Hakata-style tonkotsu broth.", "https://picsum.photos/seed/ichiran-ramen/200/140", 4.5, 3200, 2, 35.6615, 139.7035},
		{"r4", "Gonpachi Nishi-Azabu", "restaurant", "Izakaya · Japanese Grill", "Nishi-azabu, Minato, Tokyo", `Traditional multi-floor izakaya famous as the "Kill Bill restaurant".`, "https://picsum.photos/seed/gonpachi-tokyo/200/140", 4.4, 2100, 3, 35.6596, 139.7267},
		{"r5", "Tsuta Japanese Soba Noodles", "restaurant", "Ramen · Michelin Star", "Sugamo, Toshima, Tokyo", "World's first Michelin-starred ramen restaurant with truffle-infused broth.", "https://picsum.photos/seed/tsuta-ramen/200/140", 4.7, 1850, 2, 35.7335, 139.7394},
		{"r6", "Hanamaru Udon Shinjuku", "restaurant", "Udon · Casual", "Shinjuku, Tokyo", "Affordable self-serve udon chain beloved for fresh, chewy noodles.", "https://picsum.photos/seed/hanamaru-udon/200/140", 4.2, 5400, 1, 35.6904, 139.6995},
		{"l1", "Senso-ji Temple", "landmark", "Buddhist Temple · Historic", "2-3-1 Asakusa, Taito, Tokyo", "Tokyo's oldest and most iconic Buddhist temple, founded in 645 AD.", "https://picsum.photos/seed/sensoji-temple/200/140", 4.8, 45200, 1, 35.7148, 139.7967},
		{"l2", "Tokyo Skytree", "landmark", "Observation Tower", "1-1-2 Oshiage, Sumida, Tokyo", "World's second tallest structure with panoramic observation decks at 350 m and 450 m.", "https://picsum.photos/seed/tokyo-skytree/200/140", 4.6, 38900, 2, 35.7101, 139.8107},
		{"l3", "Meiji Shrine", "landmark", "Shinto Shrine · Forest Walk", "Yoyogi, Shibuya, Tokyo", "Serene Shinto shrine dedicated to Emperor Meiji, set within 70 hectares of forested grounds.", "https://picsum.photos/seed/meiji-shrine/200/140", 4.7, 29400, 1, 35.6763, 139.6993},
		{"l4", "Shibuya Crossing", "landmark", "Iconic Intersection", "Shibuya, Tokyo", "World's busiest pedestrian crossing — up to 3,000 people cross every light cycle.", "https://picsum.photos/seed/shibuya-crossing/200/140", 4.5, 52000, 1, 35.6594, 139.7005},
		{"l5", "Imperial Palace East Gardens", "landmark", "Garden · Historic Palace", "Chiyoda, Tokyo", "Beautifully landscaped gardens on the grounds of the Imperial Palace, free to enter.", "https://picsum.photos/seed/imperial-palace/200/140", 4.6, 18700, 1, 35.6852, 139.7528},
		{"h1", "Park Hyatt Tokyo", "hotel", "Luxury · 5-Star", "3-7-1-2 Nishi-Shinjuku, Shinjuku, Tokyo", "Iconic luxury hotel on floors 39–52 of Shinjuku Park Tower, as seen in Lost in Translation.", "https://picsum.photos/seed/park-hyatt-tokyo/200/140", 4.8, 4200, 4, 35.6861, 139.6922},
		{"h2", "Aman Tokyo", "hotel", "Ultra-Luxury · 5-Star", "Otemachi Tower, 1-5-6 Otemachi, Chiyoda, Tokyo", "Urban sanctuary occupying the top six floors of the Otemachi Tower with Japanese minimalist design.", "https://picsum.photos/seed/aman-tokyo/200/140", 4.9, 1890, 4, 35.6864, 139.7633},
		{"h3", "The Ritz-Carlton Tokyo", "hotel", "Luxury · 5-Star", "Tokyo Midtown, 9-7-1 Akasaka, Minato, Tokyo", "Sophisticated hotel occupying floors 45–53 of Tokyo Midtown Tower with skyline views.", "https://picsum.photos/seed/ritz-carlton-tokyo/200/140", 4.7, 3100, 4, 35.6652, 139.7311},
		{"h4", "Shinjuku Granbell Hotel", "hotel", "Boutique · 4-Star", "2-14-5 Kabukicho, Shinjuku, Tokyo", "Stylish boutique hotel steps from Shinjuku entertainment district.", "https://picsum.photos/seed/granbell-shinjuku/200/140", 4.4, 2800, 3, 35.6952, 139.7044},
		{"a1", "teamLab Borderless", "attraction", "Digital Art Museum", "1-3-8 Ariake, Koto, Tokyo", "Immersive borderless world of art where works move beyond rooms and boundaries.", "https://picsum.photos/seed/teamlab-borderless/200/140", 4.9, 28000, 2, 35.6286, 139.7930},
		{"a2", "Akihabara Electric Town", "attraction", "Shopping · Anime & Gaming", "Akihabara, Taito, Tokyo", "Global hub for anime, manga, electronics, and gaming culture.", "https://picsum.photos/seed/akihabara-tokyo/200/140", 4.5, 42000, 2, 35.7023, 139.7745},
	}
	for _, p := range pois {
		if _, err := db.Exec(
			`INSERT INTO points_of_interest
			 (id, name, category, subcategory, address, rating, review_count, description, image_url, lat, lng, price_level)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			 ON CONFLICT (id) DO NOTHING`,
			p.id, p.name, p.category, p.subcategory, p.address,
			p.rating, p.reviewCount, p.description, p.imageURL, p.lat, p.lng, p.priceLevel,
		); err != nil {
			return fmt.Errorf("insert poi %s: %w", p.id, err)
		}
	}
	return nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
