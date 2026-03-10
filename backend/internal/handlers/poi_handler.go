package handlers

import (
	"net/http"

	"cs485/internal/models"
	"cs485/internal/services"

	"github.com/gin-gonic/gin"
)

// POIHandler exposes the point-of-interest search endpoint.
type POIHandler struct {
	poiSvc *services.POIService
}

func NewPOIHandler(poiSvc *services.POIService) *POIHandler {
	return &POIHandler{poiSvc: poiSvc}
}

// Search godoc  GET /api/pois/search?q=<text>&category=<category>
// Both parameters are optional; omitting both returns all POIs sorted by rating.
func (h *POIHandler) Search(c *gin.Context) {
	query := c.Query("q")
	category := c.Query("category")
	near := c.Query("near")

	pois, err := h.poiSvc.Search(query, category, near)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if pois == nil {
		pois = []*models.POI{}
	}
	c.JSON(http.StatusOK, gin.H{"pois": pois})
}
