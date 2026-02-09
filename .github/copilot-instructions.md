# Copilot Instructions for TapeBackarr

## Project Overview

TapeBackarr is a production-grade tape library management system with a Go backend and SvelteKit frontend. It manages LTO tape drives, supports direct streaming from network shares to tape, and provides a modern web interface for backup/restore operations.

**Tech stack:** Go 1.24+ (Chi router, SQLite) · SvelteKit 5 / TypeScript · Vite · Docker

## Repository Structure

```
cmd/tapebackarr/         # Application entry point (main.go)
internal/                # Core Go packages
  api/                   # REST API handlers and server (Chi router)
  auth/                  # JWT authentication and RBAC
  backup/                # Backup execution service
  cmdutil/               # Command utility functions
  config/                # Configuration loading
  database/              # SQLite database layer (modernc.org/sqlite)
  encryption/            # AES-256 encryption service
  logging/               # Structured JSON logging
  models/                # Shared data models (User, Tape, BackupJob, etc.)
  notifications/         # Telegram and Email (SMTP) notification services
  proxmox/               # Proxmox VE integration (backup/restore VMs/LXCs)
  restore/               # Restore execution service
  scheduler/             # Cron-based job scheduler (robfig/cron)
  tape/                  # Tape device I/O operations (mt/tar)
web/frontend/            # SvelteKit frontend (TypeScript, Svelte 5)
  src/routes/            # File-based routing (pages)
  src/lib/api/client.ts  # API client library
  src/lib/components/    # Reusable Svelte components
  src/lib/stores/        # Svelte stores (auth, theme, notifications, etc.)
deploy/                  # Deployment configs (systemd, Docker, Proxmox LXC)
docs/                    # Documentation (embedded via Go embed)
```

## Build, Test, and Lint Commands

### Backend (Go)

```bash
make build-backend       # Build Go binary → ./tapebackarr
make test                # Run all Go tests: go test -v ./...
make test-coverage       # Tests with coverage report
make lint                # go vet ./... && go fmt ./...
go test -v ./internal/backup/...  # Run tests for a specific package
go test -race ./...      # Race condition detection
```

### Frontend (SvelteKit)

```bash
cd web/frontend
npm install              # Install dependencies
npm run build            # Production build (static adapter)
npm run dev              # Dev server on http://localhost:5173
npm run check            # TypeScript type checking (svelte-check)
```

### Full Build

```bash
make build               # Build backend + frontend
make dev-backend         # Run backend dev server (http://localhost:8080)
make dev-frontend        # Run frontend dev server (http://localhost:5173)
```

## Code Style and Conventions

### Go

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Format with `gofmt`; lint with `go vet`
- Handle errors explicitly — never swallow errors silently
- Doc comments on all exported functions and types
- Use `internal/` packages — nothing is exported outside the module

### Frontend

- TypeScript for all frontend code
- Svelte 5 component model
- File-based routing in `src/routes/`
- API calls via `src/lib/api/client.ts`
- Reactive state via Svelte stores in `src/lib/stores/`

### Commit Messages

Use conventional commits: `type(scope): description`

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
- `feat(backup): add incremental backup support`
- `fix(tape): handle tape full condition correctly`

## Key Patterns

### Service Layer

Each domain follows this pattern:

```
internal/{domain}/
├── service.go        # Business logic with a Service struct
└── service_test.go   # Unit tests (co-located)
```

Services are injected into the `api.Server` struct and wired in `cmd/tapebackarr/main.go`.

### API Server

The API uses `go-chi/chi` for routing. All routes are defined in `internal/api/server.go` under `/api/v1/`. The `Server` struct holds all service dependencies:

```go
type Server struct {
    router          *chi.Mux
    db              *database.DB
    authService     *auth.Service
    tapeService     *tape.Service
    backupService   *backup.Service
    restoreService  *restore.Service
    // ...
}
```

Middleware: CORS, JWT auth, role-based access (admin/operator/read-only).

### Testing

- **Table-driven tests** are the standard pattern
- Tests are co-located with source (`*_test.go`)
- Use `t.TempDir()` for temporary file/directory needs
- Mock external dependencies (tape devices, network calls)
- Test files exist for all major packages in `internal/`

Example pattern:
```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   SomeInput
        wantErr bool
    }{
        {"valid case", SomeInput{...}, false},
        {"invalid case", SomeInput{...}, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

### Database

- SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- Auto-migrations on startup in `internal/database/`
- All table definitions in the database package

### Real-Time Events

- `EventBus` in `internal/api/` publishes events
- Frontend consumes via Server-Sent Events (SSE) at `/api/v1/events/stream`

## Configuration

- Config file: `deploy/config.example.json` (template)
- Runtime config at `/etc/tapebackarr/config.json`
- Loaded by `internal/config/` package
- Key sections: `server`, `database`, `tape` (drives array), `auth`, `notifications`, `proxmox`

## Dependencies

### Go (key dependencies)

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/go-chi/cors` | CORS middleware |
| `github.com/golang-jwt/jwt/v5` | JWT authentication |
| `golang.org/x/crypto` | bcrypt password hashing |
| `modernc.org/sqlite` | SQLite database (pure Go) |
| `github.com/robfig/cron/v3` | Cron-based job scheduling |

### Frontend

| Package | Purpose |
|---------|---------|
| `@sveltejs/kit` | SvelteKit framework |
| `svelte` | Component framework (v5) |
| `typescript` | Type safety |
| `vite` | Build tool |
| `@sveltejs/adapter-static` | Static site generation |

## Docker

- Multi-stage Dockerfile: Go build → Node.js build → Debian slim runtime
- `docker-compose.yml` for deployment with privileged mode (tape device access)
- Health check: `GET /api/v1/health`

## Important Notes

- The application requires physical tape hardware (`/dev/nst0`, etc.) for tape operations — tests mock these
- Default credentials: `admin` / `changeme`
- JWT secret must be configured in production
- Frontend is built as a static site and served by the Go backend
- Documentation files in `docs/` are embedded into the binary via Go's `embed` package
