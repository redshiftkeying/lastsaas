package stripe

import (
	"context"
	"fmt"
	"time"

	"lastsaas/internal/apicounter"
	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	stripe "github.com/stripe/stripe-go/v82"
	portalsession "github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/price"
	"github.com/stripe/stripe-go/v82/product"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"
)

type Service struct {
	secretKey      string
	PublishableKey string
	webhookSecret  string
	db             *db.MongoDB
	frontendURL    string
}

func New(secretKey, publishableKey, webhookSecret string, database *db.MongoDB, frontendURL string) *Service {
	stripe.Key = secretKey
	return &Service{
		secretKey:      secretKey,
		PublishableKey: publishableKey,
		webhookSecret:  webhookSecret,
		db:             database,
		frontendURL:    frontendURL,
	}
}

// GetOrCreateCustomer finds or creates a Stripe customer for the given tenant.
func (s *Service) GetOrCreateCustomer(ctx context.Context, tenant *models.Tenant, userEmail string) (string, error) {
	if tenant.StripeCustomerID != "" {
		return tenant.StripeCustomerID, nil
	}

	params := &stripe.CustomerParams{
		Email: stripe.String(userEmail),
		Name:  stripe.String(tenant.Name),
		Metadata: map[string]string{
			"tenantId": tenant.ID.Hex(),
		},
	}
	c, err := customer.New(params)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return "", fmt.Errorf("stripe customer create: %w", err)
	}

	_, err = s.db.Tenants().UpdateOne(ctx,
		bson.M{"_id": tenant.ID},
		bson.M{"$set": bson.M{"stripeCustomerId": c.ID, "updatedAt": time.Now()}},
	)
	if err != nil {
		return "", fmt.Errorf("save stripe customer id: %w", err)
	}

	return c.ID, nil
}

// GetOrCreatePrice finds an existing Stripe price mapping or creates a new Product + Price.
func (s *Service) GetOrCreatePrice(ctx context.Context, entityType string, entityID primitive.ObjectID, name string, amountCents int64, interval string) (string, error) {
	// Check existing mapping
	var mapping models.StripeMapping
	err := s.db.StripeMappings().FindOne(ctx, bson.M{
		"entityType": entityType,
		"entityId":   entityID,
	}).Decode(&mapping)
	if err == nil {
		return mapping.StripePriceID, nil
	}
	if err != mongo.ErrNoDocuments {
		return "", fmt.Errorf("lookup stripe mapping: %w", err)
	}

	// Create product
	prod, err := product.New(&stripe.ProductParams{
		Name: stripe.String(name),
		Metadata: map[string]string{
			"entityType": entityType,
			"entityId":   entityID.Hex(),
		},
	})
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return "", fmt.Errorf("stripe product create: %w", err)
	}

	// Create price
	priceParams := &stripe.PriceParams{
		Product:    stripe.String(prod.ID),
		Currency:   stripe.String("usd"),
		UnitAmount: stripe.Int64(amountCents),
	}
	if interval != "" {
		priceParams.Recurring = &stripe.PriceRecurringParams{
			Interval: stripe.String(interval),
		}
	}
	p, err := price.New(priceParams)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return "", fmt.Errorf("stripe price create: %w", err)
	}

	// Save mapping
	s.db.StripeMappings().InsertOne(ctx, models.StripeMapping{
		EntityType:      entityType,
		EntityID:        entityID,
		StripePriceID:   p.ID,
		StripeProductID: prod.ID,
		CreatedAt:       time.Now(),
	})

	return p.ID, nil
}

type CheckoutRequest struct {
	CustomerID      string
	PlanID          *primitive.ObjectID
	PlanName        string
	BundleID        *primitive.ObjectID
	BundleName      string
	AmountCents     int64
	BillingInterval string // "month" or "year"
	TenantID        string
	UserID          string
}

// CreateCheckoutSession creates a Stripe Checkout Session for a subscription or one-time payment.
func (s *Service) CreateCheckoutSession(ctx context.Context, req CheckoutRequest) (string, error) {
	var mode string
	var priceID string
	var err error

	metadata := map[string]string{
		"tenantId": req.TenantID,
		"userId":   req.UserID,
	}

	if req.PlanID != nil {
		mode = "subscription"
		entityType := "plan_" + req.BillingInterval
		priceID, err = s.GetOrCreatePrice(ctx, entityType, *req.PlanID, req.PlanName, req.AmountCents, req.BillingInterval)
		if err != nil {
			return "", err
		}
		metadata["planId"] = req.PlanID.Hex()
		metadata["billingInterval"] = req.BillingInterval
	} else if req.BundleID != nil {
		mode = "payment"
		priceID, err = s.GetOrCreatePrice(ctx, "bundle", *req.BundleID, req.BundleName, req.AmountCents, "")
		if err != nil {
			return "", err
		}
		metadata["bundleId"] = req.BundleID.Hex()
	} else {
		return "", fmt.Errorf("must specify planId or bundleId")
	}

	params := &stripe.CheckoutSessionParams{
		Customer:            stripe.String(req.CustomerID),
		Mode:                stripe.String(mode),
		SuccessURL:          stripe.String(s.frontendURL + "/billing/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:           stripe.String(s.frontendURL + "/billing/cancel"),
		AllowPromotionCodes: stripe.Bool(true),
		Metadata:            metadata,
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
	}

	session, err := checkoutsession.New(params)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return "", fmt.Errorf("stripe checkout create: %w", err)
	}

	return session.URL, nil
}

// CreateBillingPortalSession creates a Stripe Billing Portal session for the given customer.
func (s *Service) CreateBillingPortalSession(ctx context.Context, customerID string) (string, error) {
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(s.frontendURL + "/settings"),
	}
	session, err := portalsession.New(params)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return "", fmt.Errorf("stripe portal create: %w", err)
	}
	return session.URL, nil
}

// CancelSubscriptionAtPeriodEnd marks a subscription to cancel at the end of the current period.
func (s *Service) CancelSubscriptionAtPeriodEnd(ctx context.Context, subscriptionID string) (*time.Time, error) {
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	}
	params.AddExpand("items")
	sub, err := subscription.Update(subscriptionID, params)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return nil, fmt.Errorf("stripe cancel subscription: %w", err)
	}
	// In v82, CurrentPeriodEnd is on SubscriptionItem, not Subscription
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		periodEnd := time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0)
		return &periodEnd, nil
	}
	// Fallback: use CancelAt if set
	if sub.CancelAt > 0 {
		t := time.Unix(sub.CancelAt, 0)
		return &t, nil
	}
	return nil, nil
}

// CancelSubscriptionImmediately cancels a subscription immediately.
func (s *Service) CancelSubscriptionImmediately(ctx context.Context, subscriptionID string) error {
	_, err := subscription.Cancel(subscriptionID, nil)
	apicounter.StripeAPICalls.Add(1)
	if err != nil {
		return fmt.Errorf("stripe cancel subscription: %w", err)
	}
	return nil
}

// ConstructEvent verifies a webhook signature and returns the parsed event.
func (s *Service) ConstructEvent(payload []byte, sigHeader string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, sigHeader, s.webhookSecret)
}

// NextInvoiceNumber atomically generates the next invoice number.
func (s *Service) NextInvoiceNumber(ctx context.Context) (string, error) {
	var result models.InvoiceCounter
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)
	err := s.db.Counters().FindOneAndUpdate(ctx,
		bson.M{"_id": "invoice_number"},
		bson.M{"$inc": bson.M{"value": 1}},
		opts,
	).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("generate invoice number: %w", err)
	}
	return fmt.Sprintf("INV-%06d", result.Value), nil
}

// GetCheckoutSession retrieves a checkout session by ID.
func (s *Service) GetCheckoutSession(ctx context.Context, sessionID string) (*stripe.CheckoutSession, error) {
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("subscription")
	s2, err := checkoutsession.Get(sessionID, params)
	apicounter.StripeAPICalls.Add(1)
	return s2, err
}
