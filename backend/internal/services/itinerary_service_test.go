package services

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	appdb "cs485/internal/db"

	"github.com/google/uuid"
)

// ── Test infrastructure ───────────────────────────────────────────────────────

type itineraryBundle struct {
	svc *ItineraryService
	hub *captureHub
	db  *sql.DB
}

func newItineraryBundle(t *testing.T) *itineraryBundle {
	t.Helper()
	dsn := fmt.Sprintf("file:itntest%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := appdb.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	hub := &captureHub{}
	svc := NewItineraryService(db, nil, hub)
	return &itineraryBundle{svc, hub, db}
}

func insertPOI(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO points_of_interest
		 (id, name, category, subcategory, address, rating, review_count, description, image_url, lat, lng, price_level)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		id, "Test POI", "restaurant", "Italian", "123 Test St",
		4.5, 100, "A test POI", "https://example.com/img.jpg", 35.0, 139.0, 2,
	); err != nil {
		t.Fatalf("insertPOI: %v", err)
	}
	return id
}

func insertItineraryItemDirect(t *testing.T, db *sql.DB, tripID, poiID, userID string, day, position int) string {
	t.Helper()
	id := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO itinerary_items (id, trip_id, poi_id, added_by_user_id, added_by_name, day, notes, position, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, tripID, poiID, userID, "Test User", day, "", position, fmtTime(time.Now()),
	); err != nil {
		t.Fatalf("insertItineraryItemDirect: %v", err)
	}
	return id
}

// valueScanner is a mock rowScanner used to test scanItemWithPOI directly.
// It assigns values positionally; types must match the destination pointers exactly.
type valueScanner struct {
	values []interface{}
}

func (v *valueScanner) Scan(dest ...interface{}) error {
	if len(dest) != len(v.values) {
		return fmt.Errorf("column count mismatch: have %d values, got %d destinations", len(v.values), len(dest))
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *string:
			*ptr = v.values[i].(string)
		case *int:
			*ptr = v.values[i].(int)
		case *float64:
			*ptr = v.values[i].(float64)
		}
	}
	return nil
}

// ── 1. NewItineraryService ────────────────────────────────────────────────────

// 1.1 — Returns a correctly initialised service with db and hub wired.
func TestNewItineraryService_WiresAllFields(t *testing.T) {
	dsn := fmt.Sprintf("file:itnwire%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, _ := sql.Open("sqlite", dsn)
	t.Cleanup(func() { db.Close() })
	hub := &captureHub{}

	svc := NewItineraryService(db, nil, hub)

	if svc == nil {
		t.Fatal("expected non-nil *ItineraryService")
	}
	if svc.db != db {
		t.Error("db field not wired correctly")
	}
	if svc.hub != hub {
		t.Error("hub field not wired correctly")
	}
}

// ── 2. GetItinerary ───────────────────────────────────────────────────────────

// 2.1 — Returns all items sorted by day ASC then position ASC.
func TestGetItinerary_ReturnsSortedItems(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-git@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip A", "Paris", ownerID)
	poi1 := insertPOI(t, b.db)
	poi2 := insertPOI(t, b.db)
	poi3 := insertPOI(t, b.db)

	// Insert out-of-order to verify sorting.
	insertItineraryItemDirect(t, b.db, tripID, poi3, ownerID, 2, 0)
	insertItineraryItemDirect(t, b.db, tripID, poi1, ownerID, 1, 0)
	insertItineraryItemDirect(t, b.db, tripID, poi2, ownerID, 1, 1)

	items, err := b.svc.GetItinerary(tripID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// First two items should be day 1, last should be day 2.
	if items[0].Day != 1 || items[1].Day != 1 || items[2].Day != 2 {
		t.Errorf("wrong day order: days = %d, %d, %d", items[0].Day, items[1].Day, items[2].Day)
	}
	// Within day 1 the positions must be ascending.
	if items[0].Position > items[1].Position {
		t.Errorf("positions within day 1 not sorted: %d > %d", items[0].Position, items[1].Position)
	}
	// Every item must carry its POI.
	for i, item := range items {
		if item.POI == nil {
			t.Errorf("item[%d].POI is nil", i)
		}
	}
}

// 2.2 — Returns an empty (non-error) result for a trip that has no items.
func TestGetItinerary_EmptyTrip(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-empty@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Empty Trip", "Berlin", ownerID)

	items, err := b.svc.GetItinerary(tripID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

// 2.3 — Propagates a database error when the connection is closed.
func TestGetItinerary_DBError(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-dberr@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip DB Err", "Oslo", ownerID)

	b.db.Close() // force all subsequent queries to fail

	_, err := b.svc.GetItinerary(tripID)

	if err == nil {
		t.Error("expected an error after DB close, got nil")
	}
}

// ── 3. AddPOI ─────────────────────────────────────────────────────────────────

// 3.1 — Successfully adds a new POI; first item on a day gets position 0.
func TestAddPOI_Success_FirstItemGetsPositionZero(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-add@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip Add", "Tokyo", ownerID)
	poiID := insertPOI(t, b.db)

	item, err := b.svc.AddPOI(tripID, poiID, ownerID, 1, "test notes")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item == nil {
		t.Fatal("expected non-nil item")
	}
	if item.TripID != tripID {
		t.Errorf("TripID: got %q, want %q", item.TripID, tripID)
	}
	if item.Day != 1 {
		t.Errorf("Day: got %d, want 1", item.Day)
	}
	if item.Notes != "test notes" {
		t.Errorf("Notes: got %q, want %q", item.Notes, "test notes")
	}
	if item.Position != 0 {
		t.Errorf("Position: got %d, want 0", item.Position)
	}
	if item.POI == nil {
		t.Error("POI field is nil")
	}
}

// 3.2 — Second POI added to the same day receives the next sequential position.
func TestAddPOI_NextPositionOnSameDay(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-pos@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip Pos", "Seoul", ownerID)
	poi1 := insertPOI(t, b.db)
	poi2 := insertPOI(t, b.db)

	if _, err := b.svc.AddPOI(tripID, poi1, ownerID, 1, ""); err != nil {
		t.Fatalf("first AddPOI: %v", err)
	}
	item2, err := b.svc.AddPOI(tripID, poi2, ownerID, 1, "")
	if err != nil {
		t.Fatalf("second AddPOI: %v", err)
	}
	if item2.Position != 1 {
		t.Errorf("Position: got %d, want 1", item2.Position)
	}
}

// 3.3 — Adding the same POI twice to the same trip returns ErrAlreadyOnItinerary.
func TestAddPOI_DuplicatePOI(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-dup@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip Dup", "Rome", ownerID)
	poiID := insertPOI(t, b.db)

	if _, err := b.svc.AddPOI(tripID, poiID, ownerID, 1, ""); err != nil {
		t.Fatalf("first AddPOI: %v", err)
	}
	_, err := b.svc.AddPOI(tripID, poiID, ownerID, 1, "")

	if err != ErrAlreadyOnItinerary {
		t.Errorf("err: got %v, want ErrAlreadyOnItinerary", err)
	}
}

// 3.4 — AddedByName is populated from the user's display_name.
func TestAddPOI_AddedByNameFromUser(t *testing.T) {
	b := newItineraryBundle(t)
	userID := insertUser(t, b.db, "jane@example.com", "Jane")
	tripID := insertTrip(t, b.db, "Trip Jane", "Madrid", userID)
	poiID := insertPOI(t, b.db)

	item, err := b.svc.AddPOI(tripID, poiID, userID, 1, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.AddedByName != "Jane" {
		t.Errorf("AddedByName: got %q, want %q", item.AddedByName, "Jane")
	}
}

// 3.5 — When the user exists but has an empty display_name, AddedByName is "".
func TestAddPOI_AddedByName_EmptyDisplayName(t *testing.T) {
	b := newItineraryBundle(t)
	userID := insertUser(t, b.db, "noname@example.com", "") // empty display_name
	tripID := insertTrip(t, b.db, "Trip NoName", "Lisbon", userID)
	poiID := insertPOI(t, b.db)

	item, err := b.svc.AddPOI(tripID, poiID, userID, 1, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.AddedByName != "" {
		t.Errorf("AddedByName: got %q, want empty string", item.AddedByName)
	}
}

// 3.6 — Propagates a DB error when the POI foreign key does not exist (insert fails).
func TestAddPOI_InsertFailure_PropagatesError(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-fail@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip Fail", "Vienna", ownerID)
	nonExistentPOIID := uuid.New().String() // not in points_of_interest

	_, err := b.svc.AddPOI(tripID, nonExistentPOIID, ownerID, 1, "")

	if err == nil {
		t.Error("expected an error for non-existent POI, got nil")
	}
}

// 3.7 — A successful add broadcasts an "itinerary_updated" event with action "added".
func TestAddPOI_BroadcastsWSEvent(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-ws@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip WS", "Amsterdam", ownerID)
	poiID := insertPOI(t, b.db)

	item, err := b.svc.AddPOI(tripID, poiID, ownerID, 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b.hub.mu.Lock()
	events := b.hub.events
	b.hub.mu.Unlock()

	if len(events) != 1 {
		t.Fatalf("expected 1 broadcast event, got %d", len(events))
	}
	ev := events[0]
	if ev.tripID != tripID {
		t.Errorf("event tripID: got %q, want %q", ev.tripID, tripID)
	}
	if ev.event != "itinerary_updated" {
		t.Errorf("event type: got %q, want %q", ev.event, "itinerary_updated")
	}
	payload, ok := ev.data.(map[string]interface{})
	if !ok {
		t.Fatal("event data is not map[string]interface{}")
	}
	if payload["action"] != "added" {
		t.Errorf("payload action: got %v, want \"added\"", payload["action"])
	}
	if payload["item"] != item {
		t.Error("payload item does not match returned item")
	}
}

// ── 4. RemoveItem ─────────────────────────────────────────────────────────────

// 4.1 — Successfully deletes an item and broadcasts a "removed" event.
func TestRemoveItem_Success(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-rm@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip Rm", "Copenhagen", ownerID)
	poiID := insertPOI(t, b.db)
	itemID := insertItineraryItemDirect(t, b.db, tripID, poiID, ownerID, 1, 0)

	err := b.svc.RemoveItem(tripID, itemID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify row is gone.
	var count int
	b.db.QueryRow(`SELECT COUNT(*) FROM itinerary_items WHERE id = $1`, itemID).Scan(&count)
	if count != 0 {
		t.Error("item row still exists after RemoveItem")
	}

	// Verify broadcast.
	b.hub.mu.Lock()
	events := b.hub.events
	b.hub.mu.Unlock()

	if len(events) != 1 {
		t.Fatalf("expected 1 broadcast event, got %d", len(events))
	}
	ev := events[0]
	if ev.tripID != tripID {
		t.Errorf("event tripID: got %q, want %q", ev.tripID, tripID)
	}
	payload, ok := ev.data.(map[string]interface{})
	if !ok {
		t.Fatal("event data is not map[string]interface{}")
	}
	if payload["action"] != "removed" {
		t.Errorf("payload action: got %v, want \"removed\"", payload["action"])
	}
	if payload["itemId"] != itemID {
		t.Errorf("payload itemId: got %v, want %q", payload["itemId"], itemID)
	}
}

// 4.2 — Returns ErrItemNotFound when the item ID is not in the database.
func TestRemoveItem_ItemNotFound(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-nf@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip NF", "Athens", ownerID)

	err := b.svc.RemoveItem(tripID, uuid.New().String())

	if err != ErrItemNotFound {
		t.Errorf("err: got %v, want ErrItemNotFound", err)
	}
}

// 4.3 — Returns ErrItemNotFound when the item exists but belongs to a different trip.
func TestRemoveItem_WrongTripID(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-wt@example.com", "Owner")
	tripA := insertTrip(t, b.db, "Trip A", "Dublin", ownerID)
	tripB := insertTrip(t, b.db, "Trip B", "Prague", ownerID)
	poiID := insertPOI(t, b.db)
	itemID := insertItineraryItemDirect(t, b.db, tripA, poiID, ownerID, 1, 0)

	err := b.svc.RemoveItem(tripB, itemID) // wrong trip

	if err != ErrItemNotFound {
		t.Errorf("err: got %v, want ErrItemNotFound", err)
	}
	// Verify no broadcast was sent.
	b.hub.mu.Lock()
	evCount := len(b.hub.events)
	b.hub.mu.Unlock()
	if evCount != 0 {
		t.Errorf("expected 0 broadcast events, got %d", evCount)
	}
}

// 4.4 — Propagates a database error when the connection is closed.
func TestRemoveItem_DBError(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-rmdberr@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip RmDBErr", "Reykjavik", ownerID)

	b.db.Close()

	err := b.svc.RemoveItem(tripID, uuid.New().String())

	if err == nil {
		t.Error("expected an error after DB close, got nil")
	}
}

// 4.5 — Does not broadcast when the deletion fails.
func TestRemoveItem_NoBroadcastOnFailure(t *testing.T) {
	b := newItineraryBundle(t)
	ownerID := insertUser(t, b.db, "owner-nobcast@example.com", "Owner")
	tripID := insertTrip(t, b.db, "Trip NoBcast", "Tallinn", ownerID)

	b.db.Close()

	_ = b.svc.RemoveItem(tripID, uuid.New().String())

	b.hub.mu.Lock()
	evCount := len(b.hub.events)
	b.hub.mu.Unlock()

	if evCount != 0 {
		t.Errorf("expected 0 broadcast events after failed remove, got %d", evCount)
	}
}

// ── 5. scanItemWithPOI ────────────────────────────────────────────────────────

// 5.1 — Correctly maps all 20 columns to the ItineraryItem and embedded POI.
func TestScanItemWithPOI_MapsAllFields(t *testing.T) {
	createdAt := fmtTime(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC))
	row := &valueScanner{values: []interface{}{
		// ItineraryItem fields
		"item-id-1", "trip-id-1", "user-id-1", "Alice",
		3,          // Day
		"See it!",  // Notes
		2,          // Position
		createdAt,  // created_at (string, parsed by scanItemWithPOI)
		// POI fields
		"poi-id-1", "Eiffel Tower", "landmark", "Monument",
		"Champ de Mars, Paris",
		4.8, 99000, "Iconic iron lattice tower.", "https://example.com/eiffel.jpg",
		48.8584, 2.2945, 1,
	}}

	item, err := scanItemWithPOI(row)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != "item-id-1" {
		t.Errorf("ID: got %q, want %q", item.ID, "item-id-1")
	}
	if item.TripID != "trip-id-1" {
		t.Errorf("TripID: got %q", item.TripID)
	}
	if item.AddedByUserID != "user-id-1" {
		t.Errorf("AddedByUserID: got %q", item.AddedByUserID)
	}
	if item.AddedByName != "Alice" {
		t.Errorf("AddedByName: got %q, want %q", item.AddedByName, "Alice")
	}
	if item.Day != 3 {
		t.Errorf("Day: got %d, want 3", item.Day)
	}
	if item.Notes != "See it!" {
		t.Errorf("Notes: got %q", item.Notes)
	}
	if item.Position != 2 {
		t.Errorf("Position: got %d, want 2", item.Position)
	}
	if item.CreatedAt.IsZero() {
		t.Error("CreatedAt was not parsed (still zero)")
	}
	if item.POI == nil {
		t.Fatal("POI is nil")
	}
	if item.POI.ID != "poi-id-1" {
		t.Errorf("POI.ID: got %q", item.POI.ID)
	}
	if item.POI.Name != "Eiffel Tower" {
		t.Errorf("POI.Name: got %q", item.POI.Name)
	}
	if item.POI.Rating != 4.8 {
		t.Errorf("POI.Rating: got %f, want 4.8", item.POI.Rating)
	}
	if item.POI.Lat != 48.8584 {
		t.Errorf("POI.Lat: got %f, want 48.8584", item.POI.Lat)
	}
}

// 5.2 — Returns nil and the scan error when the underlying Scan call fails.
func TestScanItemWithPOI_ScanError(t *testing.T) {
	row := &errScanner{err: sql.ErrNoRows}

	item, err := scanItemWithPOI(row)

	if item != nil {
		t.Error("expected nil item on scan error")
	}
	if err != sql.ErrNoRows {
		t.Errorf("err: got %v, want sql.ErrNoRows", err)
	}
}
