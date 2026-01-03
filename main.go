package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptoRand "crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log/slog"
	"os"
	"path"
	"strings"

	"awning-backend/common"
	"awning-backend/db"
	"awning-backend/handlers"
	"awning-backend/middleware"
	"awning-backend/processors"
	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/common/users"
	"awning-backend/sections/models"
	"awning-backend/sections/tenant/account"
	"awning-backend/sections/tenant/chat"
	"awning-backend/sections/tenant/domains"
	"awning-backend/sections/tenant/filesystem"
	"awning-backend/sections/tenant/images"
	"awning-backend/sections/tenant/payment"
	"awning-backend/sections/tenant/profile"
	"awning-backend/services"
	"awning-backend/storage"
	"awning-backend/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// VertexClientAdapter adapts the main package VertexOpenAIClient to both handlers.VertexClient
// and sections.VertexClient interfaces
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

// GenerateContentStreamSections adapts to sections.VertexClient interface
func (a *VertexClientAdapter) GenerateContentStreamSections(ctx context.Context, prompt string, callback func(sections.StreamEvent) error) error {
	return a.client.GenerateContentStream(ctx, prompt, func(event StreamEvent) error {
		return callback(sections.StreamEvent{
			Type:    event.Type,
			Content: event.Content,
		})
	})
}

// sectionsVertexAdapter wraps VertexClientAdapter to implement sections.VertexClient
type sectionsVertexAdapter struct {
	adapter *VertexClientAdapter
}

func (s *sectionsVertexAdapter) GenerateContentStream(ctx context.Context, prompt string, callback func(sections.StreamEvent) error) error {
	return s.adapter.GenerateContentStreamSections(ctx, prompt, callback)
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func main() {
	ctx := context.Background()

	// If called with geneckey argument, generate a keypair and exit
	if len(os.Args) == 2 && os.Args[1] == "geneckey" {
		// privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		// if err != nil {
		// 	slog.Error("Failed to generate key pair", "error", err)
		// 	os.Exit(1)
		// }
		// slog.Info("Generated JWT key pair")
		// // slog.Info("Private Key (base64):", "key", auth.EncodePrivateKeyToBase64(privateKey))
		// // slog.Info("Public Key (base64):", "key", auth.EncodePublicKeyToBase64(publicKey))
		// slog.Info("Private Key (PEM):", "key", auth.EncodePrivateKeyToPEM(privateKey))
		// os.Exit(0)

		// Generate ECDSA P-521 key pair
		privateKey, err := ecdsa.GenerateKey(elliptic.P521(), cryptoRand.Reader)
		if err != nil {
			slog.Error("Failed to generate ECDSA key pair", "error", err)
			os.Exit(1)
		}
		publicKey := &privateKey.PublicKey

		// Encode keys to PEM format
		privBytes, err := x509.MarshalECPrivateKey(privateKey)
		if err != nil {
			slog.Error("Failed to marshal private key", "error", err)
			os.Exit(1)
		}
		privPem := pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privBytes,
		})
		privB64 := base64.StdEncoding.EncodeToString(privPem)

		pubBytes, err := x509.MarshalPKIXPublicKey(publicKey)
		if err != nil {
			slog.Error("Failed to marshal public key", "error", err)
			os.Exit(1)
		}
		pubPem := pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubBytes,
		})
		pubB64 := base64.StdEncoding.EncodeToString(pubPem)

		slog.Info("Generated ECDSA P-521 key pair")
		slog.Info("Private Key (BASE64'd PEM):\n" + privB64)
		slog.Info("Public Key (BASE64'd PEM):\n" + pubB64)
		os.Exit(0)
	}

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

	slog.Info("cfg: ", slog.Any("config", cfg))

	// promptName := getEnv("PROMPT_NAME", common.DEFAULT_PROMPT_NAME)

	promptName := cfg.PromptName
	if promptName == "" {
		promptName = common.DEFAULT_PROMPT_NAME
	}

	promptType := "oneshot"
	if cfg.PromptFormat == common.PromptFormatHtmlTemplateBased {
		promptType = "html"
	}

	basePromptFile := path.Join(cfgDir, "prompts", promptType, promptName+"-base.md")
	requestPromptFile := path.Join(cfgDir, "prompts", promptType, promptName+"-request.md")

	// Load prompt template
	if _, err := os.Stat(basePromptFile); os.IsNotExist(err) {
		slog.Error("Base prompt template file does not exist", "file", basePromptFile)
		os.Exit(1)
	}

	if _, err := os.Stat(requestPromptFile); os.IsNotExist(err) {
		slog.Error("Request prompt template file does not exist", "file", requestPromptFile)
		os.Exit(1)
	}

	promptBuilder, err := utils.NewPromptBuilder(basePromptFile, requestPromptFile)
	if err != nil {
		slog.Error("Failed to load prompt template", "error", err)
		os.Exit(1)
	}
	slog.Info("Prompt template loaded successfully")

	plans, err := common.LoadPlans(cfgDir)
	if err != nil {
		slog.Error("Failed to load plans", "error", err)
		os.Exit(1)
	}

	env := getEnv("APP_ENV", "production")

	// Require api key and secret in production
	// if env == "production" && (cfg.ApiKey == "" || cfg.ApiKeySecret == "") {
	// 	slog.Error("API_KEY and API_KEY_SECRET must be set in environment or config in production")
	// 	os.Exit(1)
	// }
	if env == "production" && cfg.ApiFrontendKey == "" {
		slog.Error("API_FRONTEND_KEY must be set in environment or config in production")
		os.Exit(1)
	}

	// Load Vertex AI credentials
	var credData []byte
	if s := getEnv("SERVICE_CREDENTIALS_JSON", ""); s != "" {
		credData = []byte(s)
	} else if s := getEnv("SERVICE_CREDENTIALS_FILE", ""); s != "" {
		data, err := os.ReadFile(s)
		if err != nil {
			slog.Error("Failed to read credentials file", "file", s, "error", err)
			os.Exit(1)
		}
		credData = data
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

	// Initialize database connection (optional - only if DATABASE_URL is set)
	var database *db.DB
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL != "" {
		slog.Info("Connecting to database")
		database, err = db.Connect(databaseURL)
		if err != nil {
			slog.Error("Failed to connect to database", "error", err)
			os.Exit(1)
		}

		// Register and migrate shared models
		if err := database.RegisterModels(ctx,
			&models.Tenant{},
			&models.User{},
			&models.UserTenant{},
			&models.Payment{},
			&models.Subscription{},
			// Tenant models
			&models.TenantFilesystem{},
			&models.TenantChat{},
			&models.TenantProfile{},
			&models.TenantDomain{},
		); err != nil {
			slog.Error("Failed to register models", "error", err)
			os.Exit(1)
		}

		if err := database.MigrateSharedModels(ctx); err != nil {
			slog.Error("Failed to migrate shared models", "error", err)
			os.Exit(1)
		}
		slog.Info("Database connected and shared models migrated")
	} else {
		slog.Info("No DATABASE_URL set - running in legacy mode without PostgreSQL")
	}

	// Initialize JWT manager (optional - only if JWT_PRIVATE_KEY is set)
	var jwtManager *auth.JWTManager
	if jwtPrivateKey := getEnv("JWT_PRIVATE_KEY", ""); jwtPrivateKey != "" {
		jwtManager, err = auth.NewJWTManagerFromEnv()
		if err != nil {
			slog.Error("Failed to initialize JWT manager", "error", err)
			os.Exit(1)
		}
		slog.Info("JWT manager initialized")
	} else {
		slog.Info("No JWT_PRIVATE_KEY set - JWT authentication disabled")
	}

	// Create processors service
	processorsSvc := services.NewProcessors(cfg)

	// Initialize chat handler with adapter (legacy handler)
	vertexAdapter := &VertexClientAdapter{client: GlobalVertexOpenAIClient}
	// chatHandler := handlers.NewChatHandler(cfg, redisClient, promptBuilder, vertexAdapter, processorsSvc)

	// var imageHandler *handlers.ImageHandler
	var unsplashSvc *services.UnsplashService

	// Initialize Unsplash API client (if API key provided)
	if accessKey, secretKey := cfg.UnsplashAPIAccessKey, cfg.UnsplashAPISecretKey; accessKey != "" && secretKey != "" {
		slog.Info("Unsplash API keys provided, initializing Unsplash service and image handler")

		unsplashSvc = services.NewUnsplashService(accessKey, secretKey)

		// Initialize Unsplash handler
		// imageHandler = handlers.NewImageHandler(cfg, unsplashSvc)

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

	trustedProxies := getEnv("TRUSTED_PROXIES", "")

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

	corsOrigins := getEnv("CORS_ORIGINS", "")
	if env != "development" && corsOrigins == "" {
		slog.Error("In production mode, CORS_ORIGINS must be set")
		os.Exit(1)
	} else if corsOrigins != "" {
		slog.Info("CORS origins set from CORS_ORIGINS")
		corsConfig.AllowOrigins = strings.Split(corsOrigins, ",")
	} else {
		slog.Warn("Using default origin function in non-production mode (CORS_ORIGINS not defined)")
		corsConfig.AllowOriginFunc = func(origin string) bool {
			// slog.Info("CORS origin check", "origin", origin)
			// fmt.Println("CORS origin check:", origin)
			if origin == "http://localhost" || strings.HasPrefix(origin, "http://localhost:") {
				return true
			}
			return false
		}
	}

	// corsConfig.AllowOriginFunc = func(origin string) bool {
	// 	if origin == "http://localhost" || strings.HasPrefix(origin, "http://localhost:") {
	// 		return true
	// 	}
	// 	return false
	// }

	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Awning-Frontend-Key"}
	r.Use(cors.New(corsConfig))

	// // Require api key and secret for all requests
	// r.Use(middleware.APIKeyAuthMiddleware(func(ctx context.Context, providedKey, providedSecret string) (context.Context, error) {
	// 	if providedKey == cfg.ApiKey && providedSecret == cfg.ApiKeySecret {
	// 		return ctx, nil
	// 	}
	// 	return ctx, middleware.ErrMissingAPICredentials
	// }))

	// Allow frontend API key for all requests
	// r.Use(middleware.APIFrontendKeyAuthMiddleware(cfg.ApiFrontendKey))

	// publicRoutes := r.Group("/")

	frontendRoutes := r.Group("/")
	frontendRoutes.Use(middleware.APIFrontendKeyAuthMiddleware(cfg.ApiFrontendKey))

	privateRoutes := r.Group("/")
	privateRoutes.Use(middleware.APIFrontendKeyAuthMiddleware(cfg.ApiFrontendKey))
	privateRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	privateRoutes.Use(auth.TenantFromHeaderMiddleware(auth.DefaultTenantMiddlewareConfig()))

	callbackRoutes := r.Group("/callbacks")
	webhookRoutes := r.Group("/webhooks")

	slog.Info("Database: ", slog.Any("database", database))
	slog.Info("JWT Manager: ", slog.Any("jwt_manager", jwtManager))

	// Initialize new sections-based routes if database is available
	if database != nil && jwtManager != nil {
		slog.Info("Initializing multi-tenant sections")

		// Create shared dependencies for new handlers
		deps := &sections.Dependencies{
			Config:        cfg,
			DB:            database,
			Redis:         redisClient,
			PromptBuilder: promptBuilder,
			VertexClient:  &sectionsVertexAdapter{adapter: vertexAdapter},
			ProcessorsSvc: processorsSvc,
			UnsplashSvc:   unsplashSvc,
		}

		// Register user routes (public - no tenant context needed)
		users.RegisterRoutes(frontendRoutes, deps, jwtManager)

		// Register OAuth routes if configured
		if cfg.OauthGoogleClientID != "" || cfg.OauthFacebookClientID != "" || cfg.OauthTikTokClientID != "" {
			slog.Info("OAuth client IDs provided, registering OAuth routes")
			oauthConfig := users.NewOAuthConfig(cfg)
			users.RegisterOAuthRoutes(frontendRoutes, callbackRoutes, deps, jwtManager, oauthConfig)
			slog.Info("OAuth routes registered")
		}

		// Initialize Stripe service if configured
		var stripeSvc *services.StripeService
		stripeSecretKey := getEnv("STRIPE_SECRET_KEY", "")
		stripeWebhookSecret := getEnv("STRIPE_WEBHOOK_SECRET", "")
		if stripeSecretKey != "" && stripeWebhookSecret != "" {
			slog.Info("Stripe keys provided, initializing Stripe service")
			stripeSuccessURL := getEnv("STRIPE_SUCCESS_URL", cfg.BaseURL+"/payment/success")
			stripeCancelURL := getEnv("STRIPE_CANCEL_URL", cfg.BaseURL+"/payment/cancel")
			stripeSvc = services.NewStripeService(plans, stripeSecretKey, stripeWebhookSecret, stripeSuccessURL, stripeCancelURL)
			slog.Info("Stripe service initialized")
		} else {
			slog.Info("Stripe not configured - payment features disabled")
		}

		// Register tenant-scoped routes
		// Each RegisterRoutes function creates its own route group with JWT + tenant middleware
		chat.RegisterRoutes(frontendRoutes, deps, jwtManager)
		images.RegisterRoutes(frontendRoutes, deps, jwtManager)
		profile.RegisterRoutes(frontendRoutes, deps, jwtManager)
		account.RegisterRoutes(frontendRoutes, deps, jwtManager)
		filesystem.RegisterRoutes(frontendRoutes, deps, jwtManager)

		// Register payment routes if Stripe is configured
		if stripeSvc != nil {
			payment.RegisterRoutes(frontendRoutes, webhookRoutes, deps, jwtManager, stripeSvc)
			slog.Info("Payment routes registered")
		}

		// Initialize domain registrar and register domain routes
		registrarFactory := domains.NewRegistrarFactory()
		registrar, err := registrarFactory.Create(&domains.RegistrarConfig{
			Provider:  cfg.DomainRegistrarProvider,
			APIKey:    cfg.DomainRegistrarAPIKey,
			APISecret: cfg.DomainRegistrarSecret,
			Username:  cfg.DomainRegistrarUsername,
			Sandbox:   cfg.DomainRegistrarSandbox,
		})
		if err != nil {
			slog.Warn("Failed to create domain registrar, domain routes will be unavailable", "error", err)
		} else {
			domains.RegisterRoutes(r, deps, jwtManager, registrar)
			slog.Info("Domain routes registered", "provider", cfg.DomainRegistrarProvider)
		}

		slog.Info("Multi-tenant sections initialized")
	}

	// Legacy API routes - streaming chat only (backward compatibility)
	// r.POST("/api/v1/chat/stream", chatHandler.CreateChatStream)
	// r.GET("/api/v1/chat/:id", chatHandler.GetChat)
	// r.DELETE("/api/v1/chat/:id", chatHandler.DeleteChat)

	// Image API routes
	// if imageHandler != nil {
	// 	r.GET("/api/v1/images/search", imageHandler.SearchPhotos)
	// 	r.GET("/api/v1/images/photos/:id", imageHandler.GetPhoto)
	// }

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
	} else if publicProxy := os.Getenv("APP_PUBLIC_PROXY"); publicProxy != "" {
		slog.Info("Serving static files via proxy", "proxy", publicProxy)
		r.Use(middleware.StaticProxyMiddleware(publicProxy))
	} else {
		slog.Info("No static file directory set (APP_PUBLIC not defined) and no proxy set (APP_PUBLIC_PROXY not defined)")
	}

	slog.Info("Server starting", "addr", cfg.ListenAddr)
	if err := r.Run(cfg.ListenAddr); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
