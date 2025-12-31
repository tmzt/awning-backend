package images

import (
	"log/slog"
	"net/http"
	"strconv"

	"awning-backend/sections"
	"awning-backend/sections/common/auth"

	"github.com/gin-gonic/gin"
)

// Handler handles image-related requests
type Handler struct {
	logger *slog.Logger
	deps   *sections.Dependencies
}

// NewHandler creates a new images handler
func NewHandler(deps *sections.Dependencies) *Handler {
	return &Handler{
		logger: slog.With("handler", "ImageHandler"),
		deps:   deps,
	}
}

// SearchPhotos handles the /images/search endpoint
func (h *Handler) SearchPhotos(c *gin.Context) {
	if h.deps.UnsplashSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Image service not configured"})
		return
	}

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

	searchResp, err := h.deps.UnsplashSvc.SearchPhotos(c.Request.Context(), query, page, perPage, orientation, orderBy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search photos", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, searchResp)
}

// GetPhoto gets a single photo by ID
func (h *Handler) GetPhoto(c *gin.Context) {
	if h.deps.UnsplashSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Image service not configured"})
		return
	}

	photoID := c.Param("id")
	if photoID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Photo ID is required"})
		return
	}

	photo, err := h.deps.UnsplashSvc.GetPhoto(photoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get photo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, photo)
}

// RegisterRoutes registers image-related routes
func RegisterRoutes(r *gin.Engine, deps *sections.Dependencies, jwtManager *auth.JWTManager) {
	if deps.UnsplashSvc == nil {
		slog.Info("Skipping image routes - Unsplash service not configured")
		return
	}

	handler := NewHandler(deps)

	// Tenant-scoped image routes
	imageRoutes := r.Group("/api/v1/images")
	imageRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	{
		imageRoutes.GET("/search", handler.SearchPhotos)
		imageRoutes.GET("/photos/:id", handler.GetPhoto)
	}
}
