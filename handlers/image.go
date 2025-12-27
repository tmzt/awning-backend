package handlers

import (
	"awning-backend/common"
	"awning-backend/services"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ImageHandler struct {
	logger *slog.Logger
	config *common.Config
	svc    *services.UnsplashService
}

func NewImageHandler(config *common.Config, svc *services.UnsplashService) *ImageHandler {
	logger := slog.With("handler", "ImageHandler")

	return &ImageHandler{
		logger: logger,
		config: config,
		svc:    svc,
	}
}

// SearchPhotos handles the /images/search/photos endpoint
func (h *ImageHandler) SearchPhotos(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter is required"})
		return
	}

	// Parse optional parameters with defaults
	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	perPage := 10
	if pp := c.Query("per_page"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 30 {
			perPage = parsed
		}
	}

	orientation := c.Query("orientation") // landscape, portrait, squarish
	orderBy := c.Query("order_by")        // latest, oldest, popular
	if orderBy == "" {
		orderBy = "relevant"
	}

	searchResp, err := h.svc.SearchPhotos(c.Request.Context(), query, page, perPage, orientation, orderBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search photos", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, searchResp)
}

// GetPhoto gets a single photo by ID from Unsplash (/images/photos/:id)
func (h *ImageHandler) GetPhoto(c *gin.Context) {
	photoID := c.Param("id")
	if photoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Photo ID is required"})
		return
	}

	photo, err := h.svc.GetPhoto(photoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get photo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, photo)
}
