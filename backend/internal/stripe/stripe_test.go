package stripe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"lastsaas/internal/models"
	"lastsaas/internal/testutil"

	gostripe "github.com/stripe/stripe-go/v82"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func setupMockStripe(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func()) {
	t.Helper()
	mock := httptest.NewServer(handler)

	// Configure stripe to use mock backend
	backend := gostripe.GetBackendWithConfig(gostripe.APIBackend, &gostripe.BackendConfig{
		URL: gostripe.String(mock.URL),
	})
	gostripe.SetBackend(gostripe.APIBackend, backend)

	return mock, func() {
		mock.Close()
	}
}

func TestNewService(t *testing.T) {
	svc := New("sk_test_123", "pk_test_123", "whsec_123", nil, "https://myapp.example.com")
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.PublishableKey != "pk_test_123" {
		t.Errorf("expected pk_test_123, got %q", svc.PublishableKey)
	}
	if svc.secretKey != "sk_test_123" {
		t.Errorf("expected sk_test_123, got %q", svc.secretKey)
	}
	if svc.webhookSecret != "whsec_123" {
		t.Errorf("expected whsec_123, got %q", svc.webhookSecret)
	}
	if svc.frontendURL != "https://myapp.example.com" {
		t.Errorf("expected https://myapp.example.com, got %q", svc.frontendURL)
	}
}

func TestInstanceIDFromURL(t *testing.T) {
	tests := []struct {
		name        string
		frontendURL string
		expected    string
	}{
		{"standard URL", "https://myapp.example.com", "myapp.example.com"},
		{"with port", "http://localhost:4280", "localhost"},
		{"with path", "https://app.example.com/admin", "app.example.com"},
		{"empty URL", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := New("sk", "pk", "whsec", nil, tt.frontendURL)
			if svc.InstanceID() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, svc.InstanceID())
			}
		})
	}
}

func TestConstructEventInvalidSignature(t *testing.T) {
	svc := New("sk_test", "pk_test", "whsec_test_secret", nil, "http://localhost")
	payload := []byte(`{"type":"checkout.session.completed"}`)

	_, err := svc.ConstructEvent(payload, "invalid-signature")
	if err == nil {
		t.Fatal("expected error for invalid webhook signature")
	}
}

func TestNextInvoiceNumber(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)

	svc := New("sk_test", "pk_test", "whsec_test", database, "http://localhost")

	inv1, err := svc.NextInvoiceNumber(t.Context())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if inv1 != "INV-000001" {
		t.Errorf("expected INV-000001, got %q", inv1)
	}

	inv2, err := svc.NextInvoiceNumber(t.Context())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if inv2 != "INV-000002" {
		t.Errorf("expected INV-000002, got %q", inv2)
	}
}

func TestGetOrCreateCustomerExisting(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)

	svc := New("sk_test", "pk_test", "whsec_test", database, "http://localhost")

	tenant := &models.Tenant{
		ID:               primitive.NewObjectID(),
		StripeCustomerID: "cus_existing123",
	}

	custID, err := svc.GetOrCreateCustomer(t.Context(), tenant, "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if custID != "cus_existing123" {
		t.Errorf("expected existing customer ID, got %q", custID)
	}
}

func TestGetOrCreateCustomerNew(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)

	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "cus_new123",
			"object": "customer",
			"email":  "user@example.com",
		})
	})
	defer mockCleanup()
	_ = mock

	owner := testutil.CreateTestUser(t, database, "owner@example.com", "password123", "Owner")
	tenant := testutil.CreateTestTenant(t, database, "New Tenant", owner.ID, false)

	svc := New("sk_test", "pk_test", "whsec_test", database, "http://localhost:4280")

	custID, err := svc.GetOrCreateCustomer(t.Context(), tenant, "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if custID != "cus_new123" {
		t.Errorf("expected cus_new123, got %q", custID)
	}
}

func TestCreateCheckoutSessionMissingIDs(t *testing.T) {
	svc := New("sk_test", "pk_test", "whsec_test", nil, "http://localhost")

	_, err := svc.CreateCheckoutSession(t.Context(), CheckoutRequest{
		CustomerID: "cus_123",
		TenantID:   "tenant_123",
		UserID:     "user_123",
		// Neither PlanID nor BundleID set
	})
	if err == nil {
		t.Fatal("expected error for missing planId/bundleId")
	}
}

func TestCreateCheckoutSessionSubscription(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)

	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/products":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "prod_test123",
				"object": "product",
			})
		case r.URL.Path == "/v1/prices":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "price_test123",
				"object": "price",
			})
		case r.URL.Path == "/v1/checkout/sessions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "cs_test123",
				"object": "checkout.session",
				"url":    "https://checkout.stripe.com/pay/cs_test123",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer mockCleanup()
	_ = mock

	planID := primitive.NewObjectID()
	svc := New("sk_test", "pk_test", "whsec_test", database, "http://localhost:4280")

	url, err := svc.CreateCheckoutSession(t.Context(), CheckoutRequest{
		CustomerID:      "cus_123",
		PlanID:          &planID,
		PlanName:        "Pro Plan",
		AmountCents:     1999,
		BillingInterval: "month",
		TenantID:        "tenant_123",
		UserID:          "user_123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Error("expected non-empty checkout URL")
	}
}

func TestCreateCheckoutSessionOneTimePayment(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)

	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/products":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "prod_bundle",
				"object": "product",
			})
		case r.URL.Path == "/v1/prices":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "price_bundle",
				"object": "price",
			})
		case r.URL.Path == "/v1/checkout/sessions":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "cs_bundle",
				"object": "checkout.session",
				"url":    "https://checkout.stripe.com/pay/cs_bundle",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer mockCleanup()
	_ = mock

	bundleID := primitive.NewObjectID()
	svc := New("sk_test", "pk_test", "whsec_test", database, "http://localhost:4280")

	url, err := svc.CreateCheckoutSession(t.Context(), CheckoutRequest{
		CustomerID:  "cus_123",
		BundleID:    &bundleID,
		BundleName:  "Credit Bundle",
		AmountCents: 999,
		TenantID:    "tenant_123",
		UserID:      "user_123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Error("expected non-empty checkout URL")
	}
}

func TestCreateCheckoutSessionWithCustomLineItems(t *testing.T) {
	database, cleanup := testutil.MustConnectTestDB(t)
	defer cleanup()
	testutil.CleanupCollections(t, database)

	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/checkout/sessions" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "cs_custom",
				"object": "checkout.session",
				"url":    "https://checkout.stripe.com/pay/cs_custom",
			})
		}
	})
	defer mockCleanup()
	_ = mock

	planID := primitive.NewObjectID()
	svc := New("sk_test", "pk_test", "whsec_test", database, "http://localhost:4280")

	url, err := svc.CreateCheckoutSession(t.Context(), CheckoutRequest{
		CustomerID:      "cus_123",
		PlanID:          &planID,
		PlanName:        "Enterprise",
		BillingInterval: "year",
		TenantID:        "tenant_123",
		UserID:          "user_123",
		SeatQuantity:    10,
		TrialDays:       14,
		AutomaticTax:    true,
		CustomLineItems: []CheckoutLineItem{
			{PriceID: "price_base", Quantity: 1},
			{PriceID: "price_seat", Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Error("expected non-empty checkout URL")
	}
}

func TestCreateBillingPortalSession(t *testing.T) {
	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "bps_test",
			"object": "billing_portal.session",
			"url":    "https://billing.stripe.com/session/bps_test",
		})
	})
	defer mockCleanup()
	_ = mock

	svc := New("sk_test", "pk_test", "whsec_test", nil, "http://localhost:4280")

	url, err := svc.CreateBillingPortalSession(t.Context(), "cus_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Error("expected non-empty portal URL")
	}
}

func TestCancelSubscriptionAtPeriodEnd(t *testing.T) {
	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                "sub_123",
			"object":            "subscription",
			"cancel_at_period_end": true,
			"cancel_at":         1735689600,
			"items": map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{
						"id":                 "si_123",
						"current_period_end": 1735689600,
					},
				},
			},
		})
	})
	defer mockCleanup()
	_ = mock

	svc := New("sk_test", "pk_test", "whsec_test", nil, "http://localhost")

	periodEnd, err := svc.CancelSubscriptionAtPeriodEnd(t.Context(), "sub_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if periodEnd == nil {
		t.Fatal("expected non-nil period end")
	}
}

func TestCancelSubscriptionImmediately(t *testing.T) {
	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "sub_123",
			"object": "subscription",
			"status": "canceled",
		})
	})
	defer mockCleanup()
	_ = mock

	svc := New("sk_test", "pk_test", "whsec_test", nil, "http://localhost")

	err := svc.CancelSubscriptionImmediately(t.Context(), "sub_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCheckoutSession(t *testing.T) {
	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "cs_test123",
			"object": "checkout.session",
			"mode":   "subscription",
		})
	})
	defer mockCleanup()
	_ = mock

	svc := New("sk_test", "pk_test", "whsec_test", nil, "http://localhost")

	session, err := svc.GetCheckoutSession(t.Context(), "cs_test123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID != "cs_test123" {
		t.Errorf("expected cs_test123, got %q", session.ID)
	}
}

func TestGetSubscription(t *testing.T) {
	mock, mockCleanup := setupMockStripe(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "sub_test123",
			"object": "subscription",
			"status": "active",
			"items": map[string]interface{}{
				"object": "list",
				"data":   []map[string]interface{}{},
			},
		})
	})
	defer mockCleanup()
	_ = mock

	svc := New("sk_test", "pk_test", "whsec_test", nil, "http://localhost")

	sub, err := svc.GetSubscription(t.Context(), "sub_test123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.ID != "sub_test123" {
		t.Errorf("expected sub_test123, got %q", sub.ID)
	}
}
