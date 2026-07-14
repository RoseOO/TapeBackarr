# AGENTS.md — TapeBackarr Project Guide for AI Assistants

## Project Identity

**TapeBackarr** — a production-grade tape library management system with a modern web UI. It manages LTO tape drives, streams data directly from network shares to tape without large intermediate disk, and provides full file-level cataloging with guided restores.

**Module:** `github.com/RoseOO/TapeBackarr`
**License:** MIT
**Default credentials:** `admin` / `changeme`

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24 (CGO_ENABLED=0, single binary) |
| HTTP router | go-chi/chi/v5 |
| Database | SQLite via modernc.org/sqlite (pure Go, no CGO) |
| Auth | JWT (HS256) + bcrypt passwords + API keys |
| Frontend | SvelteKit 5 + TypeScript, static SPA adapter |
| Bundler | Vite 7 |
| Scheduling | robfig/cron/v3 |
| Encryption | AES-256-GCM |
| Deployment | systemd, Docker, Proxmox LXC |

## Repository Layout

```
cmd/tapebackarr/main.go           # Entry point: CLI flags, wiring, server lifecycle
internal/
  api/server.go + events.go       # Chi router, ~90+ REST handlers, SSE event bus
  auth/service.go                 # JWT tokens, bcrypt, API keys, RBAC (admin/operator/readonly)
  backup/service.go               # File scanner, tar streaming, incremental backup, multi-tape spanning
  cmdutil/cmdutil.go              # Error extraction from exec.Command
  config/config.go                # JSON config parsing (server, db, tape, auth, notifications, proxmox)
  database/database.go            # SQLite wrapper, embedded migration runner
  database/migrations/            # 18 sequential SQL migrations (001 through 018)
  encryption/service.go + stream.go  # AES-256-GCM, key sheets, chunked streaming readers
  logging/logger.go               # Structured JSON/text logger, audit logging
  models/models.go                # All domain types (598 lines)
  notifications/                  # Telegram bot + SMTP email
  proxmox/                        # Proxmox VE API client (backup/restore VMs and LXCs)
  restore/service.go              # Guided multi-tape restore engine
  scheduler/service.go            # Cron-based job scheduler
  tape/service.go + ltfs.go       # Core tape I/O (mt, tar, labels, drive detection) + LTFS format/mount/restore
web/frontend/
  src/routes/                     # 20 SvelteKit page routes (+page.svelte per feature)
  src/lib/api/client.ts           # Full API client (~745 lines)
  src/lib/components/             # Sidebar, ProgressToolbar, ToastNotifications, VirtualConsole
  src/lib/stores/                 # Svelte stores: auth, theme, console, livedata, notifications
  static/                         # PWA manifest, service worker, icons
deploy/                           # systemd unit, install.sh, proxmox-lxc-install.sh, updater.sh, config example
docs/                             # Architecture, API ref, DB schema, guides — embedded via go:embed
```

## Service Architecture Pattern

Every domain follows the same pattern — services are structs wired into `api.Server` in `main.go`:

```
internal/{domain}/
├── service.go        # Service struct with methods, dependency injection
└── service_test.go   # Table-driven tests, co-located
```

`api.Server` holds all services and defines all routes:

```go
type Server struct {
    router          *chi.Mux
    db              *database.DB
    authService     *auth.Service
    tapeService     *tape.Service
    backupService   *backup.Service
    restoreService  *restore.Service
    // + encryption, scheduler, proxmox, notifications, config
}
```

## Build & Test Commands

```bash
make build               # Build backend + frontend
make build-backend       # Go binary → ./tapebackarr
make build-frontend      # cd web/frontend && npm install && npm run build
make test                # go test -v ./...
make test-coverage       # Tests with coverage HTML report
make lint                # go vet ./... && go fmt ./...
make dev-backend         # Backend dev server on :8080
make dev-frontend        # Frontend dev server on :5173

# Go-specific:
go test -v ./internal/backup/...   # Test single package
go test -race ./...                # Race detector
CGO_ENABLED=0 go build ...         # Backend builds without CGO

# Frontend:
cd web/frontend && npm run dev     # Dev server
cd web/frontend && npm run check   # TypeScript type checking (svelte-check)
```

## Database

SQLite with 18 embedded migrations. WAL mode, single writer connection. Migrations run on startup automatically.

Key tables: `users`, `tapes`, `tape_pools`, `tape_drives`, `backup_sources`, `backup_destinations`, `backup_jobs`, `backup_sets`, `catalog_entries`, `snapshots`, `job_executions`, `tape_libraries`, `drive_statistics`, `drive_alerts`, `encryption_keys`, `api_keys`, `proxmox_nodes`, `proxmox_guests`, `proxmox_backups`, `proxmox_restores`, `audit_logs`

State is persisted in the DB. Jobs are resumable after crashes. Tape labels contain UUIDs for identity.

## API Overview

All endpoints under `/api/v1/`. Key groups:

| Group | Notable endpoints |
|-------|------------------|
| Auth | POST /login, POST /change-password |
| Tapes | CRUD, label, format (raw/LTFS), batch label, export/import, LTO types |
| Pools | CRUD |
| Drives | CRUD, scan, status, eject, rewind, inspect, format, stats, alerts, clean, retension |
| Sources | CRUD for backup source paths |
| Destinations | CRUD for backup targets (tape pools or file paths) |
| Jobs | CRUD, run, cancel, pause, resume, retry, active list, tape recommendation |
| Backup Sets | list, get, files, delete, cancel |
| Catalog | search, browse |
| Restore | plan, run, raw-read |
| LTFS | status, format, mount, unmount, browse, restore, check |
| Libraries | CRUD, scan, inventory, slots, load, unload, transfer |
| Encryption | list/create/import/delete keys, key sheets |
| Proxmox | nodes, guests, cluster, backup/restore jobs, scheduling |
| Events | SSE stream, history |
| Health | GET /health, GET /api/v1/health |

Auth middleware: JWT required for /api/v1/*. RBAC roles: admin (full), operator (manage jobs/tapes), readonly (view only).

## Frontend Conventions

- SvelteKit 5 with file-based routing in `src/routes/`
- Static adapter — backend serves `web/frontend/build/` as static files with SPA fallback
- TypeScript throughout
- API calls via `api.client.ts` functions, not raw fetch
- State via Svelte writable stores in `src/lib/stores/`
- Toast notifications via `ToastNotifications.svelte` component
- SSE for real-time updates (EventBus on backend, EventSource on frontend)
- Dark/light theme via CSS custom properties, toggled by `theme.ts` store

## Code Conventions

### Go
- Follow Go Code Review Comments
- `gofmt` formatted, `go vet` clean
- Explicit error handling — never `_` unless justified
- Doc comments on all exported symbols
- Table-driven tests in `*_test.go` files
- Use `internal/` for all packages — nothing exported outside the module
- Use `t.TempDir()` for test temporary files
- Mock external dependencies (tape devices are mocked — app needs real hardware at runtime)

### Frontend
- TypeScript for all `.ts` and `<script lang="ts">` files
- Run `npm run check` (svelte-check) before committing frontend changes
- New pages go in `src/routes/{feature}/+page.svelte`
- New API endpoints need corresponding functions in `src/lib/api/client.ts`

### Git
- Conventional commits: `type(scope): description`
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- Example: `feat(backup): add incremental backup support`

## Tape Format

Tapes have a self-describing three-section layout with file marks:

```
[Label 512B] [FM] [Backup Data (tar)] [FM] [TOC (JSON)] [FM] [EOD]
  File #0          File #1                  File #2
```

- **Label:** `TAPEBACKARR|label|uuid|pool|timestamp|encryption_fingerprint|compression_type`
- **Data:** Standard tar stream, optionally encrypted (AES-256-GCM) and/or compressed (gzip/zstd)
- **TOC:** JSON file catalog (paths, sizes, checksums) — makes tapes self-describing without the DB

LTFS format is also supported as an alternative — see `internal/tape/ltfs.go`.

## Key Design Decisions

1. **Single binary** — Go binary embeds the static frontend build. No separate web server needed.
2. **SQLite** — No external database dependency. Metadata is small, fits in one file.
3. **No CGO** — Pure Go compilation for maximum portability.
4. **System utilities** — Uses `mt`, `tar`, `mtx`, `sg3_utils`, `mbuffer` for tape I/O rather than writing raw SCSI commands.
5. **Self-describing tapes** — Label + TOC on each tape means disaster recovery without the application (see `docs/MANUAL_RECOVERY.md`).
6. **Multi-tape spanning** — Backups larger than a single tape automatically span with operator guidance.
7. **Per-device mutex** — Tape drive access is serialized per physical device to prevent conflicts.
8. **File destinations** — Backup jobs can write to NFS shares / mounted disks / local directories instead of tape. Output is a tar archive with a companion `.toc.json` file. Job's `destination_id` points to a `BackupDestination` record; if `destination_type` is `file`, `RunBackupToFile` is used instead of the tape pipeline.

## Common Tasks

### Adding a new API endpoint
1. Add handler method on `api.Server` in `internal/api/server.go`
2. Register route in the same file's route setup
3. Add corresponding function in `web/frontend/src/lib/api/client.ts`
4. If needed, add model types in `internal/models/models.go`

### Adding a new frontend page
1. Create `web/frontend/src/routes/{feature}/+page.svelte`
2. Import components from `$lib/components/`
3. Use `$lib/api/client` for data fetching
4. Use stores from `$lib/stores/` for shared state

### Adding a new service
1. Create `internal/{domain}/service.go` with a `Service` struct
2. Add constructor that accepts dependencies
3. Wire into `api.Server` in `internal/api/server.go`
4. Wire into `main()` in `cmd/tapebackarr/main.go`
5. Create `service_test.go` with table-driven tests

### Database changes
1. Create new migration file: `internal/database/migrations/NNN_description.sql`
2. Use `ALTER TABLE` or `CREATE TABLE IF NOT EXISTS` — migrations run idempotently
3. Update models in `internal/models/models.go`
4. Update database query methods as needed

### Proxmox LXC install script changes
The deploy script `deploy/proxmox-lxc-install.sh` downloads Debian LXC templates and installs TapeBackarr inside a container with tape device passthrough. Template selection is in `select_template()`.

## Runtime Dependencies

The host system needs (in addition to the Go binary):
- `mt-st` — tape positioning/control
- `tar` — data streaming
- `mbuffer` — I/O buffering
- `sg3-utils` — SCSI generic commands
- `mtx` — tape library autochanger control
- `pigz` — parallel gzip compression
- `fuse` / `libfuse2` — LTFS filesystem support
- LTFS (Linear Tape File System) — built from source if not in repos

## Configuration

Single JSON file (default `/etc/tapebackarr/config.json`). CLI: `-config path`, `-version`, `-init-config`.

Top-level keys: `server`, `database`, `tape`, `logging`, `auth`, `notifications` (telegram + email), `proxmox`.

See `deploy/config.example.json` for the full template with all options documented.
