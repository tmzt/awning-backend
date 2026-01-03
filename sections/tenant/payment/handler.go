package payment

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"awning-backend/common"
	"awning-backend/sections"
	"awning-backend/sections/common/auth"
	"awning-backend/sections/models"
	"awning-backend/services"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
)

// Handler handles payment-related requests
type Handler struct {
	logger    *slog.Logger
	deps      *sections.Dependencies
	stripeSvc *services.StripeService
}

// NewHandler creates a new payment handler
func NewHandler(deps *sections.Dependencies, stripeSvc *services.StripeService) *Handler {
	return &Handler{
		logger:    slog.With("handler", "PaymentHandler"),
		deps:      deps,
		stripeSvc: stripeSvc,
	}
}

type CreatePlanPaymentRequest struct {
	PlanID    string            `json:"planId" binding:"required"`
	PayDomain bool              `json:"payDomain,omitempty"`
	Currency  string            `json:"currency" binding:"required"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// CreateCheckoutSessionRequest represents a checkout session creation request
type CreateCheckoutSessionRequest struct {
	Mode        string            `json:"mode" binding:"required,oneof=payment subscription"` // payment or subscription
	PriceID     string            `json:"priceId,omitempty"`                                  // For subscriptions
	Amount      int64             `json:"amount,omitempty"`                                   // For one-time payments (in cents)
	Currency    string            `json:"currency,omitempty"`                                 // For one-time payments
	Description string            `json:"description,omitempty"`                              // For one-time payments
	Metadata    map[string]string `json:"metadata,omitempty"`
	SuccessURL  string            `json:"successUrl,omitempty"`
	CancelURL   string            `json:"cancelUrl,omitempty"`
}

// CheckoutSessionResponse represents the response containing session URL
type CheckoutSessionResponse struct {
	SessionID    string `json:"sessionId"`
	SessionURL   string `json:"sessionUrl"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

type PaymentIntentResponse struct {
	PaymentIntentId string `json:"paymentIntentId"`
	ClientSecret    string `json:"clientSecret"`
}

func (h *Handler) CreatePaymentIntentForPlan(c *gin.Context) {
	var req CreatePlanPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from context
	claims, ok := auth.GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get user details
	var user models.User
	if err := h.deps.DB.DB.First(&user, claims.UserID).Error; err != nil {
		h.logger.Error("Failed to get user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}

	// Get or create Stripe customer
	customerName := user.FirstName + " " + user.LastName
	metadata := map[string]string{
		"user_id": fmt.Sprintf("%d", user.ID),
		"email":   user.Email,
	}

	customer, err := h.stripeSvc.GetOrCreateCustomer(c.Request.Context(), user.Email, customerName, metadata)
	if err != nil {
		h.logger.Error("Failed to get or create customer", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create customer"})
		return
	}

	customerId := customer.ID

	// Create payment intent for the plan

	pi, err := h.stripeSvc.CreatePaymentIntentForPlan(c.Request.Context(), req.PlanID, customerId)
	if err != nil {
		h.logger.Error("Failed to create payment intent", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create payment intent"})
		return
	}

	h.logger.Info("Created payment intent for plan", "plan_id", req.PlanID, "payment_intent_id", pi.ID)

	data := &PaymentIntentResponse{
		PaymentIntentId: pi.ID,
		ClientSecret:    pi.ClientSecret,
	}

	c.JSON(http.StatusOK, common.ApiResponse[PaymentIntentResponse]{
		Data:    *data,
		Success: true,
	})
}

// CreateCheckoutSession creates a Stripe checkout session for a subscription plan

func (h *Handler) CreateCheckoutSessionForPlan(c *gin.Context) {
	var req CreatePlanPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from context
	claims, ok := auth.GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get user details
	var user models.User
	if err := h.deps.DB.DB.First(&user, claims.UserID).Error; err != nil {
		h.logger.Error("Failed to get user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}

	// Get or create Stripe customer
	customerName := user.FirstName + " " + user.LastName
	metadata := map[string]string{
		"user_id": fmt.Sprintf("%d", user.ID),
		"email":   user.Email,
	}

	customer, err := h.stripeSvc.GetOrCreateCustomer(c.Request.Context(), user.Email, customerName, metadata)
	if err != nil {
		h.logger.Error("Failed to get or create customer", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create customer"})
		return
	}

	// Add tenant schema to metadata if available
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if tenantSchema, ok := auth.GetTenantSchemaFromContext(c); ok {
		req.Metadata["tenant_schema"] = tenantSchema
	}
	req.Metadata["user_id"] = fmt.Sprintf("%d", user.ID)

	// Create checkout session

	session, err := h.stripeSvc.CreateCheckoutSessionForPlan(c.Request.Context(), user.Email, customer.ID, req.PlanID, req.Metadata)
	if err != nil {
		h.logger.Error("Failed to create checkout session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	h.logger.Info("Created checkout session for plan", "plan_id", req.PlanID, "session_id", session.ID, "client_secret", session.ClientSecret)

	data := &CheckoutSessionResponse{
		SessionID:    session.ID,
		SessionURL:   session.URL,
		ClientSecret: session.ClientSecret,
	}

	if pi := session.PaymentIntent; pi != nil {
		h.logger.Info("Payment Intent created", "payment_intent_id", pi.ID, "amount", pi.Amount, "currency", pi.Currency)
		data.ClientSecret = pi.ClientSecret
	}

	c.JSON(http.StatusOK, common.ApiResponse[CheckoutSessionResponse]{
		Data:    *data,
		Success: true,
	})
}

// CreateCheckoutSession creates a Stripe checkout session
func (h *Handler) CreateCheckoutSession(c *gin.Context) {
	var req CreateCheckoutSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user from context
	claims, ok := auth.GetClaimsFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get user details
	var user models.User
	if err := h.deps.DB.DB.First(&user, claims.UserID).Error; err != nil {
		h.logger.Error("Failed to get user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user"})
		return
	}

	// Get or create Stripe customer
	customerName := user.FirstName + " " + user.LastName
	metadata := map[string]string{
		"user_id": fmt.Sprintf("%d", user.ID),
		"email":   user.Email,
	}

	customer, err := h.stripeSvc.GetOrCreateCustomer(c.Request.Context(), user.Email, customerName, metadata)
	if err != nil {
		h.logger.Error("Failed to get or create customer", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create customer"})
		return
	}

	// Add tenant schema to metadata if available
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	if tenantSchema, ok := auth.GetTenantSchemaFromContext(c); ok {
		req.Metadata["tenant_schema"] = tenantSchema
	}
	req.Metadata["user_id"] = fmt.Sprintf("%d", user.ID)

	// Create checkout session
	sessionParams := &services.CheckoutSessionParams{
		CustomerID:  customer.ID,
		Mode:        req.Mode,
		PriceID:     req.PriceID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Description: req.Description,
		Metadata:    req.Metadata,
		SuccessURL:  req.SuccessURL,
		CancelURL:   req.CancelURL,
	}

	stripeSessionParams := &stripe.CheckoutSessionParams{
		UIMode: stripe.String("embedded"),
	}

	session, err := h.stripeSvc.CreateCheckoutSession(c.Request.Context(), sessionParams, stripeSessionParams)
	if err != nil {
		h.logger.Error("Failed to create checkout session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create checkout session"})
		return
	}

	data := &CheckoutSessionResponse{
		SessionID:  session.ID,
		SessionURL: session.URL,
	}

	if pi := session.PaymentIntent; pi != nil {
		h.logger.Info("Payment Intent created", "payment_intent_id", pi.ID, "amount", pi.Amount, "currency", pi.Currency)
		data.ClientSecret = pi.ClientSecret
	}

	c.JSON(http.StatusOK, common.ApiResponse[CheckoutSessionResponse]{
		Data:    *data,
		Success: true,
	})
}

// HandleWebhook processes Stripe webhook events
func (h *Handler) HandleWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.logger.Error("Failed to read webhook body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	event, err := h.stripeSvc.ConstructWebhookEvent(payload, signature)
	if err != nil {
		h.logger.Error("Failed to verify webhook", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		return
	}

	// Handle different event types
	switch event.Type {
	case "checkout.session.completed":
		h.handleCheckoutSessionCompleted(event)
	case "payment_intent.succeeded":
		h.handlePaymentIntentSucceeded(event)
	case "payment_intent.payment_failed":
		h.handlePaymentIntentFailed(event)
	case "customer.subscription.created":
		h.handleSubscriptionCreated(event)
	case "customer.subscription.updated":
		h.handleSubscriptionUpdated(event)
	case "customer.subscription.deleted":
		h.handleSubscriptionDeleted(event)
	case "invoice.paid":
		h.handleInvoicePaid(event)
	case "invoice.payment_failed":
		h.handleInvoicePaymentFailed(event)
	default:
		h.logger.Info("Unhandled webhook event type", "type", event.Type)
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (h *Handler) handleCheckoutSessionCompleted(event stripe.Event) {
	var session stripe.CheckoutSession
	if err := h.stripeSvc.ParseWebhookData(event.Data, &session); err != nil {
		h.logger.Error("Failed to parse checkout session", "error", err)
		return
	}

	h.logger.Info("Checkout session completed", "session_id", session.ID, "mode", session.Mode)

	// Handle based on mode
	if session.Mode == "payment" {
		h.handleOneTimePayment(&session)
	} else if session.Mode == "subscription" {
		h.handleSubscriptionCheckout(&session)
	}
}

func (h *Handler) handleOneTimePayment(session *stripe.CheckoutSession) {
	// Extract metadata
	tenantSchema := session.Metadata["tenant_schema"]
	userIDStr := session.Metadata["user_id"]

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		h.logger.Error("Failed to parse user ID", "error", err, "user_id_str", userIDStr)
		return
	}

	payment := models.Payment{
		TenantSchema:          tenantSchema,
		UserID:                uint(userID),
		StripePaymentIntentID: session.PaymentIntent.ID,
		StripeCustomerID:      session.Customer.ID,
		Amount:                session.AmountTotal,
		Currency:              string(session.Currency),
		Status:                "succeeded",
		Description:           "One-time payment",
		PaymentMethod:         "card",
	}

	now := time.Now()
	payment.PaidAt = &now

	if metadata, err := json.Marshal(session.Metadata); err == nil {
		payment.Metadata = string(metadata)
	}

	if err := h.deps.DB.DB.Create(&payment).Error; err != nil {
		h.logger.Error("Failed to create payment record", "error", err)
		return
	}

	h.logger.Info("One-time payment recorded", "payment_id", payment.ID, "amount", payment.Amount)
}

func (h *Handler) handleSubscriptionCheckout(session *stripe.CheckoutSession) {
	// Subscription details will be handled in subscription.created event
	h.logger.Info("Subscription checkout completed", "session_id", session.ID, "subscription_id", session.Subscription.ID)
}

func (h *Handler) handlePaymentIntentSucceeded(event stripe.Event) {
	var paymentIntent stripe.PaymentIntent
	if err := h.stripeSvc.ParseWebhookData(event.Data, &paymentIntent); err != nil {
		h.logger.Error("Failed to parse payment intent", "error", err)
		return
	}

	// Update payment status
	if err := h.deps.DB.DB.Model(&models.Payment{}).
		Where("stripe_payment_intent_id = ?", paymentIntent.ID).
		Updates(map[string]interface{}{
			"status":  "succeeded",
			"paid_at": time.Now(),
		}).Error; err != nil {
		h.logger.Error("Failed to update payment", "error", err)
	}

	h.logger.Info("Payment succeeded", "payment_intent_id", paymentIntent.ID)
}

func (h *Handler) handlePaymentIntentFailed(event stripe.Event) {
	var paymentIntent stripe.PaymentIntent
	if err := h.stripeSvc.ParseWebhookData(event.Data, &paymentIntent); err != nil {
		h.logger.Error("Failed to parse payment intent", "error", err)
		return
	}

	// Update payment status
	if err := h.deps.DB.DB.Model(&models.Payment{}).
		Where("stripe_payment_intent_id = ?", paymentIntent.ID).
		Update("status", "failed").Error; err != nil {
		h.logger.Error("Failed to update payment", "error", err)
	}

	h.logger.Info("Payment failed", "payment_intent_id", paymentIntent.ID)
}

func (h *Handler) handleSubscriptionCreated(event stripe.Event) {
	var sub stripe.Subscription
	if err := h.stripeSvc.ParseWebhookData(event.Data, &sub); err != nil {
		h.logger.Error("Failed to parse subscription", "error", err)
		return
	}

	// Extract metadata (should be set during checkout)
	tenantSchema := sub.Metadata["tenant_schema"]
	userIDStr := sub.Metadata["user_id"]

	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		h.logger.Error("Failed to parse user ID", "error", err, "user_id_str", userIDStr)
		return
	}

	subscription := models.Subscription{
		TenantSchema:         tenantSchema,
		UserID:               uint(userID),
		StripeSubscriptionID: sub.ID,
		StripeCustomerID:     sub.Customer.ID,
		StripePriceID:        sub.Items.Data[0].Price.ID,
		StripeProductID:      sub.Items.Data[0].Price.Product.ID,
		Status:               string(sub.Status),
		PlanName:             sub.Items.Data[0].Price.Nickname,
		Amount:               sub.Items.Data[0].Price.UnitAmount,
		Currency:             string(sub.Items.Data[0].Price.Currency),
		Interval:             string(sub.Items.Data[0].Price.Recurring.Interval),
		IntervalCount:        int(sub.Items.Data[0].Price.Recurring.IntervalCount),
		CurrentPeriodStart:   time.Unix(sub.LatestInvoice.PeriodStart, 0),
		CurrentPeriodEnd:     time.Unix(sub.LatestInvoice.PeriodEnd, 0),
	}

	if sub.TrialStart != 0 {
		trialStart := time.Unix(sub.TrialStart, 0)
		subscription.TrialStart = &trialStart
	}
	if sub.TrialEnd != 0 {
		trialEnd := time.Unix(sub.TrialEnd, 0)
		subscription.TrialEnd = &trialEnd
	}

	if metadata, err := json.Marshal(sub.Metadata); err == nil {
		subscription.Metadata = string(metadata)
	}

	if err := h.deps.DB.DB.Create(&subscription).Error; err != nil {
		h.logger.Error("Failed to create subscription record", "error", err)
		return
	}

	h.logger.Info("Subscription created", "subscription_id", subscription.ID, "stripe_id", sub.ID)
}

func (h *Handler) handleSubscriptionUpdated(event stripe.Event) {
	var sub stripe.Subscription
	if err := h.stripeSvc.ParseWebhookData(event.Data, &sub); err != nil {
		h.logger.Error("Failed to parse subscription", "error", err)
		return
	}

	updates := map[string]interface{}{
		"status":               string(sub.Status),
		"current_period_start": time.Unix(sub.LatestInvoice.PeriodStart, 0),
		"current_period_end":   time.Unix(sub.LatestInvoice.PeriodEnd, 0),
		"cancel_at_period_end": sub.CancelAtPeriodEnd,
	}

	if sub.CanceledAt != 0 {
		canceledAt := time.Unix(sub.CanceledAt, 0)
		updates["canceled_at"] = &canceledAt
	}

	if err := h.deps.DB.DB.Model(&models.Subscription{}).
		Where("stripe_subscription_id = ?", sub.ID).
		Updates(updates).Error; err != nil {
		h.logger.Error("Failed to update subscription", "error", err)
	}

	h.logger.Info("Subscription updated", "stripe_id", sub.ID, "status", sub.Status)
}

func (h *Handler) handleSubscriptionDeleted(event stripe.Event) {
	var sub stripe.Subscription
	if err := h.stripeSvc.ParseWebhookData(event.Data, &sub); err != nil {
		h.logger.Error("Failed to parse subscription", "error", err)
		return
	}

	if err := h.deps.DB.DB.Model(&models.Subscription{}).
		Where("stripe_subscription_id = ?", sub.ID).
		Update("status", "canceled").Error; err != nil {
		h.logger.Error("Failed to update subscription", "error", err)
	}

	h.logger.Info("Subscription deleted", "stripe_id", sub.ID)
}

func (h *Handler) handleInvoicePaid(event stripe.Event) {
	var invoice stripe.Invoice
	if err := h.stripeSvc.ParseWebhookData(event.Data, &invoice); err != nil {
		h.logger.Error("Failed to parse invoice", "error", err)
		return
	}

	h.logger.Info("Invoice paid", "invoice_id", invoice.ID)
}

func (h *Handler) handleInvoicePaymentFailed(event stripe.Event) {
	var invoice stripe.Invoice
	if err := h.stripeSvc.ParseWebhookData(event.Data, &invoice); err != nil {
		h.logger.Error("Failed to parse invoice", "error", err)
		return
	}

	h.logger.Info("Invoice payment failed", "invoice_id", invoice.ID)
}
