# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LastSaaS is a complete SaaS boilerplate with:
- **Backend**: Go 1.26 with gorilla/mux, MongoDB, JWT authentication
- **Frontend**: React 19 + TypeScript + Vite 7 + Tailwind CSS 4
- **Billing**: Stripe integration (subscriptions, credit bundles, invoices)
- **Auth**: JWT + OAuth (Google, GitHub, Microsoft) + MFA/TOTP + Magic Links
- **Multi-tenancy**: Full RBAC with owner/admin/user roles
- **White-label**: Custom branding, themes, landing pages
- **Webhooks**: 19 event types with HMAC-SHA256 signing
- **Health monitoring**: Real-time system metrics and alerting
- **Telemetry**: Product analytics with Go SDK and REST API
- **MCP Server**: 32 read-only tools for AI admin access

## Development Commands

### Backend (Go)

```bash
# Run server (after `source .env`)
cd backend && go run ./cmd/server

# Build CLI tool
cd backend && go build -o lastsaas ./cmd/lastsaas

# Run all tests
cd backend && go test ./...

# Run specific test
cd backend && go test -v ./internal/middleware/...

# Run unit tests only (no DB)
cd backend && go test -short ./...

# Run integration tests only
cd backend && go test -v -run Integration ./...

# Generate coverage report
cd backend && go test -coverprofile=coverage.out ./...
cd backend && go tool cover -html=coverage.out

# Build for production
cd backend && go build -ldflags="-s -w" -o server ./cmd/server

# Alternative: Use Makefile targets
cd backend && make test           # Run all tests
cd backend && make test-unit      # Unit tests only
cd backend && make test-integration # Integration tests only
cd backend && make test-coverage  # Generate coverage report
```

### Frontend (React/TypeScript)

```bash
# Install dependencies
cd frontend && npm install

# Start dev server
cd frontend && npm run dev

# Type check only
cd frontend && npx tsc --noEmit

# Run linter
cd frontend && npm run lint

# Run tests
cd frontend && npm test

# Run tests in watch mode
cd frontend && npm run test:watch

# Build for production
cd frontend && npm run build

# Preview production build
cd frontend && npm run preview
```

### Setup

```bash
# Initial project setup (creates .env and config)
./scripts/setup.sh

# Initialize database (create root tenant and owner)
cd backend && go run ./cmd/lastsaas setup
```

### CI/CD

GitHub Actions workflow (`.github/workflows/ci.yml`):
- Runs on Go 1.25
- Builds backend with `CGO_ENABLED=0 go build ./...`
- Runs tests with coverage (requires `MONGODB_URI` secret)
- Uploads coverage to Codecov

## Architecture

### Directory Structure

```
backend/
  cmd/
    server/main.go           # HTTP server entry point
    lastsaas/main.go         # CLI tool + MCP server
  internal/
    api/handlers/            # HTTP handlers (grouped by domain)
    auth/                    # JWT, OAuth, password hashing, TOTP
    middleware/              # Auth, RBAC, rate limiting, metrics
    models/                  # Data models with validation tags
    db/                      # MongoDB connection + JSON schemas
    config/                  # YAML config loader with env expansion
    configstore/             # Runtime DB-backed configuration
    events/                  # Internal event emitter
    health/                  # System health monitoring
    telemetry/               # Product analytics + Go SDK
    stripe/                  # Stripe service wrapper
    syslog/                  # Structured logging with severity levels
    validation/              # Custom validators
    datadog/                 # DataDog metrics/events/logs integration
    webhooks/                # Outgoing webhook dispatcher
frontend/
  src/
    api/client.ts            # Axios client with token refresh
    components/              # Reusable UI components
    contexts/                # Auth, Tenant, Branding, Theme contexts
    pages/
      auth/                  # Login, signup, MFA, password reset
      app/                   # Customer dashboard, billing, team, settings
      admin/                 # Admin interface
      public/                # Landing page, custom pages
```

### Key Architectural Patterns

**Authentication Pipeline**:
1. `middleware.AuthMiddleware.RequireAuth` validates JWT or API key (`lsk_*`)
2. API keys auto-resolve root tenant for admin authority
3. `middleware.TenantMiddleware.RequireTenant` resolves tenant from header/context
4. `middleware.RequireRole()` enforces RBAC (owner/admin/user)
5. `middleware.RequireRootTenant()` restricts to system admin

**Hybrid Validation** (CRITICAL):
- Go validation uses `validate` struct tags (go-playground/validator)
- MongoDB validation uses JSON Schema (`internal/db/schema.go`)
- Both must be kept in sync for every model change

**Event-Driven Webhooks**:
- `events.Emitter` interface abstracts event publishing
- `webhooks.Dispatcher` implements the interface
- Handlers call `emitter.Emit()`; webhooks deliver asynchronously
- 19 event types defined in `models/webhook.go`

**Credit System**:
- Dual buckets: subscription credits (reset/accrue) + purchased credits
- `billing.SubscriptionCredits` tracks plan-allocated credits
- Usage middleware checks credits before allowing operations

**Multi-tenancy Isolation**:
- Every authenticated request has a tenant context
- Handlers must use `middleware.GetTenantFromContext()` for scoping
- Root tenant (`isRoot: true`) bypasses billing checks

**Telemetry SDK** (`internal/telemetry/service.go`):
- In-process: `telemetry.Track()` / `TrackBatch()` / `TrackPageView()` (no HTTP overhead)
- REST API: `POST /telemetry/events` for external clients
- Auto-instruments auth and billing events
- 365-day retention via MongoDB TTL
- Async buffered writes for performance

**DataDog Integration** (`internal/datadog/client.go`):
- Async-buffered metrics, events, logs, and service checks
- Submits to DataDog REST API without requiring an agent
- Environment tagging with app name, hostname, machine ID, region

### Critical Implementation Details

**JWT + API Key Auth** (`internal/middleware/auth.go`):
- Access tokens: 30min TTL, refresh tokens: 7 days with rotation
- API keys are SHA-256 hashed; only `lsk_*` prefix keys are valid
- Revoked tokens stored in `revoked_tokens` collection

**Rate Limiting** (`internal/middleware/ratelimit.go`):
- MongoDB-backed distributed rate limiter
- Different limits per endpoint (login, registration, etc.)
- Keys: IP for anonymous, user ID for authenticated

**Database Schema Validation** (`internal/db/schema.go`):
- 17 collections with JSON Schema validators
- Applied at startup via `EnsureSchemaValidation()`
- Schema changes require migration in `internal/version/migrations.go`

**Config Store** (`internal/configstore/`):
- DB-backed runtime configuration with caching
- Auto-reloads every 60 seconds
- Access via `cfgStore.Get(key)` with dot notation
- System defaults defined in `seed.go`

**System Logging** (`internal/syslog/`):
- Use `syslog.Logger` for all significant system events
- Severity levels: critical, high, medium, low, debug
- Automatic log injection detection with critical alerts

## Validation Rules

When modifying model structs in `internal/models/`:
1. Update `validate` struct tags on the model
2. Update corresponding MongoDB JSON Schema in `internal/db/schema.go`
3. Keep both in sync — they enforce the same constraints
4. Run `cd backend && go test ./internal/validation/...` to verify

When adding a new collection that accepts user/API writes:
1. Add `validate` tags to the model struct
2. Add a schema function to `internal/db/schema.go` and include it in `AllSchemas()`
3. Add tests in `internal/validation/validate_test.go`

## Build Verification

Always verify after changes:
```bash
cd backend && go build ./...
cd frontend && npx tsc --noEmit
```

## Dependent Project Deployment (CRITICAL)

Any project built on the LastSaaS boilerplate — whether using it as a Git submodule, fork, or copy — **MUST** deploy using the SaaS Dockerfile (`Dockerfile.saas`) and the corresponding Fly config (`fly.saas.toml`). Never use bare `fly deploy` on a project that depends on LastSaaS.

**Why this matters:** The SaaS Dockerfile runs both the product backend AND the LastSaaS backend behind Caddy (via supervisord). The LastSaaS backend serves all auth endpoints (`/api/auth/*`), bootstrap status (`/api/bootstrap/status`), OAuth providers (Google, etc.), billing, and admin APIs. Without it, login breaks silently — the product backend has no auth routes, so API calls return HTML from the SPA catch-all, causing mysterious redirects to `/setup` or broken login forms with missing OAuth buttons.

**Correct deploy command:**
```bash
fly deploy -c fly.saas.toml
```

**Propagation rule:** When setting up or working on any dependent project, ensure:
1. The project has a `deploy.md` at its root with full deployment instructions and the "why" behind the multi-process architecture
2. The project's Claude Code memory (MEMORY.md or CLAUDE.md) contains a cross-reference: "See `deploy.md` — never bare `fly deploy`"
3. If the project doesn't have these yet, create them before the first deployment

## Common Development Tasks

### Adding a New API Endpoint

1. Add handler in `backend/internal/api/handlers/`
2. Wire route in `backend/cmd/server/main.go` with appropriate middleware
3. Add TypeScript types in `frontend/src/types/index.ts`
4. Add API call in `frontend/src/api/client.ts` or create new file
5. Create page/component in `frontend/src/pages/`

### Adding a Database Collection

1. Create model in `backend/internal/models/`
2. Add collection accessor in `backend/internal/db/mongodb.go`
3. Add JSON schema validator in `backend/internal/db/schema.go`
4. Add validation tests in `internal/validation/validate_test.go`

### Adding a New Configuration Variable

1. Add to `backend/internal/configstore/seed.go` with type and default
2. Access via `cfgStore.Get("variable.name")` in handlers
3. Document in admin UI at `frontend/src/pages/admin/ConfigPage.tsx`

### Adding a Webhook Event Type

1. Add constant in `backend/internal/models/webhook.go` (both type and AllWebhookEventTypes)
2. Emit from handler via `emitter.Emit()` with appropriate event type
3. Document in `backend/internal/api/handlers/docs.go`
4. Frontend webhook config automatically lists it

### Using the Telemetry SDK

For in-process event tracking (Go code):
```go
// Track a single event
telemetrySvc.Track(ctx, models.TelemetryEvent{
    EventName: "feature.used",
    UserID:    &userID,
    TenantID:  &tenantID,
    Properties: map[string]any{
        "feature_name": "export",
        "format":       "csv",
    },
})

// Track page view
telemetrySvc.TrackPageView(ctx, sessionID, path, nil)
```
