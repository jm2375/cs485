package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cs485/internal/models"
)

const (
	googleSearchURL = "https://places.googleapis.com/v1/places:searchText"
	googlePhotoURL  = "https://places.googleapis.com/v1/%s/media?maxWidthPx=800&skipHttpRedirect=true&key=%s"
	googleFieldMask = "places.id,places.displayName,places.types,places.formattedAddress,places.location,places.rating,places.userRatingCount,places.priceLevel,places.photos,places.editorialSummary"
)

// googleCategoryQuery maps our category strings to natural-language search terms.
var googleCategoryQuery = map[string]string{
	"restaurant": "restaurants",
	"hotel":      "hotels",
	"landmark":   "landmarks and historic sites",
	"attraction": "tourist attractions",
}

// googleTypeCategory maps Google Places types to our category strings.
// The first match wins, so more specific types are listed first.
var googleTypeCategory = []struct {
	googleType string
	category   string
}{
	{"restaurant", "restaurant"},
	{"cafe", "restaurant"},
	{"bakery", "restaurant"},
	{"bar", "restaurant"},
	{"meal_takeaway", "restaurant"},
	{"lodging", "hotel"},
	{"tourist_attraction", "attraction"},
	{"amusement_park", "attraction"},
	{"museum", "attraction"},
	{"art_gallery", "attraction"},
	{"zoo", "attraction"},
	{"aquarium", "attraction"},
	{"church", "landmark"},
	{"hindu_temple", "landmark"},
	{"mosque", "landmark"},
	{"synagogue", "landmark"},
	{"stadium", "landmark"},
	{"park", "landmark"},
}

var googlePriceLevel = map[string]int{
	"PRICE_LEVEL_FREE":           0,
	"PRICE_LEVEL_INEXPENSIVE":    1,
	"PRICE_LEVEL_MODERATE":       2,
	"PRICE_LEVEL_EXPENSIVE":      3,
	"PRICE_LEVEL_VERY_EXPENSIVE": 4,
}

// GooglePlacesClient calls the Google Places API (New).
type GooglePlacesClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewGooglePlacesClient(apiKey string) *GooglePlacesClient {
	return &GooglePlacesClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ── Wire types ────────────────────────────────────────────────────────────────

type googleSearchRequest struct {
	TextQuery      string `json:"textQuery"`
	MaxResultCount int    `json:"maxResultCount"`
	LanguageCode   string `json:"languageCode"`
}

type googleSearchResponse struct {
	Places []googlePlace `json:"places"`
}

type googlePlace struct {
	ID               string         `json:"id"`
	DisplayName      googleText     `json:"displayName"`
	Types            []string       `json:"types"`
	FormattedAddress string         `json:"formattedAddress"`
	Location         googleLatLng   `json:"location"`
	Rating           float64        `json:"rating"`
	UserRatingCount  int            `json:"userRatingCount"`
	PriceLevel       string         `json:"priceLevel"`
	Photos           []googlePhoto  `json:"photos"`
	EditorialSummary googleText     `json:"editorialSummary"`
}

type googleText struct {
	Text string `json:"text"`
}

type googleLatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type googlePhoto struct {
	Name string `json:"name"`
}

type googlePhotoMediaResponse struct {
	PhotoURI string `json:"photoUri"`
}

// ── Search ────────────────────────────────────────────────────────────────────

// Search queries the Google Places text search endpoint and returns normalised POIs.
// near should be a city/address string e.g. "Tokyo, Japan".
func (c *GooglePlacesClient) Search(query, category, near string) ([]*models.POI, error) {
	textQuery := buildTextQuery(query, category, near)

	body, err := json.Marshal(googleSearchRequest{
		TextQuery:      textQuery,
		MaxResultCount: 20,
		LanguageCode:   "en",
	})
	if err != nil {
		return nil, fmt.Errorf("google places: marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", googleSearchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("google places: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-FieldMask", googleFieldMask)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google places: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var e map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("google places: status %d: %v", resp.StatusCode, e)
	}

	var result googleSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("google places: decode: %w", err)
	}

	// Resolve photo URLs in parallel.
	photoURLs := c.fetchPhotoURLs(result.Places)

	pois := make([]*models.POI, 0, len(result.Places))
	for i, p := range result.Places {
		pois = append(pois, normaliseGooglePlace(p, photoURLs[i]))
	}
	return pois, nil
}

// ── Photo fetching ────────────────────────────────────────────────────────────

// fetchPhotoURLs resolves the first photo for each place in parallel.
// Returns a slice of URLs in the same order as places; empty string if no photo.
func (c *GooglePlacesClient) fetchPhotoURLs(places []googlePlace) []string {
	urls := make([]string, len(places))
	var wg sync.WaitGroup

	for i, p := range places {
		if len(p.Photos) == 0 {
			continue
		}
		wg.Add(1)
		go func(idx int, photoName string) {
			defer wg.Done()
			if url, err := c.fetchPhotoURL(photoName); err == nil {
				urls[idx] = url
			}
		}(i, p.Photos[0].Name)
	}

	wg.Wait()
	return urls
}

func (c *GooglePlacesClient) fetchPhotoURL(photoName string) (string, error) {
	url := fmt.Sprintf(googlePhotoURL, photoName, c.apiKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("photo fetch: status %d", resp.StatusCode)
	}

	var media googlePhotoMediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&media); err != nil {
		return "", err
	}
	return media.PhotoURI, nil
}

// ── Normalisation helpers ─────────────────────────────────────────────────────

func buildTextQuery(query, category, near string) string {
	cat := googleCategoryQuery[category] // "" if category not in map
	switch {
	case query != "" && cat != "" && near != "":
		return fmt.Sprintf("%s %s in %s", query, cat, near)
	case query != "" && near != "":
		return fmt.Sprintf("%s in %s", query, near)
	case cat != "" && near != "":
		return fmt.Sprintf("%s in %s", cat, near)
	case query != "" && cat != "":
		return fmt.Sprintf("%s %s", query, cat)
	case near != "":
		return fmt.Sprintf("places to visit in %s", near)
	case query != "":
		return query
	default:
		return "popular places"
	}
}

func normaliseGooglePlace(p googlePlace, photoURL string) *models.POI {
	cat, sub := classifyGoogleTypes(p.Types)
	return &models.POI{
		ID:          "gp:" + p.ID,
		Name:        p.DisplayName.Text,
		Category:    cat,
		Subcategory: sub,
		Address:     p.FormattedAddress,
		Rating:      p.Rating,
		ReviewCount: p.UserRatingCount,
		Description: p.EditorialSummary.Text,
		ImageURL:    photoURL,
		Lat:         p.Location.Latitude,
		Lng:         p.Location.Longitude,
		PriceLevel:  googlePriceLevel[p.PriceLevel],
	}
}

func classifyGoogleTypes(types []string) (category, subcategory string) {
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}
	for _, rule := range googleTypeCategory {
		if typeSet[rule.googleType] {
			// Use the matched type as a human-readable subcategory.
			return rule.category, humaniseType(rule.googleType)
		}
	}
	// Fallback: use the first non-generic type as the category label.
	for _, t := range types {
		if t != "point_of_interest" && t != "establishment" {
			return "attraction", humaniseType(t)
		}
	}
	return "attraction", ""
}

// humaniseType converts a Google Places type snake_case string to Title Case.
func humaniseType(t string) string {
	b := []byte(t)
	capitalise := true
	for i, c := range b {
		if c == '_' {
			b[i] = ' '
			capitalise = true
		} else if capitalise {
			if c >= 'a' && c <= 'z' {
				b[i] = c - 32
			}
			capitalise = false
		}
	}
	return string(b)
}
