package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	// Cache TTL for filesystem entries
	CacheTTL = 5 * time.Minute
)

// Handler handles filesystem-related requests
type Handler struct {
	logger *slog.Logger
	deps   *sections.Dependencies
}

// NewHandler creates a new filesystem handler
func NewHandler(deps *sections.Dependencies) *Handler {
	return &Handler{
		logger: slog.With("handler", "FilesystemHandler"),
		deps:   deps,
	}
}

// FilesystemEntry represents a filesystem entry response
type FilesystemEntry struct {
	ID          uint   `json:"id"`
	Key         string `json:"key"`
	Data        any    `json:"data"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
	UpdatedAt   string `json:"updatedAt"`
}

// cacheKey generates a Redis cache key for a filesystem entry
func (h *Handler) cacheKey(tenantID, key string) string {
	return fmt.Sprintf("fs:%s:%s", tenantID, key)
}

// GetEntry retrieves a filesystem entry by key
func (h *Handler) GetEntry(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	key := c.Param("key")
	if key == "" {
		key = c.Query("key")
	}
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	ctx := c.Request.Context()

	// Try to get from Redis cache first
	if h.deps.Redis != nil {
		cached, err := h.getFromCache(ctx, tenantID, key)
		if err == nil && cached != nil {
			h.logger.Debug("Cache hit", "tenant", tenantID, "key", key)
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Get from database
	var entry models.TenantFilesystem
	err := h.deps.DB.WithTenant(ctx, tenantID, func(tx *gorm.DB) error {
		return tx.Where("tenant_schema = ? AND key = ?", tenantID, key).First(&entry).Error
	})

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
		return
	}

	response := h.toResponse(&entry)

	// Cache the result
	if h.deps.Redis != nil {
		h.cacheEntry(ctx, tenantID, key, &response)
	}

	c.JSON(http.StatusOK, response)
}

// PutEntry creates or updates a filesystem entry
func (h *Handler) PutEntry(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	// Read the JSON body
	var data json.RawMessage
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON data"})
		return
	}

	dataStr := string(data)
	checksum := sha256.Sum256(data)
	checksumHex := hex.EncodeToString(checksum[:])

	ctx := c.Request.Context()

	var entry models.TenantFilesystem
	err := h.deps.DB.WithTenant(ctx, tenantID, func(tx *gorm.DB) error {
		// Try to find existing entry
		err := tx.Where("tenant_schema = ? AND key = ?", tenantID, key).First(&entry).Error
		if err != nil {
			// Create new entry
			entry = models.TenantFilesystem{
				TenantSchema: tenantID,
				Key:          key,
			}
		}

		entry.Data = dataStr
		entry.ContentType = "application/json"
		entry.Size = int64(len(data))
		entry.Checksum = checksumHex

		return tx.Save(&entry).Error
	})

	if err != nil {
		h.logger.Error("Failed to save filesystem entry", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save entry"})
		return
	}

	response := h.toResponse(&entry)

	// Update cache
	if h.deps.Redis != nil {
		h.cacheEntry(ctx, tenantID, key, &response)
	}

	c.JSON(http.StatusOK, response)
}

// DeleteEntry removes a filesystem entry
func (h *Handler) DeleteEntry(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	ctx := c.Request.Context()

	err := h.deps.DB.WithTenant(ctx, tenantID, func(tx *gorm.DB) error {
		return tx.Where("tenant_schema = ? AND key = ?", tenantID, key).Delete(&models.TenantFilesystem{}).Error
	})

	if err != nil {
		h.logger.Error("Failed to delete filesystem entry", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete entry"})
		return
	}

	// Invalidate cache
	if h.deps.Redis != nil {
		h.invalidateCache(ctx, tenantID, key)
	}

	c.JSON(http.StatusOK, gin.H{"message": "entry deleted"})
}

// ListEntries lists all filesystem entries for a tenant
func (h *Handler) ListEntries(c *gin.Context) {
	tenantID, ok := auth.GetTenantIDFromContext(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant context required"})
		return
	}

	prefix := c.Query("prefix")

	var entries []models.TenantFilesystem
	err := h.deps.DB.WithTenant(c.Request.Context(), tenantID, func(tx *gorm.DB) error {
		query := tx.Where("tenant_schema = ?", tenantID)
		if prefix != "" {
			query = query.Where("key LIKE ?", prefix+"%")
		}
		return query.Select("id, key, content_type, size, checksum, updated_at").Find(&entries).Error
	})

	if err != nil {
		h.logger.Error("Failed to list filesystem entries", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list entries"})
		return
	}

	// Return metadata only, not data
	type EntryMeta struct {
		ID          uint   `json:"id"`
		Key         string `json:"key"`
		ContentType string `json:"contentType"`
		Size        int64  `json:"size"`
		Checksum    string `json:"checksum"`
		UpdatedAt   string `json:"updatedAt"`
	}

	responses := make([]EntryMeta, len(entries))
	for i, e := range entries {
		responses[i] = EntryMeta{
			ID:          e.ID,
			Key:         e.Key,
			ContentType: e.ContentType,
			Size:        e.Size,
			Checksum:    e.Checksum,
			UpdatedAt:   e.UpdatedAt.Format(time.RFC3339),
		}
	}

	c.JSON(http.StatusOK, gin.H{"entries": responses})
}

// Redis cache helpers

func (h *Handler) getFromCache(ctx context.Context, tenantID, key string) (*FilesystemEntry, error) {
	cacheKey := h.cacheKey(tenantID, key)

	// Use Redis client to get cached data
	data, err := h.deps.Redis.Get(ctx, cacheKey)
	if err != nil {
		return nil, err
	}

	var entry FilesystemEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

func (h *Handler) cacheEntry(ctx context.Context, tenantID, key string, entry *FilesystemEntry) {
	cacheKey := h.cacheKey(tenantID, key)

	data, err := json.Marshal(entry)
	if err != nil {
		h.logger.Error("Failed to marshal entry for cache", "error", err)
		return
	}

	if err := h.deps.Redis.SetWithTTL(ctx, cacheKey, data, CacheTTL); err != nil {
		h.logger.Error("Failed to cache entry", "error", err)
	}
}

func (h *Handler) invalidateCache(ctx context.Context, tenantID, key string) {
	cacheKey := h.cacheKey(tenantID, key)
	if err := h.deps.Redis.Delete(ctx, cacheKey); err != nil {
		h.logger.Error("Failed to invalidate cache", "error", err)
	}
}

func (h *Handler) toResponse(entry *models.TenantFilesystem) FilesystemEntry {
	var data any
	if err := json.Unmarshal([]byte(entry.Data), &data); err != nil {
		data = entry.Data // Return as string if not valid JSON
	}

	return FilesystemEntry{
		ID:          entry.ID,
		Key:         entry.Key,
		Data:        data,
		ContentType: entry.ContentType,
		Size:        entry.Size,
		Checksum:    entry.Checksum,
		UpdatedAt:   entry.UpdatedAt.Format(time.RFC3339),
	}
}

// RegisterRoutes registers filesystem-related routes
func RegisterRoutes(r *gin.RouterGroup, deps *sections.Dependencies, jwtManager *auth.JWTManager) {
	handler := NewHandler(deps)

	tenantCfg := auth.DefaultTenantMiddlewareConfig()

	fsRoutes := r.Group("/api/v1/filesystem")
	fsRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	fsRoutes.Use(auth.TenantFromHeaderMiddleware(tenantCfg))
	{
		fsRoutes.GET("", handler.ListEntries)
		fsRoutes.GET("/*key", handler.GetEntry)
		fsRoutes.PUT("/*key", handler.PutEntry)
		fsRoutes.DELETE("/*key", handler.DeleteEntry)
	}
}
