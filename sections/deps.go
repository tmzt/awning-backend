package sections

import (
	"context"

	"awning-backend/common"
	"awning-backend/db"
	"awning-backend/services"
	"awning-backend/storage"
	"awning-backend/utils"
)

// StreamEvent represents a streaming event from the AI
type StreamEvent struct {
	Type    string
	Content string
}

// VertexClient interface for AI content generation (copied from handlers for decoupling)
type VertexClient interface {
	GenerateContentStream(ctx context.Context, prompt string, callback func(StreamEvent) error) error
}

// Dependencies holds all shared dependencies for handlers
type Dependencies struct {
	Config        *common.Config
	DB            *db.DB
	Redis         *storage.RedisClient
	PromptBuilder *utils.PromptBuilder
	VertexClient  VertexClient
	ProcessorsSvc *services.Processors
	UnsplashSvc   *services.UnsplashService
}

// NewDependencies creates a new Dependencies instance
func NewDependencies(
	cfg *common.Config,
	database *db.DB,
	redis *storage.RedisClient,
	promptBuilder *utils.PromptBuilder,
	vertexClient VertexClient,
	processorsSvc *services.Processors,
	unsplashSvc *services.UnsplashService,
) *Dependencies {
	return &Dependencies{
		Config:        cfg,
		DB:            database,
		Redis:         redis,
		PromptBuilder: promptBuilder,
		VertexClient:  vertexClient,
		ProcessorsSvc: processorsSvc,
		UnsplashSvc:   unsplashSvc,
	}
}
