package health

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"lastsaas/internal/models"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/balance"
	"go.mongodb.org/mongo-driver/mongo"
)

// IntegrationChecker performs a health check for a third-party service.
// Returns nil if healthy, or an error describing the problem.
type IntegrationChecker func(ctx context.Context) error

type integrationEntry struct {
	name    string
	checker IntegrationChecker // nil = not configured
}

// RegisterIntegration adds a named integration to be health-checked.
// Pass checker=nil to indicate the service is not configured.
func (s *Service) RegisterIntegration(name string, checker IntegrationChecker) {
	status := models.IntegrationNotConfigured
	msg := "Not configured"
	if checker != nil {
		status = models.IntegrationHealthy
		msg = "Pending first check"
	}
	s.intMu.Lock()
	s.integrations = append(s.integrations, integrationEntry{name: name, checker: checker})
	s.intResults = append(s.intResults, models.IntegrationCheck{
		Name:    name,
		Status:  status,
		Message: msg,
	})
	s.intMu.Unlock()
}

// GetIntegrationStatus returns the current integration check results.
func (s *Service) GetIntegrationStatus() []models.IntegrationCheck {
	s.intMu.RLock()
	defer s.intMu.RUnlock()
	results := make([]models.IntegrationCheck, len(s.intResults))
	copy(results, s.intResults)
	return results
}

// IntegrationsHealthy returns true if no configured integration is unhealthy.
func (s *Service) IntegrationsHealthy() (bool, []string) {
	s.intMu.RLock()
	defer s.intMu.RUnlock()
	healthy := true
	var issues []string
	for _, r := range s.intResults {
		if r.Status == models.IntegrationUnhealthy {
			healthy = false
			issues = append(issues, fmt.Sprintf("%s integration unhealthy: %s", r.Name, r.Message))
		}
	}
	return healthy, issues
}

func (s *Service) integrationCheckLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("health: integration check recovered from panic: %v", r)
		}
	}()

	s.runIntegrationChecks()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runIntegrationChecks()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) runIntegrationChecks() {
	s.intMu.RLock()
	count := len(s.integrations)
	s.intMu.RUnlock()

	for i := 0; i < count; i++ {
		s.intMu.RLock()
		entry := s.integrations[i]
		s.intMu.RUnlock()

		if entry.checker == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		start := time.Now()
		err := entry.checker(ctx)
		elapsed := time.Since(start)
		cancel()

		result := models.IntegrationCheck{
			Name:       entry.name,
			LastCheck:  time.Now(),
			ResponseMs: elapsed.Milliseconds(),
		}
		if err != nil {
			result.Status = models.IntegrationUnhealthy
			result.Message = err.Error()
			log.Printf("health: integration %s unhealthy: %v", entry.name, err)
		} else {
			result.Status = models.IntegrationHealthy
			result.Message = "OK"
		}

		s.intMu.Lock()
		s.intResults[i] = result
		s.intMu.Unlock()
	}
}

// --- Checker factories ---

// NewMongoChecker returns a checker that pings the MongoDB client.
func NewMongoChecker(client *mongo.Client) IntegrationChecker {
	return func(ctx context.Context) error {
		return client.Ping(ctx, nil)
	}
}

// NewStripeChecker returns a checker that calls the Stripe Balance API.
// Relies on the global stripe.Key being set by the stripe service.
func NewStripeChecker() IntegrationChecker {
	return func(ctx context.Context) error {
		params := &stripe.BalanceParams{}
		params.Context = ctx
		_, err := balance.Get(params)
		return err
	}
}

// NewResendChecker returns a checker that hits the Resend domains endpoint.
func NewResendChecker(apiKey string) IntegrationChecker {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://api.resend.com/domains", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("resend API returned status %d", resp.StatusCode)
		}
		return nil
	}
}

// NewGoogleOAuthChecker returns a checker that verifies Google's OpenID discovery endpoint is reachable.
// This confirms the OAuth service is available; credential validity is verified at login time.
func NewGoogleOAuthChecker() IntegrationChecker {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://accounts.google.com/.well-known/openid-configuration", nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("google openid discovery returned status %d", resp.StatusCode)
		}
		return nil
	}
}

// Ensure mutex fields are initialized (called from init or New doesn't need explicit init for sync.RWMutex)
var _ sync.RWMutex
