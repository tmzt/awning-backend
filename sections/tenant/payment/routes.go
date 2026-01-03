package payment

import (
	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/services"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers payment routes
func RegisterRoutes(frontendRoutes, callbackRoutes *gin.RouterGroup, deps *sections.Dependencies, jwtManager *auth.JWTManager, stripeSvc *services.StripeService) {
	handler := NewHandler(deps, stripeSvc)

	// Protected routes for creating checkout sessions (requires authentication)
	payment := frontendRoutes.Group("/api/v1/payments")
	payment.Use(auth.JWTAuthMiddleware(jwtManager))
	{
		payment.POST("/plan", handler.CreatePaymentIntentForPlan)
		payment.POST("/checkout", handler.CreateCheckoutSession)
	}

	// Webhook routes (no authentication, verified via Stripe signature)
	webhooks := callbackRoutes.Group("/stripe")
	{
		webhooks.POST("/webhook", handler.HandleWebhook)
	}
}
