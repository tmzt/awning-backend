package services

import (
	"awning-backend/common"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/checkout/session"
	"github.com/stripe/stripe-go/v84/customer"
	"github.com/stripe/stripe-go/v84/paymentintent"
	"github.com/stripe/stripe-go/v84/subscription"
	"github.com/stripe/stripe-go/v84/webhook"
)

// StripeService handles Stripe API interactions
type StripeService struct {
	plans         []common.Plan
	secretKey     string
	webhookSecret string
	successURL    string
	cancelURL     string
	logger        *slog.Logger
}

// NewStripeService creates a new Stripe service
func NewStripeService(plans []common.Plan, secretKey, webhookSecret, successURL, cancelURL string) *StripeService {
	stripe.Key = secretKey

	return &StripeService{
		plans:         plans,
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		successURL:    successURL,
		cancelURL:     cancelURL,
		logger:        slog.With("service", "StripeService"),
	}
}

// CheckoutSessionParams represents parameters for creating a checkout session
type CheckoutSessionParams struct {
	CustomerEmail string
	CustomerID    string
	PriceID       string // For subscriptions
	Amount        int64  // For one-time payments (in cents)
	Currency      string // For one-time payments (e.g., "usd")
	Description   string // For one-time payments
	Mode          string // "payment" or "subscription"
	Metadata      map[string]string
	SuccessURL    string // Optional override
	CancelURL     string // Optional override
}

// CreateCheckoutSession creates a Stripe checkout session
func (s *StripeService) CreateCheckoutSession(ctx context.Context, params *CheckoutSessionParams, stripeSessionParams *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	sessionParams := &stripe.CheckoutSessionParams{
		Mode:     stripe.String(params.Mode),
		Metadata: params.Metadata,
	}

	// // Set success and cancel URLs
	// successURL := params.SuccessURL
	// if successURL == "" {
	// 	successURL = s.successURL
	// }
	// cancelURL := params.CancelURL
	// if cancelURL == "" {
	// 	cancelURL = s.cancelURL
	// }
	// sessionParams.SuccessURL = stripe.String(successURL)
	// sessionParams.CancelURL = stripe.String(cancelURL)

	sessionParams.RedirectOnCompletion = stripe.String("never")

	// Set customer
	if params.CustomerID != "" {
		sessionParams.Customer = stripe.String(params.CustomerID)
	} else if params.CustomerEmail != "" {
		sessionParams.CustomerEmail = stripe.String(params.CustomerEmail)
	}

	// Configure based on mode
	if params.Mode == "subscription" {
		if params.PriceID == "" {
			return nil, fmt.Errorf("priceID is required for subscription mode")
		}
		if len(sessionParams.LineItems) == 0 {

			sessionParams.LineItems = []*stripe.CheckoutSessionLineItemParams{
				{
					Price:    stripe.String(params.PriceID),
					Quantity: stripe.Int64(1),
				},
			}
		}
	} else if params.Mode == "payment" {
		if params.Amount <= 0 {
			return nil, fmt.Errorf("amount is required for payment mode")
		}
		if params.Currency == "" {
			params.Currency = "usd"
		}
		if len(sessionParams.LineItems) == 0 {

			sessionParams.LineItems = []*stripe.CheckoutSessionLineItemParams{
				{
					PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
						Currency: stripe.String(params.Currency),
						ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
							Name:        stripe.String(params.Description),
							Description: stripe.String(params.Description),
						},
						UnitAmount: stripe.Int64(params.Amount),
					},
					Quantity: stripe.Int64(1),
				},
			}
		}
	} else {
		return nil, fmt.Errorf("invalid mode: must be 'payment' or 'subscription'")
	}

	if stripeSessionParams != nil {
		// Merge additional session params
		if stripeSessionParams.UIMode != nil {
			sessionParams.UIMode = stripeSessionParams.UIMode
		}
		if stripeSessionParams.LineItems != nil {
			sessionParams.LineItems = stripeSessionParams.LineItems
		}
		// Add more fields as needed
	}

	sess, err := session.New(sessionParams)
	if err != nil {
		s.logger.Error("Failed to create checkout session", "error", err)
		return nil, fmt.Errorf("failed to create checkout session: %w", err)
	}

	s.logger.Info("Created checkout session", "session_id", sess.ID, "mode", params.Mode)
	return sess, nil
}

func (s *StripeService) CreateCheckoutSessionForPlan(ctx context.Context, customerEmail, customerID, planID string, metadata map[string]string) (*stripe.CheckoutSession, error) {
	var selectedPlan *common.Plan
	for _, plan := range s.plans {
		if plan.ID == planID {
			selectedPlan = &plan
			break
		}
	}
	if selectedPlan == nil {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}

	mode := "payment"
	if planID == "payMonthly" {
		mode = "subscription"
	}

	// priceID := fmt.Sprintf("awning-plan-%s", planID)

	priceID := selectedPlan.PriceId

	metadata["plan_id"] = selectedPlan.ID
	metadata["plan_name"] = selectedPlan.Name

	totalPrice := selectedPlan.PriceCents
	if selectedPlan.ChargeDomain {
		totalPrice += 1450 // Example domain charge
	}

	var lineItems []*stripe.CheckoutSessionLineItemParams
	lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
		Price:    stripe.String(priceID),
		Quantity: stripe.Int64(1),
	})
	if selectedPlan.ChargeDomain {
		domainPlan := common.GetPlan(s.plans, "domainOnly")
		if domainPlan == nil {
			return nil, fmt.Errorf("domainOnly plan not found")
		}

		lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
			Price:    stripe.String(domainPlan.PriceId),
			Quantity: stripe.Int64(1),
		})
	}

	params := &CheckoutSessionParams{
		CustomerEmail: customerEmail,
		CustomerID:    customerID,
		PriceID:       priceID,
		Mode:          mode,
		Amount:        totalPrice,
		Metadata:      metadata,
	}

	stripeSessionParams := &stripe.CheckoutSessionParams{
		UIMode:    stripe.String("embedded"),
		LineItems: lineItems,
	}

	return s.CreateCheckoutSession(ctx, params, stripeSessionParams)
}

func (s *StripeService) CreatePaymentIntentForPlan(ctx context.Context, planId, customerID string) (*stripe.PaymentIntent, error) {

	plan := common.GetPlan(s.plans, planId)
	if plan == nil {
		return nil, fmt.Errorf("plan not found: %s", planId)
	}

	amount := plan.PriceCents
	if plan.ChargeDomain {
		amount += 1450 // Example domain charge
	}

	description := fmt.Sprintf("Payment for plan: %s", plan.Name)
	if plan.ChargeDomain {
		description += " (including domain charge)"
	}

	metadata := map[string]string{
		"plan_id":   plan.ID,
		"plan_name": plan.Name,
	}

	currency := plan.Currency

	// return s.CreatePaymentIntent(ctx, int64(amount), plan.Currency, customerID, description, metadata)
	params := &stripe.PaymentIntentParams{
		Amount:      stripe.Int64(amount),
		Currency:    stripe.String(currency),
		Customer:    stripe.String(customerID),
		Description: stripe.String(description),
		Metadata:    metadata,
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		s.logger.Error("Failed to create payment intent", "error", err)
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	s.logger.Info("Created payment intent", "payment_intent_id", pi.ID, "amount", amount, "currency", currency)
	return pi, nil
}

// CreatePaymentIntent creates a Stripe payment intent
func (s *StripeService) CreatePaymentIntent(ctx context.Context, amount int64, currency, customerID, description string, metadata map[string]string) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:      stripe.Int64(amount),
		Currency:    stripe.String(currency),
		Customer:    stripe.String(customerID),
		Description: stripe.String(description),
		Metadata:    metadata,
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		s.logger.Error("Failed to create payment intent", "error", err)
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	s.logger.Info("Created payment intent", "payment_intent_id", pi.ID, "amount", amount, "currency", currency)
	return pi, nil
}

// GetOrCreateCustomer retrieves an existing customer or creates a new one
func (s *StripeService) GetOrCreateCustomer(ctx context.Context, email, name string, metadata map[string]string) (*stripe.Customer, error) {
	// Try to find existing customer by email
	searchParams := &stripe.CustomerSearchParams{
		SearchParams: stripe.SearchParams{
			Query: fmt.Sprintf("email:'%s'", email),
		},
	}
	iter := customer.Search(searchParams)

	if iter.Next() {
		cust := iter.Customer()
		s.logger.Info("Found existing Stripe customer", "customer_id", cust.ID, "email", email)
		return cust, nil
	}

	// Create new customer
	params := &stripe.CustomerParams{
		Email:    stripe.String(email),
		Name:     stripe.String(name),
		Metadata: metadata,
	}

	cust, err := customer.New(params)
	if err != nil {
		s.logger.Error("Failed to create Stripe customer", "error", err)
		return nil, fmt.Errorf("failed to create customer: %w", err)
	}

	s.logger.Info("Created new Stripe customer", "customer_id", cust.ID, "email", email)
	return cust, nil
}

// CancelSubscription cancels a subscription
func (s *StripeService) CancelSubscription(ctx context.Context, subscriptionID string, cancelAtPeriodEnd bool) (*stripe.Subscription, error) {
	var sub *stripe.Subscription
	var err error

	if cancelAtPeriodEnd {
		// Schedule cancellation at period end
		params := &stripe.SubscriptionParams{
			CancelAtPeriodEnd: stripe.Bool(true),
		}
		sub, err = subscription.Update(subscriptionID, params)
	} else {
		// Cancel immediately
		sub, err = subscription.Cancel(subscriptionID, nil)
	}

	if err != nil {
		s.logger.Error("Failed to cancel subscription", "error", err, "subscription_id", subscriptionID)
		return nil, fmt.Errorf("failed to cancel subscription: %w", err)
	}

	s.logger.Info("Canceled subscription", "subscription_id", subscriptionID, "cancel_at_period_end", cancelAtPeriodEnd)
	return sub, nil
}

// ConstructWebhookEvent constructs and validates a webhook event
func (s *StripeService) ConstructWebhookEvent(payload []byte, signature string) (stripe.Event, error) {
	options := &webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	}

	event, err := webhook.ConstructEventWithOptions(payload, signature, s.webhookSecret, *options)
	if err != nil {
		s.logger.Error("Failed to verify webhook signature", "error", err)
		return stripe.Event{}, fmt.Errorf("failed to verify webhook: %w", err)
	}

	s.logger.Debug("Webhook event verified", "type", event.Type, "id", event.ID)
	return event, nil
}

// ParseWebhookData parses webhook data into a target struct
func (s *StripeService) ParseWebhookData(data *stripe.EventData, target interface{}) error {
	if err := json.Unmarshal(data.Raw, target); err != nil {
		s.logger.Error("Failed to parse webhook data", "error", err)
		return fmt.Errorf("failed to parse webhook data: %w", err)
	}
	return nil
}
