package main

import (
	"context"
	"log/slog"
	"os"
	"path"
	"strings"

	"awning-backend/common"
	"awning-backend/handlers"
	"awning-backend/processors"
	"awning-backend/services"
	"awning-backend/storage"
	"awning-backend/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// VertexClientAdapter adapts the main package VertexOpenAIClient to the handlers.VertexClient interface
type VertexClientAdapter struct {
	client *VertexOpenAIClient
}

func (a *VertexClientAdapter) GenerateContentStream(ctx context.Context, prompt string, callback func(handlers.StreamEvent) error) error {
	return a.client.GenerateContentStream(ctx, prompt, func(event StreamEvent) error {
		// Convert main.StreamEvent to handlers.StreamEvent
		return callback(handlers.StreamEvent{
			Type:    event.Type,
			Content: event.Content,
		})
	})
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func main() {
	ctx := context.Background()

	// Set up structured logging with debug level
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Load environment variables
	if _, err := os.Stat(common.PRIVATE_CREDENTIALS_DOTENV); err == nil {
		if err := godotenv.Load(".env.private"); err != nil {
			slog.Error("Failed to load .env.private file", "error", err)
			os.Exit(1)
		}
	}

	cfgDir := getEnv("CONFIG_DIR", common.DEFAULT_CONFIG_DIR)

	// Load configuration

	cfg, err := common.LoadConfig(cfgDir)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	promptName := getEnv("PROMPT_NAME", common.DEFAULT_PROMPT_NAME)

	promptFile := path.Join(cfgDir, "prompts", promptName+".template")

	// Load prompt template
	if _, err := os.Stat(promptFile); os.IsNotExist(err) {
		slog.Error("Prompt template file does not exist", "file", promptFile)
		os.Exit(1)
	}

	promptBuilder, err := utils.NewPromptBuilder(promptFile)
	if err != nil {
		slog.Error("Failed to load prompt template", "error", err)
		os.Exit(1)
	}
	slog.Info("Prompt template loaded successfully")

	// Load Vertex AI credentials
	var credData []byte
	if s := getEnv("SERVICE_CREDENTIALS_JSON", ""); s != "" {
		credData = []byte(s)
	} else if _, err := os.Stat(common.PRIVATE_CREDENTIALS_FILE); err == nil {
		data, err := os.ReadFile(common.PRIVATE_CREDENTIALS_FILE)
		if err != nil {
			slog.Error("Failed to read credentials file", "error", err)
			os.Exit(1)
		}
		credData = data
	} else {
		slog.Error("Credentials not provided", "file", common.PRIVATE_CREDENTIALS_FILE)
		os.Exit(1)
	}

	// Initialize Vertex AI client using service account credentials
	if err := InitGlobalVertexOpenAIClient(ctx, cfg, credData); err != nil {
		slog.Error("Failed to initialize Vertex AI client", "error", err)
		os.Exit(1)
	}

	// Initialize Redis client
	redisClient, err := storage.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword, 0)
	if err != nil {
		slog.Error("Failed to initialize Redis client", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// Create processors service
	processorsSvc := services.NewProcessors(cfg)

	// Initialize chat handler with adapter
	vertexAdapter := &VertexClientAdapter{client: GlobalVertexOpenAIClient}
	chatHandler := handlers.NewChatHandler(cfg, redisClient, promptBuilder, vertexAdapter, processorsSvc)

	var imageHandler *handlers.ImageHandler

	// Initialize Unsplash API client (if API key provided)
	if accessKey, secretKey := cfg.UnsplashAPIAccessKey, cfg.UnsplashAPISecretKey; accessKey != "" && secretKey != "" {
		slog.Info("Unsplash API keys provided, initializing Unsplash service and image handler")

		unsplashSvc := services.NewUnsplashService(accessKey, secretKey)

		// Initialize Unsplash handler
		imageHandler = handlers.NewImageHandler(cfg, unsplashSvc)

		// Register header processor
		processorsSvc.RegisterProcessor("header", processors.NewHeaderProcessor(cfg))

		// Register image processor
		processorsSvc.RegisterProcessor("image", processors.NewImageProcessor(cfg, unsplashSvc))

		// Register cleanup processor
		processorsSvc.RegisterProcessor("cleanup", processors.NewCleanupProcessor(cfg))

	} else {
		slog.Info("No Unsplash API key provided - skipping Unsplash service and image handler initialization")
	}

	// Initialize Gin router
	r := gin.Default()

	env := getEnv("APP_ENV", "production")
	trustedProxies := getEnv("TRUSTED_PROXIES", "")
	corsOrigins := getEnv("CORS_ORIGINS", "")

	if env != "development" && trustedProxies == "" {
		slog.Error("In production mode, TRUSTED_PROXIES must be set")
		os.Exit(1)
	} else if trustedProxies != "" {
		slog.Info("Setting trusted proxies", "proxies", trustedProxies)
		proxies := strings.Split(trustedProxies, ",")
		if err := r.SetTrustedProxies(proxies); err != nil {
			slog.Error("Failed to set trusted proxies", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Warn("No trusted proxies set (TRUSTED_PROXIES not defined)")
	}

	// Configure CORS
	corsConfig := cors.DefaultConfig()

	if env != "development" && corsOrigins == "" {
		slog.Error("In production mode, CORS_ORIGINS must be set")
		os.Exit(1)
	} else if corsOrigins != "" {
		slog.Info("CORS origins set from CORS_ORIGINS")
		corsConfig.AllowOrigins = strings.Split(corsOrigins, ",")
	} else {
		slog.Warn("Using default origin function in non-production mode (CORS_ORIGINS not defined)")
		corsConfig.AllowOriginFunc = func(origin string) bool {
			if origin == "http://localhost" || strings.HasPrefix(origin, "http://localhost:") {
				return true
			}
			return false
		}
	}

	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	r.Use(cors.New(corsConfig))

	// API routes - streaming chat only
	r.POST("/api/v1/chat/stream", chatHandler.CreateChatStream)
	r.GET("/api/v1/chat/:id", chatHandler.GetChat)
	r.DELETE("/api/v1/chat/:id", chatHandler.DeleteChat)

	// Image API routes
	if imageHandler != nil {
		r.GET("/api/v1/images/search", imageHandler.SearchPhotos)
		r.GET("/api/v1/images/photos/:id", imageHandler.GetPhoto)
	}

	// Serve static files from APP_PUBLIC if set
	if publicDir := os.Getenv("APP_PUBLIC"); publicDir != "" {
		slog.Info("Serving static files", "directory", publicDir)
		// r.Static("/assets", publicDir+"/assets")
		// r.StaticFile("/", publicDir+"/index.html")
		r.Static("/", publicDir)
		r.NoRoute(func(c *gin.Context) {
			// For SPA: serve index.html for non-API routes that don't match static files
			if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.File(publicDir + "/index.html")
			}
		})
	} else {
		slog.Info("No static file directory set (APP_PUBLIC not defined)")
	}

	slog.Info("Server starting", "addr", cfg.ListenAddr)
	if err := r.Run(cfg.ListenAddr); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
