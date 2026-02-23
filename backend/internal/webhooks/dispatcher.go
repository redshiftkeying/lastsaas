package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/events"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Dispatcher listens for events and fires matching webhooks.
type Dispatcher struct {
	db     *db.MongoDB
	client *http.Client
}

func NewDispatcher(database *db.MongoDB) *Dispatcher {
	return &Dispatcher{
		db: database,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Emit implements events.Emitter — it checks for matching webhooks and fires them.
func (d *Dispatcher) Emit(event events.Event) {
	eventType := mapEventType(event.Type)
	if eventType == "" {
		return
	}

	go d.dispatch(eventType, event)
}

// mapEventType converts from events.EventType to models.WebhookEventType.
func mapEventType(et events.EventType) models.WebhookEventType {
	switch et {
	// Tier 1: Billing
	case events.EventSubscriptionActivated:
		return models.WebhookEventSubscriptionActivated
	case events.EventSubscriptionCanceled:
		return models.WebhookEventSubscriptionCanceled
	case events.EventPaymentReceived:
		return models.WebhookEventPaymentReceived
	case events.EventPaymentFailed:
		return models.WebhookEventPaymentFailed
	// Tier 2: Team lifecycle
	case events.EventMemberInvited:
		return models.WebhookEventMemberInvited
	case events.EventMemberJoined:
		return models.WebhookEventMemberJoined
	case events.EventMemberRemoved:
		return models.WebhookEventMemberRemoved
	case events.EventMemberRoleChanged:
		return models.WebhookEventMemberRoleChanged
	case events.EventOwnershipTransferred:
		return models.WebhookEventOwnershipTransferred
	// Tier 3: User lifecycle
	case events.EventUserRegistered:
		return models.WebhookEventUserRegistered
	case events.EventUserVerified:
		return models.WebhookEventUserVerified
	case events.EventUserDeactivated:
		return models.WebhookEventUserDeactivated
	// Tier 4: Credits & billing details
	case events.EventCreditsPurchased:
		return models.WebhookEventCreditsPurchased
	case events.EventPlanChanged:
		return models.WebhookEventPlanChanged
	case events.EventTenantCreated:
		return models.WebhookEventTenantCreated
	case events.EventTenantDeactivated:
		return models.WebhookEventTenantDeactivated
	// Tier 5: Audit & security
	case events.EventUserDeleted:
		return models.WebhookEventUserDeleted
	case events.EventTenantDeleted:
		return models.WebhookEventTenantDeleted
	case events.EventAPIKeyCreated:
		return models.WebhookEventAPIKeyCreated
	case events.EventAPIKeyRevoked:
		return models.WebhookEventAPIKeyRevoked
	default:
		return ""
	}
}

func (d *Dispatcher) dispatch(eventType models.WebhookEventType, event events.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := d.db.Webhooks().Find(ctx, bson.M{
		"events":   eventType,
		"isActive": true,
	})
	if err != nil {
		log.Printf("webhooks: failed to query webhooks for %s: %v", eventType, err)
		return
	}
	defer cursor.Close(ctx)

	var hooks []models.Webhook
	if err := cursor.All(ctx, &hooks); err != nil {
		log.Printf("webhooks: failed to decode webhooks: %v", err)
		return
	}

	for _, hook := range hooks {
		d.deliver(ctx, hook, eventType, event)
	}
}

// deliver sends a single webhook and records the delivery.
func (d *Dispatcher) deliver(ctx context.Context, hook models.Webhook, eventType models.WebhookEventType, event events.Event) {
	payload := map[string]interface{}{
		"event":     string(eventType),
		"timestamp": event.Timestamp.UTC().Format(time.RFC3339),
		"data":      event.Data,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhooks: failed to create request for %s: %v", hook.Name, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", string(eventType))

	// Sign with HMAC-SHA256
	if hook.Secret != "" {
		sig := computeSignature(body, hook.Secret)
		req.Header.Set("X-Webhook-Signature", sig)
	}

	start := time.Now()
	resp, err := d.client.Do(req)
	duration := time.Since(start).Milliseconds()

	delivery := models.WebhookDelivery{
		ID:        primitive.NewObjectID(),
		WebhookID: hook.ID,
		EventType: eventType,
		Payload:   string(body),
		Duration:  duration,
		CreatedAt: time.Now(),
	}

	if err != nil {
		delivery.Success = false
		delivery.ResponseCode = 0
		delivery.ResponseBody = err.Error()
	} else {
		defer resp.Body.Close()
		// Read limited response body for logging
		var respBuf bytes.Buffer
		respBuf.ReadFrom(http.MaxBytesReader(nil, resp.Body, 4096))
		delivery.ResponseCode = resp.StatusCode
		delivery.ResponseBody = respBuf.String()
		delivery.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	if _, err := d.db.WebhookDeliveries().InsertOne(ctx, delivery); err != nil {
		log.Printf("webhooks: failed to record delivery for %s: %v", hook.Name, err)
	}

	if !delivery.Success {
		log.Printf("webhooks: delivery to %s failed (status: %d)", hook.Name, delivery.ResponseCode)
	}
}

// computeSignature generates an HMAC-SHA256 hex digest.
func computeSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}

// DeliverTest sends a test event to a specific webhook and returns the delivery record.
func (d *Dispatcher) DeliverTest(ctx context.Context, hook models.Webhook) models.WebhookDelivery {
	testEvent := events.Event{
		Type:      events.EventTenantCreated,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tenantId":   "test_" + primitive.NewObjectID().Hex()[:8],
			"tenantName": "Test Tenant",
			"userId":     "test_" + primitive.NewObjectID().Hex()[:8],
			"test":       true,
		},
	}

	payload := map[string]interface{}{
		"event":     string(models.WebhookEventTenantCreated),
		"timestamp": testEvent.Timestamp.UTC().Format(time.RFC3339),
		"data":      testEvent.Data,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		return models.WebhookDelivery{
			ID:           primitive.NewObjectID(),
			WebhookID:    hook.ID,
			EventType:    models.WebhookEventTenantCreated,
			Payload:      string(body),
			ResponseCode: 0,
			ResponseBody: err.Error(),
			Success:      false,
			CreatedAt:    time.Now(),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", string(models.WebhookEventTenantCreated))
	req.Header.Set("X-Webhook-Test", "true")

	if hook.Secret != "" {
		sig := computeSignature(body, hook.Secret)
		req.Header.Set("X-Webhook-Signature", sig)
	}

	start := time.Now()
	resp, err := d.client.Do(req)
	duration := time.Since(start).Milliseconds()

	delivery := models.WebhookDelivery{
		ID:        primitive.NewObjectID(),
		WebhookID: hook.ID,
		EventType: models.WebhookEventTenantCreated,
		Payload:   string(body),
		Duration:  duration,
		CreatedAt: time.Now(),
	}

	if err != nil {
		delivery.Success = false
		delivery.ResponseCode = 0
		delivery.ResponseBody = err.Error()
	} else {
		defer resp.Body.Close()
		var respBuf bytes.Buffer
		respBuf.ReadFrom(http.MaxBytesReader(nil, resp.Body, 4096))
		delivery.ResponseCode = resp.StatusCode
		delivery.ResponseBody = respBuf.String()
		delivery.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	if _, err := d.db.WebhookDeliveries().InsertOne(ctx, delivery); err != nil {
		log.Printf("webhooks: failed to record test delivery for %s: %v", hook.Name, err)
	}

	return delivery
}
