# Copilot Instructions for TapeBackarr

> This file is also available as `AGENTS.md` in the repo root. See that file for the canonical project guide used by all LLM coding assistants.

## Project Overview

TapeBackarr is a production-grade tape library management system with a Go backend and SvelteKit frontend. It manages LTO tape drives, supports direct streaming from network shares to tape, and provides a modern web interface for backup/restore operations.

**Tech stack:** Go 1.24+ (Chi router, SQLite) · SvelteKit 5 / TypeScript · Vite · Docker
**Module:** `github.com/RoseOO/TapeBackarr`
**Default credentials:** `admin` / `changeme`

## Repository Structure

```
cmd/tapebackarr/         # Application entry point (main.go)
internal/                # Core Go packages
  api/                   # REST API handlers, SSE event bus (9,058 lines — the largest file)
  auth/                  # JWT authentication and RBAC
  backup/                # Backup execution service (3,252 lines — second largest, streaming engine)
  cmdutil/               # Command utility functions
  config/                # Configuration loading
  database/              # SQLite database layer + 18 embedded migrations
  encryption/            # AES-256-GCM encryption service + streaming readers
  logging/               # Structured JSON/text logging with audit trail
  models/                # Shared data models (User, Tape, BackupJob, etc. — 598 lines)
  notifications/         # Telegram bot and Email (SMTP) notification services
  proxmox/               # Proxmox VE integration (backup/restore VMs/LXCs)
  restore/               # Restore execution service (1,247 lines)
  scheduler/             # Cron-based job scheduler (robfig/cron)
  tape/                  # Tape device I/O (mt/tar) + LTFS format/mount/restore
web/frontend/            # SvelteKit frontend (TypeScript, Svelte 5)
  src/routes/            # 20 page routes (file-based routing)
  src/lib/api/client.ts  # API client library (745 lines)
  src/lib/components/    # Reusable Svelte components
  src/lib/stores/        # Svelte stores (auth, theme, console, livedata, notifications)
  static/                # PWA: manifest.json, service worker, icons
deploy/                  # Deployment configs (systemd, Docker, Proxmox LXC)
docs/                    # Documentation (embedded via Go embed)
```

## Build, Test, and Lint Commands

### Backend (Go)

```bash
make build-backend       # Build Go binary → ./tapebackarr
make test                # Run all Go tests: go test -v ./...
make test-coverage       # Tests with coverage report (HTML)
make lint                # go vet ./... && go fmt ./...
go test -v ./internal/backup/...  # Run tests for a specific package
go test -race ./...      # Race condition detection
CGO_ENABLED=0 go build -o tapebackarr ./cmd/tapebackarr  # Pure Go build
```

### Frontend (SvelteKit)

```bash
cd web/frontend
npm install              # Install dependencies
npm run build            # Production build (static adapter → build/)
npm run dev              # Dev server on http://localhost:5173
npm run check            # TypeScript type checking (svelte-check)
```

### Full Build & Dev

```bash
make build               # Build backend + frontend
make dev-backend         # Run backend dev server (http://localhost:8080)
make dev-frontend        # Run frontend dev server (http://localhost:5173)
make install             # Install to /opt/tapebackarr (requires root)
make clean               # Remove build artifacts
```

## Code Style and Conventions

### Go

- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Format with `gofmt`; lint with `go vet`
- Handle errors explicitly — never swallow errors silently
- Doc comments on all exported functions and types
- Use `internal/` packages — nothing is exported outside the module
- Table-driven tests in `*_test.go` files (see Testing section below)

### Frontend

- TypeScript for all `.ts` and `<script lang="ts">` files
- Svelte 5 component model with file-based routing in `src/routes/`
- API calls via `src/lib/api/client.ts` functions, not raw fetch
- Reactive state via Svelte stores in `src/lib/stores/`
- Run `npm run check` before committing frontend changes

### Commit Messages

Use conventional commits: `type(scope): description`

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
- `feat(backup): add incremental backup support`
- `fix(tape): handle tape full condition correctly`
- `chore(deps): update go-chi to v5.2.5`

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
    encryptionService *encryption.Service
    schedulerService  *scheduler.Service
    proxmoxClient   *proxmox.Client
    // ... additional fields
}
```

Middleware: CORS, JWT auth, role-based access (admin/operator/read-only).

### API Endpoints (90+ total)

All under `/api/v1/`. Auth required except `/health`, `/api/v1/health`, and `/api/v1/login`.

| Group | Key endpoints |
|-------|--------------|
| Auth | POST /login, POST /change-password |
| Tapes | CRUD, label, format (raw/LTFS), batch-label, export, import, read-label |
| Pools | CRUD |
| Drives | CRUD, scan, status, eject, rewind, inspect, format, stats, alerts, clean, retension |
| Sources | CRUD for backup source paths |
| Destinations | CRUD for backup targets (tape pools or file paths) |
| Jobs | CRUD, run, cancel, pause, resume, retry, active, resumable, tape-recommendation |
| Backup Sets | list, get, files, delete, cancel |
| Catalog | search, browse |
| Restore | plan, run, raw-read |
| LTFS | status, format, mount, unmount, browse, restore, check |
| Libraries | CRUD, scan, inventory, slots, load, unload, transfer |
| Encryption | list/create/import/delete keys, key-sheets |
| API Keys | list, create, delete (admin only) |
| Proxmox | nodes, guests, cluster, backup/restore jobs, scheduling |
| Users | list, create, delete (admin only) |
| Settings | get, update, telegram-test, restart |
| Events | SSE stream, history |
| Health | GET /health, GET /api/v1/health |
| Docs | list, get (in-app documentation) |

### Testing

- **Table-driven tests** are the standard pattern
- Tests are co-located with source (`*_test.go`)
- Use `t.TempDir()` for temporary file/directory needs
- Mock external dependencies (tape devices, network calls — app requires real hardware at runtime)
- Test files exist for all major packages in `internal/`

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
- Auto-migrations on startup — 18 sequential files in `internal/database/migrations/`
- WAL mode, single writer connection
- Key tables: users, tapes, tape_pools, tape_drives, backup_sources, backup_destinations, backup_jobs, backup_sets, catalog_entries, snapshots, job_executions, tape_libraries, drive_statistics, drive_alerts, encryption_keys, api_keys, proxmox_nodes, proxmox_guests, proxmox_backups, proxmox_restores, audit_logs

### Real-Time Events

- `EventBus` in `internal/api/events.go` publishes events
- Frontend consumes via Server-Sent Events (SSE) at `/api/v1/events/stream`

### Tape Format

Self-describing three-section layout on tape:

```
[Label 512B] [FM] [Backup Data (tar)] [FM] [TOC (JSON)] [FM] [EOD]
  File #0          File #1                  File #2
```

Label: `TAPEBACKARR|label|uuid|pool|timestamp|encryption_fingerprint|compression_type`
Data: tar stream, optionally encrypted (AES-256-GCM) and/or compressed (gzip/zstd)
TOC: JSON catalog (paths, sizes, checksums) — tapes are self-describing without the DB

LTFS format is also supported as an alternative.

## Configuration

- Config file: `deploy/config.example.json` (template with all options)
- Runtime config at `/etc/tapebackarr/config.json`
- Loaded by `internal/config/` package
- CLI flags: `-config path`, `-version`, `-init-config`
- Key sections: `server`, `database`, `tape` (drives array), `logging`, `auth`, `notifications` (telegram + email), `proxmox`

## Dependencies

### Go (key dependencies)

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` v5.2.5 | HTTP router |
| `github.com/go-chi/cors` v1.2.2 | CORS middleware |
| `github.com/golang-jwt/jwt/v5` v5.3.1 | JWT authentication |
| `golang.org/x/crypto` v0.47.0 | bcrypt password hashing |
| `modernc.org/sqlite` v1.44.3 | SQLite database (pure Go) |
| `github.com/robfig/cron/v3` v3.0.1 | Cron-based job scheduling |

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
- Builds LTFS from source in the Docker image

## Common Task Workflows

### Adding a new API endpoint
1. Add handler method on `api.Server` in `internal/api/server.go`
2. Register route in the route setup section of the same file
3. Add corresponding function in `web/frontend/src/lib/api/client.ts`
4. Optionally add model types in `internal/models/models.go`

### Adding a new frontend page
1. Create `web/frontend/src/routes/{feature}/+page.svelte`
2. Import components from `$lib/components/`
3. Use `$lib/api/client` for data fetching
4. Use stores from `$lib/stores/` for shared state

### Database changes
1. Create migration: `internal/database/migrations/NNN_description.sql`
2. Use idempotent DDL (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE`)
3. Update models in `internal/models/models.go`
4. Update query methods as needed

### Proxmox LXC install script
The script at `deploy/proxmox-lxc-install.sh` automates container creation with tape device passthrough. Template selection is in the `select_template()` function — see `AGENTS.md` for current Debian version used.

## Important Notes

- The application requires physical tape hardware (`/dev/nst0`, etc.) for tape operations — tests mock these
- CGO_ENABLED=0 for maximum portability
- JWT secret must be configured in production
- Frontend is built as a static site and served by the Go backend with SPA fallback
- Documentation files in `docs/` are embedded into the binary via Go's `embed` package
- All packages under `internal/` — the only public API is the HTTP server
- Runtime deps on the host: mt-st, tar, mbuffer, sg3-utils, mtx, pigz, fuse, libfuse2, LTFS
