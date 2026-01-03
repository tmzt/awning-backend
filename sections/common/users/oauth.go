package users

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"awning-backend/common"
	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
	"golang.org/x/oauth2/google"
)

// OAuthConfig holds OAuth provider configurations
type OAuthConfig struct {
	Google   *oauth2.Config
	Facebook *oauth2.Config
	TikTok   *oauth2.Config
}

// OAuthHandler handles OAuth authentication
type OAuthHandler struct {
	logger      *slog.Logger
	deps        *sections.Dependencies
	jwtManager  *auth.JWTManager
	configs     *OAuthConfig
	userService *UserService
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(deps *sections.Dependencies, jwtManager *auth.JWTManager, configs *OAuthConfig) *OAuthHandler {
	return &OAuthHandler{
		logger:      slog.With("handler", "OAuthHandler"),
		deps:        deps,
		jwtManager:  jwtManager,
		configs:     configs,
		userService: NewUserService(deps),
	}
}

// NewOAuthConfig creates OAuth configurations from config
func NewOAuthConfig(config *common.Config) *OAuthConfig {
	configs := &OAuthConfig{}

	if config.OauthGoogleClientID != "" && config.OauthGoogleClientSecret != "" {
		configs.Google = &oauth2.Config{
			ClientID:     config.OauthGoogleClientID,
			ClientSecret: config.OauthGoogleClientSecret,
			// RedirectURL:  config.BaseURL + "/api/v1/auth/google/callback",
			RedirectURL: config.BaseURL + "/callbacks/oauth/google",
			Scopes:      []string{"openid", "email", "profile"},
			Endpoint:    google.Endpoint,
		}
	}

	if config.OauthFacebookClientID != "" && config.OauthFacebookClientSecret != "" {
		configs.Facebook = &oauth2.Config{
			ClientID:     config.OauthFacebookClientID,
			ClientSecret: config.OauthFacebookClientSecret,
			RedirectURL:  config.BaseURL + "/api/v1/auth/facebook/callback",
			Scopes:       []string{"email", "public_profile"},
			Endpoint:     facebook.Endpoint,
		}
	}

	if config.OauthTikTokClientID != "" && config.OauthTikTokClientSecret != "" {
		configs.TikTok = &oauth2.Config{
			ClientID:     config.OauthTikTokClientID,
			ClientSecret: config.OauthTikTokClientSecret,
			RedirectURL:  config.BaseURL + "/api/v1/auth/tiktok/callback",
			Scopes:       []string{"user.info.basic"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://www.tiktok.com/v2/auth/authorize/",
				TokenURL: "https://open.tiktokapis.com/v2/oauth/token/",
			},
		}
	}

	return configs
}

// GoogleLogin initiates Google OAuth flow
func (h *OAuthHandler) GoogleLogin(c *gin.Context) {
	h.logger.Debug("Initiating Google OAuth login")

	if h.configs.Google == nil {
		h.logger.Error("Google OAuth not configured")
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Google OAuth not configured"})
		return
	}

	state := generateOAuthState()
	// Store state in session/cookie for verification
	c.SetCookie("oauth_state", state, 300, "/", "", true, true)

	url := h.configs.Google.AuthCodeURL(state)
	h.logger.Debug("Redirecting to Google OAuth URL", "url", url)

	acceptJson := c.GetHeader("Accept") == "application/json"

	// For server-to-server flow, return the redirect URI instead of redirecting
	if c.Query("return_url") == "true" || acceptJson {
		c.JSON(http.StatusOK, common.ApiResponse[map[string]string]{
			Data:    map[string]string{"redirectUrl": url},
			Success: true,
		})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GoogleCallback handles Google OAuth callback
func (h *OAuthHandler) GoogleCallback(c *gin.Context) {
	if h.configs.Google == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Google OAuth not configured"})
		return
	}

	// Verify state
	state := c.Query("state")
	storedState, err := c.Cookie("oauth_state")
	if err != nil || state != storedState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}

	token, err := h.configs.Google.Exchange(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("Failed to exchange code for token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	// Get user info from Google
	userInfo, err := h.getGoogleUserInfo(token.AccessToken)
	if err != nil {
		h.logger.Error("Failed to get Google user info", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}

	// Find or create user
	user, err := h.findOrCreateGoogleUser(c.Request.Context(), userInfo)
	if err != nil {
		h.logger.Error("Failed to find or create user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	// Get primary tenant schema
	tenantSchema, err := h.userService.GetPrimaryTenantSchema(c.Request.Context(), user.ID)
	if err != nil {
		h.logger.Error("Failed to get primary tenant", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant"})
		return
	}

	// Generate JWT
	jwtToken, err := h.jwtManager.GenerateToken(user.ID, user.Email, tenantSchema)
	if err != nil {
		h.logger.Error("Failed to generate token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Store session in Redis
	sessionID := generateOAuthState() // Generate unique session ID
	if err := h.deps.Redis.SetSession(c.Request.Context(), sessionID, jwtToken, 24*time.Hour); err != nil {
		h.logger.Error("Failed to store session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store session"})
		return
	}

	// Redirect to frontend with token (or return JSON based on Accept header)
	frontendURL := c.Query("redirect_uri")
	if frontendURL != "" {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"?token="+jwtToken+"&session_id="+sessionID)
		return
	}

	c.JSON(http.StatusOK, common.ApiResponse[AuthResponse]{
		Data: AuthResponse{
			Token: jwtToken,
			User:  toUserResponse(user),
		},
		Success: true,
	})
}

type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

func (h *OAuthHandler) getGoogleUserInfo(accessToken string) (*googleUserInfo, error) {
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo googleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

func (h *OAuthHandler) findOrCreateGoogleUser(ctx context.Context, info *googleUserInfo) (*models.User, error) {
	now := time.Now()
	googleID := info.ID

	user := models.User{
		Email:           info.Email,
		FirstName:       info.GivenName,
		LastName:        info.FamilyName,
		GoogleID:        &googleID,
		EmailVerified:   info.VerifiedEmail,
		EmailVerifiedAt: &now,
		LastLoginAt:     &now,
		Active:          true,
	}

	return h.userService.FindOrCreateUserWithOAuth(ctx, user, "google", info.ID)
}

// FacebookLogin initiates Facebook OAuth flow
func (h *OAuthHandler) FacebookLogin(c *gin.Context) {
	if h.configs.Facebook == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Facebook OAuth not configured"})
		return
	}

	state := generateOAuthState()
	c.SetCookie("oauth_state", state, 300, "/", "", true, true)

	url := h.configs.Facebook.AuthCodeURL(state)

	acceptJson := c.GetHeader("Accept") == "application/json"

	// For server-to-server flow, return the redirect URI instead of redirecting
	if c.Query("return_url") == "true" || acceptJson {
		c.JSON(http.StatusOK, common.ApiResponse[map[string]string]{
			Data:    map[string]string{"redirectUrl": url},
			Success: true,
		})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, url)
}

// FacebookCallback handles Facebook OAuth callback
func (h *OAuthHandler) FacebookCallback(c *gin.Context) {
	if h.configs.Facebook == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "Facebook OAuth not configured"})
		return
	}

	state := c.Query("state")
	storedState, err := c.Cookie("oauth_state")
	if err != nil || state != storedState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}

	token, err := h.configs.Facebook.Exchange(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("Failed to exchange code for token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	userInfo, err := h.getFacebookUserInfo(token.AccessToken)
	if err != nil {
		h.logger.Error("Failed to get Facebook user info", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}

	user, err := h.findOrCreateFacebookUser(c.Request.Context(), userInfo)
	if err != nil {
		h.logger.Error("Failed to find or create user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	// Get primary tenant schema
	tenantSchema, err := h.userService.GetPrimaryTenantSchema(c.Request.Context(), user.ID)
	if err != nil {
		h.logger.Error("Failed to get primary tenant", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant"})
		return
	}

	jwtToken, err := h.jwtManager.GenerateToken(user.ID, user.Email, tenantSchema)
	if err != nil {
		h.logger.Error("Failed to generate token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Store session in Redis
	sessionID := generateOAuthState() // Generate unique session ID
	if err := h.deps.Redis.SetSession(c.Request.Context(), sessionID, jwtToken, 24*time.Hour); err != nil {
		h.logger.Error("Failed to store session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store session"})
		return
	}

	frontendURL := c.Query("redirect_uri")
	if frontendURL != "" {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"?token="+jwtToken+"&session_id="+sessionID)
		return
	}

	c.JSON(http.StatusOK, common.ApiResponse[AuthResponse]{
		Data: AuthResponse{
			Token: jwtToken,
			User:  toUserResponse(user),
		},
		Success: true,
	})
}

type facebookUserInfo struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (h *OAuthHandler) getFacebookUserInfo(accessToken string) (*facebookUserInfo, error) {
	resp, err := http.Get("https://graph.facebook.com/me?fields=id,email,name,first_name,last_name&access_token=" + accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo facebookUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	if userInfo.Email == "" {
		return nil, errors.New("email not provided by Facebook")
	}

	return &userInfo, nil
}

func (h *OAuthHandler) findOrCreateFacebookUser(ctx context.Context, info *facebookUserInfo) (*models.User, error) {
	facebookID := info.ID
	now := time.Now()
	user := models.User{
		Email:       info.Email,
		FirstName:   info.FirstName,
		LastName:    info.LastName,
		FacebookID:  &facebookID,
		LastLoginAt: &now,
		Active:      true,
	}

	return h.userService.FindOrCreateUserWithOAuth(ctx, user, "facebook", info.ID)
}

// TikTokLogin initiates TikTok OAuth flow
func (h *OAuthHandler) TikTokLogin(c *gin.Context) {
	if h.configs.TikTok == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "TikTok OAuth not configured"})
		return
	}

	state := generateOAuthState()
	c.SetCookie("oauth_state", state, 300, "/", "", true, true)

	// TikTok requires additional parameters
	url := fmt.Sprintf("%s?client_key=%s&scope=%s&response_type=code&redirect_uri=%s&state=%s",
		h.configs.TikTok.Endpoint.AuthURL,
		h.configs.TikTok.ClientID,
		"user.info.basic",
		h.configs.TikTok.RedirectURL,
		state,
	)

	acceptJson := c.GetHeader("Accept") == "application/json"

	// For server-to-server flow, return the redirect URI instead of redirecting
	if c.Query("return_url") == "true" || acceptJson {
		c.JSON(http.StatusOK, common.ApiResponse[map[string]string]{
			Data:    map[string]string{"redirectUrl": url},
			Success: true,
		})
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, url)
}

// TikTokCallback handles TikTok OAuth callback
func (h *OAuthHandler) TikTokCallback(c *gin.Context) {
	if h.configs.TikTok == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "TikTok OAuth not configured"})
		return
	}

	state := c.Query("state")
	storedState, err := c.Cookie("oauth_state")
	if err != nil || state != storedState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}

	token, err := h.configs.TikTok.Exchange(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("Failed to exchange code for token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	userInfo, err := h.getTikTokUserInfo(token.AccessToken)
	if err != nil {
		h.logger.Error("Failed to get TikTok user info", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user info"})
		return
	}

	user, err := h.findOrCreateTikTokUser(c.Request.Context(), userInfo)
	if err != nil {
		h.logger.Error("Failed to find or create user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	// Get primary tenant schema
	tenantSchema, err := h.userService.GetPrimaryTenantSchema(c.Request.Context(), user.ID)
	if err != nil {
		h.logger.Error("Failed to get primary tenant", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tenant"})
		return
	}

	jwtToken, err := h.jwtManager.GenerateToken(user.ID, user.Email, tenantSchema)
	if err != nil {
		h.logger.Error("Failed to generate token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Store session in Redis
	sessionID := generateOAuthState() // Generate unique session ID
	if err := h.deps.Redis.SetSession(c.Request.Context(), sessionID, jwtToken, 24*time.Hour); err != nil {
		h.logger.Error("Failed to store session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store session"})
		return
	}

	frontendURL := c.Query("redirect_uri")
	if frontendURL != "" {
		c.Redirect(http.StatusTemporaryRedirect, frontendURL+"?token="+jwtToken+"&session_id="+sessionID)
		return
	}

	c.JSON(http.StatusOK, common.ApiResponse[AuthResponse]{
		Data: AuthResponse{
			Token: jwtToken,
			User:  toUserResponse(user),
		},
		Success: true,
	})
}

type tiktokUserInfo struct {
	OpenID      string `json:"open_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

func (h *OAuthHandler) getTikTokUserInfo(accessToken string) (*tiktokUserInfo, error) {
	req, err := http.NewRequest("GET", "https://open.tiktokapis.com/v2/user/info/?fields=open_id,display_name,avatar_url", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Data struct {
			User tiktokUserInfo `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Data.User, nil
}

func (h *OAuthHandler) findOrCreateTikTokUser(ctx context.Context, info *tiktokUserInfo) (*models.User, error) {
	// TikTok doesn't provide email, so we need to handle this differently
	// Generate a placeholder email or require user to provide one later
	tiktokID := info.OpenID
	now := time.Now()
	user := models.User{
		Email:       fmt.Sprintf("tiktok_%s@placeholder.local", info.OpenID),
		FirstName:   info.DisplayName,
		TikTokID:    &tiktokID,
		LastLoginAt: &now,
		Active:      true,
	}

	return h.userService.FindOrCreateUserWithOAuth(ctx, user, "tiktok", info.OpenID)
}

func generateOAuthState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func toUserResponse(user *models.User) UserResponse {
	return UserResponse{
		ID:            user.ID,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		EmailVerified: user.EmailVerified,
		LastLoginAt:   user.LastLoginAt,
	}
}

// RegisterOAuthRoutes registers OAuth-related routes
func RegisterOAuthRoutes(frontendRoutes *gin.RouterGroup, callbackRoutes *gin.RouterGroup, deps *sections.Dependencies, jwtManager *auth.JWTManager, configs *OAuthConfig) {
	if configs == nil {
		return
	}

	handler := NewOAuthHandler(deps, jwtManager, configs)

	oauth := frontendRoutes.Group("/api/v1/auth")
	{
		if configs.Google != nil {
			oauth.GET("/google", handler.GoogleLogin)
			oauth.GET("/google/callback", handler.GoogleCallback)
		}
		if configs.Facebook != nil {
			oauth.GET("/facebook", handler.FacebookLogin)
			oauth.GET("/facebook/callback", handler.FacebookCallback)
		}
		if configs.TikTok != nil {
			oauth.GET("/tiktok", handler.TikTokLogin)
			oauth.GET("/tiktok/callback", handler.TikTokCallback)
		}
	}

	// oauthCallbacks := callbackRoutes.Group("/oauth")
	// {
	// 	if configs.Google != nil {
	// 		slog.Debug("Registering Google OAuth callback route")
	// 		oauthCallbacks.GET("/google", handler.GoogleCallback)
	// 	}
	// 	if configs.Facebook != nil {
	// 		oauthCallbacks.GET("/facebook", handler.FacebookCallback)
	// 	}
	// 	if configs.TikTok != nil {
	// 		oauthCallbacks.GET("/tiktok", handler.TikTokCallback)
	// 	}
	// }
}
