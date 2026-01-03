package auth

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrMissingToken     = errors.New("missing authorization token")
	ErrInvalidSignature = errors.New("invalid token signature")
)

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID       uint   `json:"userId"`
	Email        string `json:"email"`
	TenantSchema string `json:"tenantSchema,omitempty"`
}

// JWTManager handles JWT operations
type JWTManager struct {
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
	issuer     string
	expiry     time.Duration
}

// NewJWTManager creates a new JWT manager from a PEM-encoded ES512 private key
func NewJWTManager(privateKeyPEM string, issuer string, expiryHours int) (*JWTManager, error) {
	if privateKeyPEM == "" {
		return nil, errors.New("JWT_PRIVATE_KEY environment variable is required")
	}

	slog.Info("Decoding JWT private key: ", slog.String("key", privateKeyPEM))

	// Parse PEM block
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("failed to parse PEM block from private key")
	}

	// privateKeyBytes := []byte(privateKeyPEM)
	privateKeyBytes := block.Bytes

	// // Parse private key
	// privateKey, err := x509.ParseECPrivateKey(privateKeyBytes)
	// if err != nil {
	// 	// Try PKCS8 format
	// 	key, err := x509.ParsePKCS8PrivateKey(privateKeyBytes)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to parse private key: %w", err)
	// 	}
	// 	var ok bool
	// 	privateKey, ok = key.(*ecdsa.PrivateKey)
	// 	if !ok {
	// 		return nil, errors.New("key is not an ECDSA private key")
	// 	}
	// }

	privateKey, err := x509.ParseECPrivateKey(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &JWTManager{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		issuer:     issuer,
		expiry:     time.Duration(expiryHours) * time.Hour,
	}, nil
}

// GenerateToken creates a new JWT token for a user
func (j *JWTManager) GenerateToken(userID uint, email string, tenantSchema string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Subject:   fmt.Sprintf("%d", userID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.expiry)),
			NotBefore: jwt.NewNumericDate(now),
		},
		UserID:       userID,
		Email:        email,
		TenantSchema: tenantSchema,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES512, claims)
	res, err := token.SignedString(j.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	return res, nil
}

// ValidateToken parses and validates a JWT token
func (j *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.publicKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshToken generates a new token with extended expiry
func (j *JWTManager) RefreshToken(claims *Claims) (string, error) {
	return j.GenerateToken(claims.UserID, claims.Email, claims.TenantSchema)
}

// JWTAuthMiddleware returns a Gin middleware for JWT authentication
func JWTAuthMiddleware(jwtManager *JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Extract Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			slog.Warn("Invalid authorization header format")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := jwtManager.ValidateToken(tokenString)
		if err != nil {
			slog.Warn("JWT validation failed", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		slog.Debug("JWT validated", "user_id", claims.UserID, "email", claims.Email, "tenantSchema", claims.TenantSchema)

		// Set claims in context for downstream handlers
		c.Set("claims", claims)
		c.Set("userId", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("tenantSchema", claims.TenantSchema)

		c.Next()
	}
}

// OptionalJWTAuthMiddleware is like JWTAuthMiddleware but doesn't fail if no token provided
func OptionalJWTAuthMiddleware(jwtManager *JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.Next()
			return
		}

		tokenString := parts[1]
		claims, err := jwtManager.ValidateToken(tokenString)
		if err != nil {
			c.Next()
			return
		}

		c.Set("claims", claims)
		c.Set("userId", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("tenantSchema", claims.TenantSchema)

		c.Next()
	}
}

// GetClaimsFromContext retrieves JWT claims from the Gin context
func GetClaimsFromContext(c *gin.Context) (*Claims, bool) {
	claims, exists := c.Get("claims")
	if !exists {
		return nil, false
	}
	return claims.(*Claims), true
}

// GetUserIDFromContext retrieves user ID from the Gin context
func GetUserIDFromContext(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("userId")
	if !exists {
		return 0, false
	}
	return userID.(uint), true
}

// GetTenantSchemaFromContext retrieves tenant schema from the Gin context
func GetTenantSchemaFromContext(c *gin.Context) (string, bool) {
	schema, exists := c.Get("tenantSchema")
	if !exists {
		return "", false
	}
	return schema.(string), true
}

// NewJWTManagerFromEnv creates a JWTManager from environment variables
func NewJWTManagerFromEnv() (*JWTManager, error) {
	privateKeyStr := os.Getenv("JWT_PRIVATE_KEY")
	privateKey, err := base64.StdEncoding.DecodeString(privateKeyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT private key from base64: %w", err)
	}

	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "awning-backend"
	}
	return NewJWTManager(string(privateKey), issuer, 24) // 24 hour expiry
}
