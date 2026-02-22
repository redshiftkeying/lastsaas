# LastSaaS

**The last SaaS boilerplate you'll ever need.**

LastSaaS is a complete, production-ready SaaS foundation built entirely through conversation with [Claude Code](https://claude.ai/claude-code). It gives you multi-tenant account management, authentication, role-based access control, a full admin interface, system health monitoring, billing plans, and credit-based usage tracking — everything you need to launch a SaaS business, ready to customize for your specific product.

The bottleneck for building software isn't engineering capacity anymore — it's imagination. LastSaaS proves it: a single person with a clear vision and an AI agent can stand up what used to require a team and months of work. Now anyone can stand up a complete SaaS business using Claude Code.

**[Project Page](https://metavert.io/lastsaas)**

---

## Why LastSaaS Exists

Every SaaS product needs the same boring foundation: user accounts, teams, roles, authentication, admin dashboards, billing, usage limits. Historically, building that foundation meant weeks of plumbing before you could write a single line of your actual product.

LastSaaS eliminates that. Fork it, point Claude Code at it, and start building your product on top of a foundation that already handles:

- Multi-tenant isolation with role-based access
- JWT authentication with refresh tokens
- Google OAuth integration
- Email verification and password resets
- Team invitations and member management
- Subscription plans with entitlements
- Credit-based usage tracking (subscription + purchased buckets)
- A full admin interface for managing everything
- Real-time system health monitoring
- CLI tools for server administration
- Production deployment on Fly.io

This is open-source infrastructure for the creator era of software — where the person with the idea is also the person who ships it.

---

## Features

### Authentication & Identity
- Email/password registration with bcrypt hashing
- Email verification via [Resend](https://resend.com)
- Google OAuth with automatic account linking
- JWT access tokens (30min) + refresh tokens (7 days)
- Account lockout after failed login attempts
- Password reset flow with secure tokens
- Password strength enforcement

### Multi-Tenancy
- Root tenant (system admin) + customer tenants
- Users belong to tenants via memberships
- Roles: **owner**, **admin**, **user**
- Team invitations with email notifications
- Ownership transfer between members

### Billing & Credits
- Subscription plans with monthly pricing and annual discounts
- Per-plan entitlements (boolean and numeric)
- Usage credits with configurable reset policies (reset or accrue)
- Dual credit buckets: subscription credits + purchased credits
- Credit bundles for one-time purchases
- Billing waiver for special accounts

### Admin Interface
- Dashboard with system overview
- User management (list, view, edit, suspend, delete with ownership preflight)
- Tenant management (list, view, edit, plan assignment, status control)
- Plan management (create, edit, entitlements, assign to tenants)
- Credit bundle management
- System log viewer with severity filtering
- Configuration variable editor (strings, numbers, enums, templates)
- In-app messaging system
- System health monitoring (see below)

### System Health Monitoring
- Automatic node registration with heartbeat (30s interval)
- Metrics collection every 60s: CPU, memory, disk, network, HTTP request stats, MongoDB stats, Go runtime
- HTTP metrics middleware with percentile latency tracking (p50/p95/p99)
- Threshold-based alerting (configurable warning/critical levels)
- 30-day automatic data retention via MongoDB TTL indexes
- Real-time dashboard with 8 time-series charts (Recharts)
- Aggregate, all-nodes overlay, and single-node filter modes
- Time range selection: 1h, 6h, 24h, 7d, 30d

### CLI Administration
- `lastsaas setup` — Initialize the system (create root tenant + owner)
- `lastsaas start` / `stop` / `restart` — Server process management
- `lastsaas change-password` — Reset any user's password
- `lastsaas send-message` — Send system messages to users
- `lastsaas transfer-root-owner` — Transfer root tenant ownership
- `lastsaas config list|get|set` — Manage configuration variables
- `lastsaas version` — Show binary and database versions
- `lastsaas status` — Check system health

### Production Ready
- Dockerized multi-stage build (Go + Node + Alpine)
- Fly.io deployment with auto-stop/auto-start machines
- SPA serving from the Go binary (no separate web server needed)
- CORS, security headers, rate limiting
- Graceful shutdown with connection draining

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25, gorilla/mux |
| Frontend | React 19, TypeScript, Vite 7, Tailwind CSS 4 |
| Database | MongoDB (Atlas or local) |
| Auth | JWT (access + refresh), bcrypt, Google OAuth |
| Email | Resend |
| Charts | Recharts |
| Metrics | gopsutil v4 |
| Deployment | Docker, Fly.io |

---

## Prerequisites

- **Go 1.25+** — [install](https://go.dev/doc/install)
- **Node.js 22+** — [install](https://nodejs.org)
- **MongoDB** — either:
  - [MongoDB Atlas](https://www.mongodb.com/atlas) (free M0 tier works fine)
  - Local MongoDB Community Edition
- **Git**

### Optional
- [Resend](https://resend.com) API key — for email verification, password resets, and invitations
- Google OAuth credentials — for Google sign-in
- [Fly.io](https://fly.io) account — for production deployment

---

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/jonradoff/lastsaas.git
cd lastsaas
```

### 2. Run the setup script

```bash
./scripts/setup.sh
```

This will prompt you for:
- **Database name** — the project identity (two projects sharing a name share the same user base)
- **MongoDB URI** — your Atlas connection string or `mongodb://localhost:27017`
- **JWT secrets** — auto-generated
- **Google OAuth credentials** — optional, press Enter to skip
- **Resend API key** — optional, press Enter to skip
- **App name and email settings**

It writes a `.env` file and copies the config template.

### 3. Start the backend

```bash
set -a && source .env && set +a
cd backend
go run ./cmd/server
```

The server starts on `http://localhost:4290`.

### 4. Start the frontend

In a separate terminal:

```bash
set -a && source .env && set +a
cd frontend
npm install
npm run dev
```

The frontend starts on `http://localhost:4280`.

### 5. Initialize the system

Run the CLI setup to create the root tenant and admin account:

```bash
cd backend
go run ./cmd/lastsaas setup
```

This creates the root tenant (your admin organization) and the owner account. You can now log in at `http://localhost:4280`.

---

## Configuration

Config files live in `backend/config/`:
- `dev.example.yaml` / `prod.example.yaml` — committed templates
- `dev.yaml` / `prod.yaml` — your actual configs (gitignored)

Set `LASTSAAS_ENV=dev` or `LASTSAAS_ENV=prod` to select which config to load. Defaults to `dev`.

Secrets are referenced as `${ENV_VAR}` in YAML and expanded from environment variables at load time. Default values use `${VAR:default}` syntax.

### Key Configuration

| Variable | Description |
|----------|------------|
| `DATABASE_NAME` | Project identity — shared name = shared user base |
| `MONGODB_URI` | MongoDB connection string |
| `JWT_ACCESS_SECRET` | Secret for signing access tokens |
| `JWT_REFRESH_SECRET` | Secret for signing refresh tokens |
| `GOOGLE_CLIENT_ID` | Google OAuth client ID (optional) |
| `GOOGLE_CLIENT_SECRET` | Google OAuth secret (optional) |
| `RESEND_API_KEY` | Resend email service key (optional) |
| `APP_NAME` | Your application name (used in emails, UI) |
| `FRONTEND_URL` | Frontend URL for CORS and email links |

---

## Project Structure

```
lastsaas/
  backend/
    cmd/
      server/main.go              Entry point (HTTP server)
      lastsaas/main.go             CLI administration tool
    config/                        YAML config files
    internal/
      api/handlers/                HTTP handlers (auth, admin, tenant, health, etc.)
      auth/                        JWT, password hashing, Google OAuth
      config/                      Config loader with env expansion
      configstore/                 Runtime configuration (DB-backed, cached)
      db/                          MongoDB connection, collections, indexes
      email/                       Resend email service with templates
      events/                      Event emitter interface
      health/                      System health monitoring service
      middleware/                   Auth, tenant, RBAC, rate limiting, metrics, security
      models/                      All data models
      planstore/                   Plan seeding
      syslog/                      System logging service
      version/                     Version management and migration
  frontend/
    src/
      api/client.ts                Axios API client
      components/                  Layout, AdminLayout, shared components
      contexts/                    Auth and Tenant React contexts
      pages/
        admin/                     Admin interface pages
        admin/health/              Health monitoring components and charts
        app/                       Customer-facing pages
        auth/                      Login, signup, verification, password reset
      types/index.ts               TypeScript type definitions
  scripts/
    setup.sh                       Interactive setup script
  Dockerfile                       Multi-stage production build
  fly.toml                         Fly.io deployment config
  VERSION                          Current version number
```

---

## Deployment

### Fly.io

LastSaaS includes a Dockerfile and Fly.io configuration for production deployment.

```bash
# Install flyctl if needed
curl -L https://fly.io/install.sh | sh

# Create the app (first time only)
flyctl apps create your-app-name --org your-org

# Set production secrets
flyctl secrets set \
  DATABASE_NAME="your-db-name" \
  MONGODB_URI="mongodb+srv://..." \
  JWT_ACCESS_SECRET="$(openssl rand -hex 32)" \
  JWT_REFRESH_SECRET="$(openssl rand -hex 32)" \
  FRONTEND_URL="https://your-app-name.fly.dev" \
  APP_NAME="YourApp"

# Deploy
flyctl deploy
```

The Dockerfile builds both the Go backend and React frontend into a single ~14MB Alpine container. The Go binary serves the frontend SPA directly — no nginx or separate web server required.

### Other Platforms

The Docker image works anywhere containers run. The only external dependency is MongoDB. Set the environment variables listed above and expose port 8080.

---

## API Routes

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/bootstrap/status` | No | Check if system is initialized |
| POST | `/api/auth/register` | No | Create account |
| POST | `/api/auth/login` | No | Login |
| POST | `/api/auth/refresh` | No | Refresh tokens |
| POST | `/api/auth/verify-email` | No | Verify email address |
| POST | `/api/auth/forgot-password` | No | Request password reset |
| POST | `/api/auth/reset-password` | No | Reset password with token |
| GET | `/api/auth/google` | No | Start Google OAuth |
| GET | `/api/auth/me` | JWT | Current user + memberships |
| POST | `/api/auth/logout` | JWT | Logout |
| POST | `/api/auth/change-password` | JWT | Change password |
| POST | `/api/auth/accept-invitation` | JWT | Accept team invitation |
| GET | `/api/tenant/members` | JWT+Tenant | List team members |
| POST | `/api/tenant/members/invite` | JWT+Admin | Invite member |
| DELETE | `/api/tenant/members/{id}` | JWT+Admin | Remove member |
| PATCH | `/api/tenant/members/{id}/role` | JWT+Owner | Change member role |
| POST | `/api/tenant/members/{id}/transfer-ownership` | JWT+Owner | Transfer ownership |
| GET | `/api/messages` | JWT | List messages |
| GET | `/api/messages/unread-count` | JWT | Unread message count |
| PATCH | `/api/messages/{id}/read` | JWT | Mark message as read |
| GET | `/api/plans` | JWT | List plans (public) |
| GET | `/api/credit-bundles` | JWT | List credit bundles (public) |
| GET | `/api/admin/about` | JWT+Root+Admin | System info |
| GET | `/api/admin/logs` | JWT+Root+Admin | System logs |
| GET/POST | `/api/admin/config` | JWT+Root+Admin | Configuration management |
| GET | `/api/admin/tenants` | JWT+Root+Admin | List tenants |
| GET | `/api/admin/plans` | JWT+Root+Admin | List plans |
| GET | `/api/admin/health/*` | JWT+Root+Admin | Health monitoring |
| GET | `/api/admin/users` | JWT+Root+Owner | User management |
| PUT/DELETE | `/api/admin/users/{id}` | JWT+Root+Owner | Modify users |
| POST/PUT/DELETE | `/api/admin/plans/*` | JWT+Root+Owner | Manage plans |
| PATCH | `/api/admin/tenants/{id}/plan` | JWT+Root+Owner | Assign plans |

---

## Extending LastSaaS

LastSaaS is designed to be a starting point. Fork it and build your product on top:

1. **Add your product's data models** in `backend/internal/models/`
2. **Add API handlers** in `backend/internal/api/handlers/`
3. **Wire routes** in `backend/cmd/server/main.go`
4. **Add frontend pages** in `frontend/src/pages/`
5. **Use the tenant context** — every authenticated request carries the user's tenant, so your product logic gets multi-tenancy for free
6. **Use the credit system** — check and deduct credits for usage-based features
7. **Use the config store** — add runtime-configurable settings without redeployment

The entire codebase was built conversationally with Claude Code. You can keep building it the same way — describe what you want, and let the agent implement it on top of the existing patterns.

---

## License

[MIT](LICENSE) - Copyright 2026 Metavert LLC
