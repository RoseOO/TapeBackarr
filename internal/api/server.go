package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/auth"
	"github.com/RoseOO/TapeBackarr/internal/backup"
	"github.com/RoseOO/TapeBackarr/internal/config"
	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/encryption"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/models"
	"github.com/RoseOO/TapeBackarr/internal/notifications"
	"github.com/RoseOO/TapeBackarr/internal/proxmox"
	"github.com/RoseOO/TapeBackarr/internal/restore"
	"github.com/RoseOO/TapeBackarr/internal/scheduler"
	"github.com/RoseOO/TapeBackarr/internal/tape"

	embeddedDocs "github.com/RoseOO/TapeBackarr/docs"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

var cryptoRand io.Reader = rand.Reader

// batchLabelState tracks the current batch label operation
type batchLabelState struct {
	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
	progress  int
	total     int
	current   string
	message   string
	completed int
	failed    int
}

// Server represents the API server
type Server struct {
	router                *chi.Mux
	db                    *database.DB
	authService           *auth.Service
	tapeService           *tape.Service
	backupService         *backup.Service
	restoreService        *restore.Service
	encryptionService     *encryption.Service
	scheduler             *scheduler.Service
	logger                *logging.Logger
	proxmoxBackupService  *proxmox.BackupService
	proxmoxRestoreService *proxmox.RestoreService
	proxmoxClient         *proxmox.Client
	staticDir             string
	configPath            string
	config                *config.Config
	eventBus              *EventBus
	telegramService       *notifications.TelegramService
	batchLabel            batchLabelState
}

// NewServer creates a new API server
func NewServer(
	db *database.DB,
	authService *auth.Service,
	tapeService *tape.Service,
	backupService *backup.Service,
	restoreService *restore.Service,
	encryptionService *encryption.Service,
	scheduler *scheduler.Service,
	logger *logging.Logger,
	proxmoxClient *proxmox.Client,
	proxmoxBackupService *proxmox.BackupService,
	proxmoxRestoreService *proxmox.RestoreService,
	staticDir string,
	configPath string,
	cfg *config.Config,
) *Server {
	s := &Server{
		router:                chi.NewRouter(),
		db:                    db,
		authService:           authService,
		tapeService:           tapeService,
		backupService:         backupService,
		restoreService:        restoreService,
		encryptionService:     encryptionService,
		scheduler:             scheduler,
		logger:                logger,
		proxmoxClient:         proxmoxClient,
		proxmoxBackupService:  proxmoxBackupService,
		proxmoxRestoreService: proxmoxRestoreService,
		staticDir:             staticDir,
		configPath:            configPath,
		config:                cfg,
		eventBus:              NewEventBus(),
	}

	// Wire up backup service events to the event bus
	if backupService != nil {
		backupService.EventCallback = func(eventType, category, title, message string) {
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     eventType,
					Category: category,
					Title:    title,
					Message:  message,
				})
			}
		}
	}

	s.setupRoutes()

	// Initialize Telegram bot if configured
	if cfg != nil && cfg.Notifications.Telegram.Enabled {
		s.telegramService = notifications.NewTelegramService(notifications.TelegramConfig{
			Enabled:  cfg.Notifications.Telegram.Enabled,
			BotToken: cfg.Notifications.Telegram.BotToken,
			ChatID:   cfg.Notifications.Telegram.ChatID,
		})
		go s.StartTelegramBot(context.Background())
	}

	return s
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	r := s.router

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes
	r.Post("/api/v1/auth/login", s.handleLogin)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(s.authMiddleware)

		// Dashboard
		r.Get("/api/v1/dashboard", s.handleDashboard)

		// Tapes
		r.Route("/api/v1/tapes", func(r chi.Router) {
			r.Get("/", s.handleListTapes)
			r.Get("/lto-types", s.handleGetLTOTypes)
			r.Post("/", s.handleCreateTape)
			r.Get("/{id}", s.handleGetTape)
			r.Put("/{id}", s.handleUpdateTape)
			r.Delete("/{id}", s.handleDeleteTape)
			r.Post("/{id}/label", s.handleLabelTape)
			r.Post("/{id}/format", s.handleFormatTape)
			r.Post("/{id}/export", s.handleExportTape)
			r.Post("/{id}/import", s.handleImportTape)
			r.Get("/{id}/read-label", s.handleReadTapeLabel)
			r.Post("/batch-label", s.handleTapesBatchLabel)
			r.Get("/batch-label/status", s.handleBatchLabelStatus)
			r.Post("/batch-label/cancel", s.handleBatchLabelCancel)
			r.Post("/batch-update", s.handleBatchUpdateTapes)
		})

		// Tape Pools
		r.Route("/api/v1/pools", func(r chi.Router) {
			r.Get("/", s.handleListPools)
			r.Post("/", s.handleCreatePool)
			r.Get("/{id}", s.handleGetPool)
			r.Put("/{id}", s.handleUpdatePool)
			r.Delete("/{id}", s.handleDeletePool)
		})

		// Drives
		r.Route("/api/v1/drives", func(r chi.Router) {
			r.Get("/", s.handleListDrives)
			r.Post("/", s.handleCreateDrive)
			r.Get("/scan", s.handleScanDrives)
			r.Get("/{id}/status", s.handleDriveStatus)
			r.Get("/{id}/detect-tape", s.handleDetectTape)
			r.Put("/{id}", s.handleUpdateDrive)
			r.Delete("/{id}", s.handleDeleteDrive)
			r.Post("/{id}/eject", s.handleEjectTape)
			r.Post("/{id}/rewind", s.handleRewindTape)
			r.Post("/{id}/select", s.handleSelectDrive)
			r.Post("/{id}/format-tape", s.handleFormatTapeInDrive)
			r.Get("/{id}/inspect-tape", s.handleInspectTape)
			r.Get("/{id}/scan-for-db-backup", s.handleScanForDBBackup)
			r.Post("/{id}/batch-label", s.handleBatchLabel)
		})

		// Backup Sources
		r.Route("/api/v1/sources", func(r chi.Router) {
			r.Get("/", s.handleListSources)
			r.Post("/", s.handleCreateSource)
			r.Get("/{id}", s.handleGetSource)
			r.Put("/{id}", s.handleUpdateSource)
			r.Delete("/{id}", s.handleDeleteSource)
		})

		// Backup Jobs
		r.Route("/api/v1/jobs", func(r chi.Router) {
			r.Get("/", s.handleListJobs)
			r.Post("/", s.handleCreateJob)
			r.Get("/active", s.handleActiveJobs)
			r.Get("/resumable", s.handleResumableJobs)
			r.Get("/{id}", s.handleGetJob)
			r.Put("/{id}", s.handleUpdateJob)
			r.Delete("/{id}", s.handleDeleteJob)
			r.Post("/{id}/run", s.handleRunJob)
			r.Post("/{id}/cancel", s.handleCancelJob)
			r.Post("/{id}/pause", s.handlePauseJob)
			r.Post("/{id}/resume", s.handleResumeJob)
			r.Post("/{id}/retry", s.handleRetryJob)
			r.Get("/{id}/recommend-tape", s.handleRecommendTape)
		})

		// Backup Sets
		r.Route("/api/v1/backup-sets", func(r chi.Router) {
			r.Get("/", s.handleListBackupSets)
			r.Get("/{id}", s.handleGetBackupSet)
			r.Get("/{id}/files", s.handleListBackupFiles)
			r.Delete("/{id}", s.handleDeleteBackupSet)
		})

		// Catalog
		r.Route("/api/v1/catalog", func(r chi.Router) {
			r.Get("/search", s.handleSearchCatalog)
			r.Get("/browse/{backupSetId}", s.handleBrowseCatalog)
		})

		// Restore
		r.Route("/api/v1/restore", func(r chi.Router) {
			r.Post("/plan", s.handleRestorePlan)
			r.Post("/run", s.handleRunRestore)
		})

		// Logs
		r.Route("/api/v1/logs", func(r chi.Router) {
			r.Get("/audit", s.handleListAuditLogs)
			r.Get("/export", s.handleExportLogs)
		})

		// Users (admin only)
		r.Route("/api/v1/users", func(r chi.Router) {
			r.Use(s.adminOnlyMiddleware)
			r.Get("/", s.handleListUsers)
			r.Post("/", s.handleCreateUser)
			r.Delete("/{id}", s.handleDeleteUser)
		})

		// Password change (any authenticated user)
		r.Post("/api/v1/auth/change-password", s.handleChangePassword)

		// Settings/Config (admin only for write, all authenticated for read)
		r.Route("/api/v1/settings", func(r chi.Router) {
			r.Get("/", s.handleGetConfig)
			r.Group(func(r chi.Router) {
				r.Use(s.adminOnlyMiddleware)
				r.Put("/", s.handleUpdateConfig)
				r.Post("/telegram/test", s.handleTestTelegram)
				r.Post("/restart", s.handleRestart)
			})
		})

		// Events / Notifications
		r.Get("/api/v1/events/stream", s.handleEventStream)
		r.Get("/api/v1/events", s.handleGetNotifications)

		// Documentation
		r.Route("/api/v1/docs", func(r chi.Router) {
			r.Get("/", s.handleListDocs)
			r.Get("/{id}", s.handleGetDoc)
		})

		// Database backup
		r.Route("/api/v1/database-backup", func(r chi.Router) {
			r.Get("/", s.handleListDatabaseBackups)
			r.Post("/backup", s.handleBackupDatabase)
			r.Post("/restore", s.handleRestoreDatabaseBackup)
		})

		// Encryption keys (admin only for management, all authenticated users can list)
		r.Route("/api/v1/encryption-keys", func(r chi.Router) {
			r.Get("/", s.handleListEncryptionKeys)
			r.Get("/keysheet", s.handleGetKeySheet)
			r.Get("/keysheet/text", s.handleGetKeySheetText)
			r.Group(func(r chi.Router) {
				r.Use(s.adminOnlyMiddleware)
				r.Post("/", s.handleCreateEncryptionKey)
				r.Post("/import", s.handleImportEncryptionKey)
				r.Delete("/{id}", s.handleDeleteEncryptionKey)
			})
		})

		// API Keys (admin only)
		r.Route("/api/v1/api-keys", func(r chi.Router) {
			r.Use(s.adminOnlyMiddleware)
			r.Get("/", s.handleListAPIKeys)
			r.Post("/", s.handleCreateAPIKey)
			r.Delete("/{id}", s.handleDeleteAPIKey)
		})

		// Proxmox VE integration
		r.Route("/api/v1/proxmox", func(r chi.Router) {
			// Nodes and discovery
			r.Get("/nodes", s.handleProxmoxListNodes)
			r.Get("/guests", s.handleProxmoxListGuests)
			r.Get("/guests/{vmid}", s.handleProxmoxGetGuest)
			r.Get("/guests/{vmid}/config", s.handleProxmoxGetGuestConfig)

			// Cluster info
			r.Get("/cluster/status", s.handleProxmoxClusterStatus)

			// Backup operations
			r.Get("/backups", s.handleProxmoxListBackups)
			r.Get("/backups/{id}", s.handleProxmoxGetBackup)
			r.Post("/backups", s.handleProxmoxCreateBackup)
			r.Post("/backups/all", s.handleProxmoxBackupAll)

			// Restore operations
			r.Get("/restores", s.handleProxmoxListRestores)
			r.Post("/restores", s.handleProxmoxCreateRestore)
			r.Post("/restores/plan", s.handleProxmoxRestorePlan)

			// Backup jobs (scheduled)
			r.Get("/jobs", s.handleProxmoxListJobs)
			r.Post("/jobs", s.handleProxmoxCreateJob)
			r.Get("/jobs/{id}", s.handleProxmoxGetJob)
			r.Put("/jobs/{id}", s.handleProxmoxUpdateJob)
			r.Delete("/jobs/{id}", s.handleProxmoxDeleteJob)
			r.Post("/jobs/{id}/run", s.handleProxmoxRunJob)
		})

		// Tape Libraries (autochangers)
		r.Route("/api/v1/libraries", func(r chi.Router) {
			r.Get("/", s.handleListLibraries)
			r.Post("/", s.handleCreateLibrary)
			r.Get("/scan", s.handleScanLibraries)
			r.Get("/{id}", s.handleGetLibrary)
			r.Put("/{id}", s.handleUpdateLibrary)
			r.Delete("/{id}", s.handleDeleteLibrary)
			r.Post("/{id}/inventory", s.handleLibraryInventory)
			r.Get("/{id}/slots", s.handleListLibrarySlots)
			r.Post("/{id}/load", s.handleLibraryLoad)
			r.Post("/{id}/unload", s.handleLibraryUnload)
			r.Post("/{id}/transfer", s.handleLibraryTransfer)
		})
	})

	// Health check endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Detailed health check for API v1
	r.Get("/api/v1/health", s.handleHealthCheck)

	// Serve frontend static files
	if s.staticDir != "" {
		// Serve static files with SPA fallback
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			// Don't serve frontend for API routes
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "404 page not found", http.StatusNotFound)
				return
			}

			// Clean the path and ensure it stays within the static directory
			cleanPath := filepath.Clean(r.URL.Path)
			filePath := filepath.Join(s.staticDir, cleanPath)
			absStaticDir, err := filepath.Abs(s.staticDir)
			if err == nil {
				absFilePath, err := filepath.Abs(filePath)
				if err == nil && (strings.HasPrefix(absFilePath, absStaticDir+string(filepath.Separator)) || absFilePath == absStaticDir) {
					if info, err := os.Stat(absFilePath); err == nil && !info.IsDir() {
						http.ServeFile(w, r, absFilePath)
						return
					}
				}
			}

			// Fallback to index.html for SPA routing
			indexPath := filepath.Join(s.staticDir, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}

			http.Error(w, "404 page not found", http.StatusNotFound)
		})
	}
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}

// auditLog records an audit log entry for the given action
func (s *Server) auditLog(r *http.Request, action, resourceType string, resourceID int64, details string) {
	var userID int64
	if claims, ok := r.Context().Value("claims").(*auth.Claims); ok {
		userID = claims.UserID
	}
	ipAddress := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ipAddress = fwd
	}
	s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, action, resourceType, resourceID, details, ipAddress)
}

// StartTelegramBot registers commands and starts polling for Telegram bot interactions
func (s *Server) StartTelegramBot(ctx context.Context) {
	if s.telegramService == nil || !s.telegramService.IsEnabled() {
		return
	}

	// Register commands with Telegram
	s.telegramService.RegisterCommands(ctx)

	// Start polling for commands
	s.telegramService.StartCommandPolling(ctx, func(command, args string) string {
		switch command {
		case "status":
			return s.telegramStatusCommand()
		case "jobs":
			return s.telegramJobsCommand()
		case "tapes":
			return s.telegramTapesCommand()
		case "drives":
			return s.telegramDrivesCommand()
		case "active":
			return s.telegramActiveCommand()
		case "help":
			return "📼 TapeBackarr Commands:\n\n" +
				"/status - System status & loaded tape\n" +
				"/jobs - List backup jobs\n" +
				"/tapes - List tapes\n" +
				"/drives - Drive status\n" +
				"/active - Running operations\n" +
				"/help - This message"
		default:
			return "Unknown command. Use /help to see available commands."
		}
	})
}

func (s *Server) telegramStatusCommand() string {
	var totalTapes, activeTapes, totalJobs, runningJobs int
	s.db.QueryRow("SELECT COUNT(*) FROM tapes").Scan(&totalTapes)
	s.db.QueryRow("SELECT COUNT(*) FROM tapes WHERE status = 'active'").Scan(&activeTapes)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_jobs").Scan(&totalJobs)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE status = 'running'").Scan(&runningJobs)

	msg := "📼 TapeBackarr Status\n\n"
	msg += fmt.Sprintf("Tapes: %d total, %d active\n", totalTapes, activeTapes)
	msg += fmt.Sprintf("Jobs: %d configured, %d running\n", totalJobs, runningJobs)

	// Drive status
	ctx := context.Background()
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := s.tapeService.GetStatus(statusCtx)
	if err != nil {
		msg += "\nDrive: error"
	} else if status.Online {
		msg += "\nDrive: online ✅"
		if cache := s.tapeService.GetLabelCache(); cache != nil {
			if cached := cache.Get(s.tapeService.DevicePath(), 5*time.Minute); cached != nil && cached.Label != nil {
				msg += fmt.Sprintf("\nLoaded tape: %s", cached.Label.Label)
				if cached.Label.Pool != "" {
					msg += fmt.Sprintf(" (pool: %s)", cached.Label.Pool)
				}
			}
		}
	} else {
		msg += "\nDrive: offline"
	}

	// Active jobs
	activeJobs := s.backupService.GetActiveJobs()
	if len(activeJobs) > 0 {
		msg += "\n\n⚡ Active Operations:"
		for _, j := range activeJobs {
			pct := float64(0)
			if j.TotalBytes > 0 {
				pct = float64(j.BytesWritten) / float64(j.TotalBytes) * 100
			}
			msg += fmt.Sprintf("\n  %s: %s (%.1f%%)", j.JobName, j.Phase, pct)
			if j.EstimatedSecondsRemaining > 0 {
				eta := time.Duration(j.EstimatedSecondsRemaining) * time.Second
				msg += fmt.Sprintf(" ETA: %s", eta.Round(time.Second))
			}
		}
	}

	return msg
}

func (s *Server) telegramJobsCommand() string {
	rows, _ := s.db.Query("SELECT name, backup_type, enabled, schedule_cron, last_run_at FROM backup_jobs ORDER BY name LIMIT 20")
	if rows == nil {
		return "Failed to query jobs"
	}
	defer rows.Close()

	msg := "📦 Backup Jobs\n"
	count := 0
	for rows.Next() {
		var name, backupType, cron string
		var enabled bool
		var lastRun *string
		rows.Scan(&name, &backupType, &enabled, &cron, &lastRun)
		status := "✅"
		if !enabled {
			status = "⏸"
		}
		schedStr := "manual"
		if cron != "" {
			schedStr = cron
		}
		msg += fmt.Sprintf("\n%s %s (%s) [%s]", status, name, backupType, schedStr)
		count++
	}
	if count == 0 {
		msg += "\nNo jobs configured"
	}
	return msg
}

func (s *Server) telegramTapesCommand() string {
	rows, _ := s.db.Query("SELECT label, status, used_bytes, capacity_bytes FROM tapes ORDER BY label LIMIT 20")
	if rows == nil {
		return "Failed to query tapes"
	}
	defer rows.Close()

	msg := "💾 Tapes\n"
	count := 0
	for rows.Next() {
		var label, status string
		var used, capacity int64
		rows.Scan(&label, &status, &used, &capacity)
		pct := float64(0)
		if capacity > 0 {
			pct = float64(used) / float64(capacity) * 100
		}
		msg += fmt.Sprintf("\n%s: %s (%.1f%% used)", label, status, pct)
		count++
	}
	if count == 0 {
		msg += "\nNo tapes"
	}
	return msg
}

func (s *Server) telegramDrivesCommand() string {
	rows, _ := s.db.Query("SELECT name, device_path, status FROM tape_drives WHERE enabled = 1")
	if rows == nil {
		return "Failed to query drives"
	}
	defer rows.Close()

	msg := "🔌 Tape Drives\n"
	for rows.Next() {
		var name, devicePath, status string
		rows.Scan(&name, &devicePath, &status)
		msg += fmt.Sprintf("\n%s (%s): %s", name, devicePath, status)
	}
	return msg
}

func (s *Server) telegramActiveCommand() string {
	activeJobs := s.backupService.GetActiveJobs()
	if len(activeJobs) == 0 {
		return "No active operations"
	}

	msg := "⚡ Active Operations\n"
	for _, j := range activeJobs {
		pct := float64(0)
		if j.TotalBytes > 0 {
			pct = float64(j.BytesWritten) / float64(j.TotalBytes) * 100
		}
		msg += fmt.Sprintf("\n%s\n", j.JobName)
		msg += fmt.Sprintf("  Phase: %s | Status: %s\n", j.Phase, j.Status)
		msg += fmt.Sprintf("  Progress: %.1f%%\n", pct)
		if j.WriteSpeed > 0 {
			speedMB := j.WriteSpeed / (1024 * 1024)
			msg += fmt.Sprintf("  Speed: %.1f MB/s\n", speedMB)
		}
		if j.EstimatedSecondsRemaining > 0 {
			eta := time.Duration(j.EstimatedSecondsRemaining) * time.Second
			msg += fmt.Sprintf("  ETA: %s\n", eta.Round(time.Second))
		}
		if j.TapeLabel != "" {
			msg += fmt.Sprintf("  Tape: %s\n", j.TapeLabel)
		}
	}
	return msg
}

// Middleware

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		// Fallback to query parameter for SSE connections (EventSource doesn't support headers)
		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}

		// Check for API key authentication
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" {
			claims, err := s.authService.ValidateAPIKey(apiKey)
			if err != nil {
				s.respondError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			ctx := context.WithValue(r.Context(), "claims", claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if tokenStr == "" {
			s.respondError(w, http.StatusUnauthorized, "missing authorization")
			return
		}

		claims, err := s.authService.ValidateToken(tokenStr)
		if err != nil {
			s.respondError(w, http.StatusUnauthorized, err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), "claims", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) adminOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value("claims").(*auth.Claims)
		if claims.Role != models.RoleAdmin {
			s.respondError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Helper functions

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{"error": message})
}

func (s *Server) getIDParam(r *http.Request) (int64, error) {
	idStr := chi.URLParam(r, "id")
	return strconv.ParseInt(idStr, 10, 64)
}

// Auth handlers

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, user, err := s.authService.Login(req.Username, req.Password)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

// Dashboard handlers

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	type PoolStorageStats struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		TapeCount          int    `json:"tape_count"`
		TotalCapacityBytes int64  `json:"total_capacity_bytes"`
		TotalUsedBytes     int64  `json:"total_used_bytes"`
		TotalFreeBytes     int64  `json:"total_free_bytes"`
	}

	var stats struct {
		TotalTapes            int                `json:"total_tapes"`
		ActiveTapes           int                `json:"active_tapes"`
		TotalJobs             int                `json:"total_jobs"`
		RunningJobs           int                `json:"running_jobs"`
		RecentBackups         int                `json:"recent_backups"`
		DriveStatus           string             `json:"drive_status"`
		TotalDataBytes        int64              `json:"total_data_bytes"`
		LoadedTape            string             `json:"loaded_tape"`
		LoadedTapeUUID        string             `json:"loaded_tape_uuid"`
		LoadedTapePool        string             `json:"loaded_tape_pool"`
		LoadedTapeEncrypted   bool               `json:"loaded_tape_encrypted"`
		LoadedTapeEncKeyFP    string             `json:"loaded_tape_enc_key_fingerprint"`
		LoadedTapeCompression string             `json:"loaded_tape_compression"`
		PoolStorage           []PoolStorageStats `json:"pool_storage"`
	}

	s.db.QueryRow("SELECT COUNT(*) FROM tapes").Scan(&stats.TotalTapes)
	s.db.QueryRow("SELECT COUNT(*) FROM tapes WHERE status = 'active'").Scan(&stats.ActiveTapes)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_jobs").Scan(&stats.TotalJobs)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE status = 'running'").Scan(&stats.RunningJobs)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE start_time > datetime('now', '-24 hours')").Scan(&stats.RecentBackups)
	s.db.QueryRow("SELECT COALESCE(SUM(total_bytes), 0) FROM backup_sets WHERE status = 'completed'").Scan(&stats.TotalDataBytes)

	// Get per-pool storage stats
	stats.PoolStorage = make([]PoolStorageStats, 0)
	poolRows, err := s.db.Query(`
		SELECT tp.id, tp.name,
		       COUNT(t.id) as tape_count,
		       COALESCE(SUM(t.capacity_bytes), 0) as total_capacity_bytes,
		       COALESCE(SUM(t.used_bytes), 0) as total_used_bytes
		FROM tape_pools tp
		LEFT JOIN tapes t ON t.pool_id = tp.id
		GROUP BY tp.id
		ORDER BY tp.name
	`)
	if err == nil {
		defer poolRows.Close()
		for poolRows.Next() {
			var ps PoolStorageStats
			if err := poolRows.Scan(&ps.ID, &ps.Name, &ps.TapeCount, &ps.TotalCapacityBytes, &ps.TotalUsedBytes); err == nil {
				ps.TotalFreeBytes = ps.TotalCapacityBytes - ps.TotalUsedBytes
				stats.PoolStorage = append(stats.PoolStorage, ps)
			}
		}
	}

	// Get drive status and loaded tape label
	ctx := r.Context()
	statusCtx, statusCancel := context.WithTimeout(ctx, 10*time.Second)
	defer statusCancel()
	status, err := s.tapeService.GetStatus(statusCtx)
	if err != nil {
		stats.DriveStatus = "error"
	} else if status.Online {
		stats.DriveStatus = "online"
		// Use cached label data to avoid rewinding the tape on every dashboard load
		if cache := s.tapeService.GetLabelCache(); cache != nil {
			if cached := cache.Get(s.tapeService.DevicePath(), 5*time.Minute); cached != nil {
				if cached.Label != nil {
					stats.LoadedTape = cached.Label.Label
					stats.LoadedTapeUUID = cached.Label.UUID
					stats.LoadedTapePool = cached.Label.Pool
					if cached.Label.EncryptionKeyFingerprint != "" {
						stats.LoadedTapeEncrypted = true
						stats.LoadedTapeEncKeyFP = cached.Label.EncryptionKeyFingerprint
					}
					if cached.Label.CompressionType != "" {
						stats.LoadedTapeCompression = cached.Label.CompressionType
					}
				}
			} else {
				// Cache miss - read label and cache it
				labelCtx, labelCancel := context.WithTimeout(ctx, 5*time.Second)
				defer labelCancel()
				if labelData, err := s.tapeService.ReadTapeLabel(labelCtx); err == nil && labelData != nil {
					stats.LoadedTape = labelData.Label
					stats.LoadedTapeUUID = labelData.UUID
					stats.LoadedTapePool = labelData.Pool
					if labelData.EncryptionKeyFingerprint != "" {
						stats.LoadedTapeEncrypted = true
						stats.LoadedTapeEncKeyFP = labelData.EncryptionKeyFingerprint
					}
					if labelData.CompressionType != "" {
						stats.LoadedTapeCompression = labelData.CompressionType
					}
					cache.Set(s.tapeService.DevicePath(), labelData, true)
				}
			}
		}
	} else {
		stats.DriveStatus = "offline"
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// Tape handlers

func (s *Server) handleListTapes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT t.id, t.uuid, t.barcode, t.label, COALESCE(t.lto_type, '') as lto_type, t.pool_id, tp.name as pool_name, t.status, 
		       t.capacity_bytes, t.used_bytes, t.write_count, t.last_written_at, t.labeled_at, t.created_at,
		       COALESCE(t.encryption_key_fingerprint, '') as encryption_key_fingerprint,
		       COALESCE(t.encryption_key_name, '') as encryption_key_name
		FROM tapes t
		LEFT JOIN tape_pools tp ON t.pool_id = tp.id
		ORDER BY t.label
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	tapes := make([]map[string]interface{}, 0)
	for rows.Next() {
		var t models.Tape
		var poolName *string
		var ltoType string
		var encFingerprint, encKeyName string
		if err := rows.Scan(&t.ID, &t.UUID, &t.Barcode, &t.Label, &ltoType, &t.PoolID, &poolName, &t.Status,
			&t.CapacityBytes, &t.UsedBytes, &t.WriteCount, &t.LastWrittenAt, &t.LabeledAt, &t.CreatedAt,
			&encFingerprint, &encKeyName); err != nil {
			continue
		}
		tape := map[string]interface{}{
			"id":                         t.ID,
			"uuid":                       t.UUID,
			"barcode":                    t.Barcode,
			"label":                      t.Label,
			"lto_type":                   ltoType,
			"pool_id":                    t.PoolID,
			"pool_name":                  poolName,
			"status":                     t.Status,
			"capacity_bytes":             t.CapacityBytes,
			"used_bytes":                 t.UsedBytes,
			"write_count":                t.WriteCount,
			"last_written_at":            t.LastWrittenAt,
			"labeled_at":                 t.LabeledAt,
			"created_at":                 t.CreatedAt,
			"encryption_key_fingerprint": encFingerprint,
			"encryption_key_name":        encKeyName,
		}
		tapes = append(tapes, tape)
	}

	s.respondJSON(w, http.StatusOK, tapes)
}

func (s *Server) handleCreateTape(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Barcode       string `json:"barcode"`
		Label         string `json:"label"`
		PoolID        *int64 `json:"pool_id"`
		LTOType       string `json:"lto_type"`
		CapacityBytes int64  `json:"capacity_bytes"`
		DriveID       *int64 `json:"drive_id"`
		WriteLabel    bool   `json:"write_label"`
		AutoEject     bool   `json:"auto_eject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Label == "" {
		s.respondError(w, http.StatusBadRequest, "label is required")
		return
	}

	// Auto-detect LTO type from drive if not manually set and a drive is specified
	if req.LTOType == "" && req.DriveID != nil {
		var devicePath string
		if err := s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", *req.DriveID).Scan(&devicePath); err == nil {
			ctx := r.Context()
			driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())
			if detectedType, err := driveSvc.DetectTapeType(ctx); err == nil && detectedType != "" {
				req.LTOType = detectedType
			}
		}
	}

	// Auto-infer capacity from LTO type if not manually set
	if req.LTOType != "" {
		if capacity, ok := models.LTOCapacities[req.LTOType]; ok {
			if req.CapacityBytes == 0 {
				req.CapacityBytes = capacity
			}
		}
	}

	// Check if a tape with the same label already exists in the database
	var existingCount int
	s.db.QueryRow("SELECT COUNT(*) FROM tapes WHERE label = ?", req.Label).Scan(&existingCount)
	if existingCount > 0 {
		s.respondError(w, http.StatusConflict, "a tape with this label already exists in the database")
		return
	}

	// Check if a tape with the same barcode already exists (if barcode provided)
	if req.Barcode != "" {
		s.db.QueryRow("SELECT COUNT(*) FROM tapes WHERE barcode = ? AND barcode != ''", req.Barcode).Scan(&existingCount)
		if existingCount > 0 {
			s.respondError(w, http.StatusConflict, "a tape with this barcode already exists in the database")
			return
		}
	}

	// Generate UUID for the tape
	tapeUUID := generateUUID()

	// Get pool name if pool assigned
	poolName := ""
	if req.PoolID != nil {
		_ = s.db.QueryRow("SELECT name FROM tape_pools WHERE id = ?", *req.PoolID).Scan(&poolName)
	}

	// If write_label is requested and a drive is specified, write to physical tape
	if req.WriteLabel && req.DriveID != nil {
		var devicePath string
		err := s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", *req.DriveID).Scan(&devicePath)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "drive not found or not enabled")
			return
		}

		ctx := r.Context()
		driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

		// Verify tape is loaded
		loaded, err := driveSvc.IsTapeLoaded(ctx)
		if err != nil || !loaded {
			s.respondError(w, http.StatusConflict, "no tape loaded in drive - please insert a tape first")
			return
		}

		// Check write protection
		status, err := driveSvc.GetStatus(ctx)
		if err == nil && status.WriteProtect {
			s.respondError(w, http.StatusConflict, "tape is write-protected")
			return
		}

		// Check if tape already has data/label before writing
		if existingLabel, err := driveSvc.ReadTapeLabel(ctx); err == nil && existingLabel != nil && existingLabel.Label != "" {
			s.respondError(w, http.StatusConflict, fmt.Sprintf("tape already has a label: '%s' (UUID: %s). Format the tape first to re-label it.", existingLabel.Label, existingLabel.UUID))
			return
		}

		// Write label to physical tape
		if err := driveSvc.WriteTapeLabel(ctx, req.Label, tapeUUID, poolName); err != nil {
			s.logger.Warn("Failed to write label to physical tape, continuing with software tracking", map[string]interface{}{
				"error": err.Error(),
				"label": req.Label,
			})
		}

		// Auto-eject after labeling if requested
		if req.AutoEject {
			if err := driveSvc.Eject(ctx); err != nil {
				s.logger.Warn("Failed to auto-eject tape after labeling", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}

	result, err := s.db.Exec(`
		INSERT INTO tapes (uuid, barcode, label, pool_id, lto_type, status, capacity_bytes, labeled_at)
		VALUES (?, ?, ?, ?, ?, 'blank', ?, CASE WHEN ? THEN CURRENT_TIMESTAMP ELSE NULL END)
	`, tapeUUID, req.Barcode, req.Label, req.PoolID, req.LTOType, req.CapacityBytes, req.WriteLabel)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Added",
			Message:  fmt.Sprintf("Tape '%s' has been added to the library", req.Label),
			Details: map[string]interface{}{
				"label":    req.Label,
				"uuid":     tapeUUID,
				"lto_type": req.LTOType,
			},
		})
	}

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   id,
		"uuid": tapeUUID,
	})
}

func (s *Server) handleGetTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var t models.Tape
	err = s.db.QueryRow(`
		SELECT id, uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes, 
		       write_count, last_written_at, offsite_location, export_time, import_time, labeled_at, created_at, updated_at
		FROM tapes WHERE id = ?
	`, id).Scan(&t.ID, &t.UUID, &t.Barcode, &t.Label, &t.PoolID, &t.Status, &t.CapacityBytes, &t.UsedBytes,
		&t.WriteCount, &t.LastWrittenAt, &t.OffsiteLocation, &t.ExportTime, &t.ImportTime, &t.LabeledAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}

	s.respondJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var req struct {
		Label           *string            `json:"label"`
		Barcode         *string            `json:"barcode"`
		PoolID          *int64             `json:"pool_id"`
		Status          *models.TapeStatus `json:"status"`
		OffsiteLocation *string            `json:"offsite_location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get current tape state for lifecycle safeguards
	var currentStatus string
	var currentPoolID *int64
	var labeledAt *time.Time
	err = s.db.QueryRow("SELECT status, pool_id, labeled_at FROM tapes WHERE id = ?", id).Scan(&currentStatus, &currentPoolID, &labeledAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}

	// Prevent label changes after the tape has been physically labelled
	if req.Label != nil && labeledAt != nil {
		s.respondError(w, http.StatusConflict, "cannot change label after tape has been physically labelled - the label on tape must match the database")
		return
	}

	// Lifecycle safeguards
	if req.Status != nil {
		newStatus := string(*req.Status)

		// Refuse to overwrite exported tapes
		if currentStatus == "exported" && newStatus != "exported" {
			s.respondError(w, http.StatusConflict, "tape is exported/offsite - import it first before changing status")
			return
		}

		// Refuse to reuse active or unexpired tapes by setting them to blank
		if newStatus == "blank" && (currentStatus == "active" || currentStatus == "full") {
			s.respondError(w, http.StatusConflict, "cannot set active or full tape to blank - use the format/erase endpoint instead")
			return
		}
	}

	// Pool mismatch detection - refuse to change pool if tape has data
	if req.PoolID != nil && currentPoolID != nil && *req.PoolID != *currentPoolID {
		if currentStatus == "active" || currentStatus == "full" {
			s.respondError(w, http.StatusConflict, "cannot change pool of a tape that contains data")
			return
		}
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}

	if req.Label != nil {
		updates = append(updates, "label = ?")
		args = append(args, *req.Label)
	}
	if req.Barcode != nil {
		updates = append(updates, "barcode = ?")
		args = append(args, *req.Barcode)
	}
	if req.PoolID != nil {
		updates = append(updates, "pool_id = ?")
		args = append(args, *req.PoolID)
	}
	if req.Status != nil {
		updates = append(updates, "status = ?")
		args = append(args, *req.Status)
	}
	if req.OffsiteLocation != nil {
		updates = append(updates, "offsite_location = ?")
		args = append(args, *req.OffsiteLocation)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE tapes SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	// Safety: refuse to delete tapes that are active or exported
	var status string
	err = s.db.QueryRow("SELECT status FROM tapes WHERE id = ?", id).Scan(&status)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}
	if status == "active" || status == "exported" {
		s.respondError(w, http.StatusConflict, "cannot delete tape with status '"+status+"' - retire or format it first")
		return
	}

	// Clear foreign key references before deleting the tape
	s.db.Exec("UPDATE tape_drives SET current_tape_id = NULL WHERE current_tape_id = ?", id)
	s.db.Exec("DELETE FROM backup_sets WHERE tape_id = ?", id)
	s.db.Exec("DELETE FROM database_backups WHERE tape_id = ?", id)
	s.db.Exec("UPDATE proxmox_backups SET tape_id = NULL WHERE tape_id = ?", id)
	s.db.Exec("UPDATE proxmox_job_executions SET tape_id = NULL WHERE tape_id = ?", id)

	_, err = s.db.Exec("DELETE FROM tapes WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLog(r, "delete", "tape", id, "Deleted tape")

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleBatchUpdateTapes updates status or pool for multiple tapes at once
func (s *Server) handleBatchUpdateTapes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TapeIDs []int64            `json:"tape_ids"`
		Status  *models.TapeStatus `json:"status"`
		PoolID  *int64             `json:"pool_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.TapeIDs) == 0 {
		s.respondError(w, http.StatusBadRequest, "tape_ids is required")
		return
	}
	if req.Status == nil && req.PoolID == nil {
		s.respondError(w, http.StatusBadRequest, "at least one of status or pool_id is required")
		return
	}

	updated := 0
	skipped := 0
	for _, tapeID := range req.TapeIDs {
		// Get current state
		var currentStatus string
		err := s.db.QueryRow("SELECT status FROM tapes WHERE id = ?", tapeID).Scan(&currentStatus)
		if err != nil {
			skipped++
			continue
		}

		// Apply lifecycle safeguards
		if req.Status != nil {
			newStatus := string(*req.Status)
			if currentStatus == "exported" && newStatus != "exported" {
				skipped++
				continue
			}
			if newStatus == "blank" && (currentStatus == "active" || currentStatus == "full") {
				skipped++
				continue
			}
		}

		updates := []string{}
		args := []interface{}{}
		if req.Status != nil {
			updates = append(updates, "status = ?")
			args = append(args, *req.Status)
		}
		if req.PoolID != nil {
			updates = append(updates, "pool_id = ?")
			args = append(args, *req.PoolID)
		}
		updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
		args = append(args, tapeID)

		query := "UPDATE tapes SET " + strings.Join(updates, ", ") + " WHERE id = ?"
		_, err = s.db.Exec(query, args...)
		if err != nil {
			skipped++
			continue
		}
		updated++
	}

	s.auditLog(r, "batch_update", "tape", 0, fmt.Sprintf("Batch updated %d tapes (skipped %d)", updated, skipped))

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"updated": updated,
		"skipped": skipped,
	})
}

func (s *Server) handleLabelTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var req struct {
		Label     string `json:"label"`
		DriveID   *int64 `json:"drive_id"`
		Force     bool   `json:"force"`
		AutoEject bool   `json:"auto_eject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get tape UUID and pool
	var tapeUUID string
	var poolID *int64
	err = s.db.QueryRow("SELECT uuid, pool_id FROM tapes WHERE id = ?", id).Scan(&tapeUUID, &poolID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}

	poolName := ""
	if poolID != nil {
		_ = s.db.QueryRow("SELECT name FROM tape_pools WHERE id = ?", *poolID).Scan(&poolName)
	}

	// Determine which drive to use
	devicePath := ""
	if req.DriveID != nil {
		err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", *req.DriveID).Scan(&devicePath)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "drive not found or not enabled")
			return
		}
	}

	ctx := r.Context()

	if devicePath != "" {
		driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "tape",
				Title:    "Label Operation Started",
				Message:  fmt.Sprintf("Writing label '%s' to tape on drive %s", req.Label, devicePath),
				Details:  map[string]interface{}{"label": req.Label, "device": devicePath, "uuid": tapeUUID},
			})
		}

		// Verify tape is loaded
		loaded, err := driveSvc.IsTapeLoaded(ctx)
		if err != nil || !loaded {
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "error",
					Category: "tape",
					Title:    "Label Operation Failed",
					Message:  "No tape loaded in drive " + devicePath,
				})
			}
			s.respondError(w, http.StatusConflict, "no tape loaded in drive")
			return
		}

		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "tape",
				Title:    "Tape Detected",
				Message:  fmt.Sprintf("Tape is loaded in drive %s, proceeding with label write", devicePath),
			})
		}

		// Check if tape already has data/label before writing (unless force=true)
		if !req.Force {
			if existingLabel, err := driveSvc.ReadTapeLabel(ctx); err == nil && existingLabel != nil && existingLabel.Label != "" {
				if s.eventBus != nil {
					s.eventBus.Publish(SystemEvent{
						Type:     "warning",
						Category: "tape",
						Title:    "Label Conflict",
						Message:  fmt.Sprintf("Tape already has label '%s' (UUID: %s). Use force to overwrite.", existingLabel.Label, existingLabel.UUID),
					})
				}
				s.respondError(w, http.StatusConflict, fmt.Sprintf("tape already has a label: '%s' (UUID: %s). Use force=true or format the tape first.", existingLabel.Label, existingLabel.UUID))
				return
			}
		}

		// Write label to tape with UUID and pool info
		if err := driveSvc.WriteTapeLabel(ctx, req.Label, tapeUUID, poolName); err != nil {
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "warning",
					Category: "tape",
					Title:    "Physical Label Write Failed",
					Message:  fmt.Sprintf("Could not write label to tape: %s. Continuing with software tracking.", err.Error()),
				})
			}
			s.logger.Warn("Failed to write label to physical tape, continuing with software tracking", map[string]interface{}{
				"error": err.Error(),
			})
		} else if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "success",
				Category: "tape",
				Title:    "Label Written",
				Message:  fmt.Sprintf("Label '%s' written to physical tape on %s", req.Label, devicePath),
			})
		}

		// Auto-eject after labeling if requested
		if req.AutoEject {
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "info",
					Category: "tape",
					Title:    "Auto-Eject",
					Message:  fmt.Sprintf("Ejecting tape from drive %s after labeling", devicePath),
				})
			}
			if err := driveSvc.Eject(ctx); err != nil {
				if s.eventBus != nil {
					s.eventBus.Publish(SystemEvent{
						Type:     "warning",
						Category: "tape",
						Title:    "Auto-Eject Failed",
						Message:  fmt.Sprintf("Failed to auto-eject tape: %s", err.Error()),
					})
				}
				s.logger.Warn("Failed to auto-eject tape after labeling", map[string]interface{}{
					"error": err.Error(),
				})
			} else if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "success",
					Category: "tape",
					Title:    "Tape Ejected",
					Message:  fmt.Sprintf("Tape ejected from drive %s", devicePath),
				})
			}
		}
	} else {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "tape",
				Title:    "Label Operation Started",
				Message:  fmt.Sprintf("Writing label '%s' to tape (default drive)", req.Label),
			})
		}
		// Use default tape service
		if err := s.tapeService.WriteTapeLabel(ctx, req.Label, tapeUUID, poolName); err != nil {
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "warning",
					Category: "tape",
					Title:    "Physical Label Write Failed",
					Message:  fmt.Sprintf("Could not write label to tape: %s. Continuing with software tracking.", err.Error()),
				})
			}
			s.logger.Warn("Failed to write label to physical tape, continuing with software tracking", map[string]interface{}{
				"error": err.Error(),
			})
		} else if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "success",
				Category: "tape",
				Title:    "Label Written",
				Message:  fmt.Sprintf("Label '%s' written to physical tape", req.Label),
			})
		}
	}

	// Update database
	_, err = s.db.Exec("UPDATE tapes SET label = ?, labeled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.Label, id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Labeled",
			Message:  fmt.Sprintf("Tape labeled as '%s' (UUID: %s)", req.Label, tapeUUID),
			Details:  map[string]interface{}{"label": req.Label, "uuid": tapeUUID, "pool": poolName},
		})
	}

	s.auditLog(r, "label", "tape", id, fmt.Sprintf("Labelled tape as '%s'", req.Label))

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "labeled"})
}

// Pool handlers

func (s *Server) handleListPools(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT tp.id, tp.name, tp.description, tp.retention_days, tp.allow_reuse, tp.allocation_policy, tp.created_at,
		       COUNT(t.id) as tape_count,
		       COALESCE(SUM(t.capacity_bytes), 0) as total_capacity_bytes,
		       COALESCE(SUM(t.used_bytes), 0) as total_used_bytes
		FROM tape_pools tp
		LEFT JOIN tapes t ON t.pool_id = tp.id
		GROUP BY tp.id
		ORDER BY tp.name
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	pools := make([]map[string]interface{}, 0)
	for rows.Next() {
		var p models.TapePool
		var tapeCount int
		var totalCapacity, totalUsed int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RetentionDays, &p.AllowReuse, &p.AllocationPolicy, &p.CreatedAt, &tapeCount, &totalCapacity, &totalUsed); err != nil {
			continue
		}
		pools = append(pools, map[string]interface{}{
			"id":                   p.ID,
			"name":                 p.Name,
			"description":          p.Description,
			"retention_days":       p.RetentionDays,
			"allow_reuse":          p.AllowReuse,
			"allocation_policy":    p.AllocationPolicy,
			"tape_count":           tapeCount,
			"total_capacity_bytes": totalCapacity,
			"total_used_bytes":     totalUsed,
			"total_free_bytes":     totalCapacity - totalUsed,
			"created_at":           p.CreatedAt,
		})
	}

	s.respondJSON(w, http.StatusOK, pools)
}

func (s *Server) handleCreatePool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string `json:"name"`
		Description      string `json:"description"`
		RetentionDays    int    `json:"retention_days"`
		AllowReuse       *bool  `json:"allow_reuse"`
		AllocationPolicy string `json:"allocation_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowReuse := true
	if req.AllowReuse != nil {
		allowReuse = *req.AllowReuse
	}
	if req.AllocationPolicy == "" {
		req.AllocationPolicy = "continue"
	}

	result, err := s.db.Exec(`
		INSERT INTO tape_pools (name, description, retention_days, allow_reuse, allocation_policy)
		VALUES (?, ?, ?, ?, ?)
	`, req.Name, req.Description, req.RetentionDays, allowReuse, req.AllocationPolicy)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	s.respondJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleGetPool(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid pool id")
		return
	}

	var p models.TapePool
	err = s.db.QueryRow(`
		SELECT id, name, description, retention_days, allow_reuse, allocation_policy, created_at, updated_at
		FROM tape_pools WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.RetentionDays, &p.AllowReuse, &p.AllocationPolicy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "pool not found")
		return
	}

	// Fetch storage statistics for the pool
	var tapeCount int
	var totalCapacity, totalUsed int64
	s.db.QueryRow(`
		SELECT COUNT(id), COALESCE(SUM(capacity_bytes), 0), COALESCE(SUM(used_bytes), 0)
		FROM tapes WHERE pool_id = ?
	`, id).Scan(&tapeCount, &totalCapacity, &totalUsed)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   p.ID,
		"name":                 p.Name,
		"description":          p.Description,
		"retention_days":       p.RetentionDays,
		"allow_reuse":          p.AllowReuse,
		"allocation_policy":    p.AllocationPolicy,
		"tape_count":           tapeCount,
		"total_capacity_bytes": totalCapacity,
		"total_used_bytes":     totalUsed,
		"total_free_bytes":     totalCapacity - totalUsed,
		"created_at":           p.CreatedAt,
		"updated_at":           p.UpdatedAt,
	})
}

func (s *Server) handleUpdatePool(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid pool id")
		return
	}

	var req struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		RetentionDays    *int    `json:"retention_days"`
		AllowReuse       *bool   `json:"allow_reuse"`
		AllocationPolicy *string `json:"allocation_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		updates = append(updates, "description = ?")
		args = append(args, *req.Description)
	}
	if req.RetentionDays != nil {
		updates = append(updates, "retention_days = ?")
		args = append(args, *req.RetentionDays)
	}
	if req.AllowReuse != nil {
		updates = append(updates, "allow_reuse = ?")
		args = append(args, *req.AllowReuse)
	}
	if req.AllocationPolicy != nil {
		updates = append(updates, "allocation_policy = ?")
		args = append(args, *req.AllocationPolicy)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE tape_pools SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeletePool(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid pool id")
		return
	}

	_, err = s.db.Exec("DELETE FROM tape_pools WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Drive handlers

func (s *Server) handleListDrives(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT id, device_path, COALESCE(display_name, '') as display_name, COALESCE(vendor, '') as vendor,
		       COALESCE(serial_number, '') as serial_number, COALESCE(model, '') as model, status, current_tape_id, COALESCE(enabled, 1) as enabled, created_at
		FROM tape_drives ORDER BY device_path
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	drives := make([]models.TapeDrive, 0)
	for rows.Next() {
		var d models.TapeDrive
		if err := rows.Scan(&d.ID, &d.DevicePath, &d.DisplayName, &d.Vendor, &d.SerialNumber, &d.Model, &d.Status, &d.CurrentTapeID, &d.Enabled, &d.CreatedAt); err != nil {
			continue
		}
		drives = append(drives, d)
	}

	// Probe live status for each enabled drive
	ctx := r.Context()
	for i, d := range drives {
		if !d.Enabled {
			continue
		}

		// Skip hardware probing if drive is busy (e.g., during backup)
		// The status was already set to 'busy' by the backup service
		if d.Status == models.DriveStatusBusy {
			// Resolve tape label from DB
			if d.CurrentTapeID != nil {
				var tapeLabel string
				if err := s.db.QueryRow("SELECT label FROM tapes WHERE id = ?", *d.CurrentTapeID).Scan(&tapeLabel); err == nil {
					drives[i].CurrentTape = tapeLabel
				}
			}
			continue
		}

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		driveSvc := tape.NewServiceForDevice(d.DevicePath, s.tapeService.GetBlockSize())
		hwStatus, err := driveSvc.GetStatus(probeCtx)
		cancel()
		if err != nil || hwStatus.Error != "" {
			drives[i].Status = models.DriveStatusOffline
		} else if hwStatus.Online {
			drives[i].Status = models.DriveStatusReady

			// Use cached label data to avoid rewinding tape on every list
			var labelData *tape.TapeLabelData
			if mainCache := s.tapeService.GetLabelCache(); mainCache != nil {
				if cached := mainCache.Get(d.DevicePath, 5*time.Minute); cached != nil {
					labelData = cached.Label
				}
			}
			if labelData == nil {
				// Cache miss - read label and cache it
				labelCtx, labelCancel := context.WithTimeout(ctx, 5*time.Second)
				if ld, err := driveSvc.ReadTapeLabel(labelCtx); err == nil && ld != nil {
					labelData = ld
					if mainCache := s.tapeService.GetLabelCache(); mainCache != nil {
						mainCache.Set(d.DevicePath, ld, true)
					}
				}
				labelCancel()
			}
			if labelData != nil {
				drives[i].CurrentTape = labelData.Label
				// Try to match by UUID first, then by label
				var tapeID int64
				found := false
				if labelData.UUID != "" {
					if err := s.db.QueryRow("SELECT id FROM tapes WHERE uuid = ?", labelData.UUID).Scan(&tapeID); err == nil {
						drives[i].CurrentTapeID = &tapeID
						found = true
					}
				}
				if !found {
					if err := s.db.QueryRow("SELECT id FROM tapes WHERE label = ?", labelData.Label).Scan(&tapeID); err == nil {
						drives[i].CurrentTapeID = &tapeID
						found = true
					}
				}
				if !found {
					drives[i].UnknownTape = &models.UnknownTapeInfo{
						Label:     labelData.Label,
						UUID:      labelData.UUID,
						Pool:      labelData.Pool,
						Timestamp: labelData.Timestamp,
					}
					if s.eventBus != nil {
						s.eventBus.Publish(SystemEvent{
							Type:     "warning",
							Category: "tape",
							Title:    "Unknown Tape Detected",
							Message:  fmt.Sprintf("Tape '%s' (UUID: %s) is loaded in drive but not in database", labelData.Label, labelData.UUID),
							Details: map[string]interface{}{
								"label":    labelData.Label,
								"uuid":     labelData.UUID,
								"pool":     labelData.Pool,
								"drive_id": d.ID,
							},
						})
					}
				}
			}

			// Try to get vendor/model info if missing
			if d.Vendor == "" || d.Model == "" {
				infoCtx, infoCancel := context.WithTimeout(ctx, 3*time.Second)
				if info, err := driveSvc.GetDriveInfo(infoCtx); err == nil {
					if v, ok := info["Vendor identification"]; ok && d.Vendor == "" {
						drives[i].Vendor = v
					}
					if v, ok := info["Product identification"]; ok && d.Model == "" {
						drives[i].Model = v
					}
					if v, ok := info["Unit serial number"]; ok && d.SerialNumber == "" {
						drives[i].SerialNumber = v
					}
				}
				infoCancel()
			}
		} else {
			drives[i].Status = models.DriveStatusOffline
		}

		// Update the DB with the probed status
		s.db.Exec(`UPDATE tape_drives SET status = ?, vendor = ?, model = ?, serial_number = ?, current_tape_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			string(drives[i].Status), drives[i].Vendor, drives[i].Model, drives[i].SerialNumber, drives[i].CurrentTapeID, d.ID)
	}

	s.respondJSON(w, http.StatusOK, drives)
}

func (s *Server) handleDriveStatus(w http.ResponseWriter, r *http.Request) {
	driveID, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	// Check if drive is busy (backup in progress) - return cached status
	var driveStatus string
	if err := s.db.QueryRow("SELECT status FROM tape_drives WHERE id = ?", driveID).Scan(&driveStatus); err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found")
		return
	}
	if driveStatus == string(models.DriveStatusBusy) {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"ready":  true,
			"online": true,
			"busy":   true,
			"error":  "",
		})
		return
	}

	ctx := r.Context()
	status, err := s.tapeService.GetStatus(ctx)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleDetectTape(w http.ResponseWriter, r *http.Request) {
	driveID, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", driveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found or not enabled")
		return
	}

	ctx := r.Context()
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	status, err := driveSvc.GetStatus(ctx)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get drive status: "+err.Error())
		return
	}

	if status.Error != "" || !status.Online || !status.Ready {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"loaded":   false,
			"lto_type": "",
			"density":  "",
		})
		return
	}

	// Determine LTO type: first from mt description, then from density code lookup
	ltoType := status.DriveType
	if ltoType == "" && status.Density != "" {
		if detected, ok := models.LTOTypeFromDensity(status.Density); ok {
			ltoType = detected
		}
	}

	var capacityBytes int64
	if ltoType != "" {
		if cap, ok := models.LTOCapacities[ltoType]; ok {
			capacityBytes = cap
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"loaded":         true,
		"lto_type":       ltoType,
		"density":        status.Density,
		"capacity_bytes": capacityBytes,
	})
}

func (s *Server) handleEjectTape(w http.ResponseWriter, r *http.Request) {
	driveID, _ := s.getIDParam(r)
	ctx := r.Context()
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Eject Started",
			Message:  "Ejecting tape from drive...",
		})
	}
	if err := s.tapeService.Eject(ctx); err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Eject Failed",
				Message:  fmt.Sprintf("Failed to eject tape: %s", err.Error()),
			})
		}
		s.respondError(w, http.StatusInternalServerError, "failed to eject tape: "+err.Error())
		return
	}

	if cache := s.tapeService.GetLabelCache(); cache != nil {
		cache.Invalidate(s.tapeService.DevicePath())
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Ejected",
			Message:  "Tape has been ejected from the drive",
		})
	}
	s.auditLog(r, "eject", "tape_drive", driveID, "Ejected tape")
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ejected"})
}

func (s *Server) handleRewindTape(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Rewind Started",
			Message:  "Rewinding tape to beginning...",
		})
	}
	if err := s.tapeService.Rewind(ctx); err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Rewind Failed",
				Message:  fmt.Sprintf("Failed to rewind tape: %s", err.Error()),
			})
		}
		s.respondError(w, http.StatusInternalServerError, "failed to rewind tape: "+err.Error())
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Rewound",
			Message:  "Tape has been rewound to the beginning",
		})
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "rewound"})
}

// Source handlers

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT id, name, source_type, path, COALESCE(include_patterns, '[]'), COALESCE(exclude_patterns, '[]'), enabled, created_at
		FROM backup_sources ORDER BY name
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	sources := make([]models.BackupSource, 0)
	for rows.Next() {
		var src models.BackupSource
		if err := rows.Scan(&src.ID, &src.Name, &src.SourceType, &src.Path, &src.IncludePatterns, &src.ExcludePatterns, &src.Enabled, &src.CreatedAt); err != nil {
			continue
		}
		sources = append(sources, src)
	}

	s.respondJSON(w, http.StatusOK, sources)
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string   `json:"name"`
		SourceType      string   `json:"source_type"`
		Path            string   `json:"path"`
		IncludePatterns []string `json:"include_patterns"`
		ExcludePatterns []string `json:"exclude_patterns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	includeJSON, _ := json.Marshal(req.IncludePatterns)
	excludeJSON, _ := json.Marshal(req.ExcludePatterns)

	result, err := s.db.Exec(`
		INSERT INTO backup_sources (name, source_type, path, include_patterns, exclude_patterns, enabled)
		VALUES (?, ?, ?, ?, ?, 1)
	`, req.Name, req.SourceType, req.Path, string(includeJSON), string(excludeJSON))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "source",
			Title:    "Source Created",
			Message:  fmt.Sprintf("Backup source '%s' created", req.Name),
		})
	}

	s.auditLog(r, "create", "backup_source", id, fmt.Sprintf("Created source '%s'", req.Name))

	s.respondJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid source id")
		return
	}

	var src models.BackupSource
	err = s.db.QueryRow(`
		SELECT id, name, source_type, path, include_patterns, exclude_patterns, enabled, created_at, updated_at
		FROM backup_sources WHERE id = ?
	`, id).Scan(&src.ID, &src.Name, &src.SourceType, &src.Path, &src.IncludePatterns, &src.ExcludePatterns, &src.Enabled, &src.CreatedAt, &src.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "source not found")
		return
	}

	s.respondJSON(w, http.StatusOK, src)
}

func (s *Server) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid source id")
		return
	}

	var req struct {
		Name            *string  `json:"name"`
		Path            *string  `json:"path"`
		IncludePatterns []string `json:"include_patterns"`
		ExcludePatterns []string `json:"exclude_patterns"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Path != nil {
		updates = append(updates, "path = ?")
		args = append(args, *req.Path)
	}
	if req.IncludePatterns != nil {
		includeJSON, _ := json.Marshal(req.IncludePatterns)
		updates = append(updates, "include_patterns = ?")
		args = append(args, string(includeJSON))
	}
	if req.ExcludePatterns != nil {
		excludeJSON, _ := json.Marshal(req.ExcludePatterns)
		updates = append(updates, "exclude_patterns = ?")
		args = append(args, string(excludeJSON))
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE backup_sources SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLog(r, "update", "backup_source", id, "Updated source settings")

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid source id")
		return
	}

	_, err = s.db.Exec("DELETE FROM backup_sources WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "source",
			Title:    "Source Deleted",
			Message:  fmt.Sprintf("Backup source %d deleted", id),
		})
	}

	s.auditLog(r, "delete", "backup_source", id, "Deleted source")

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Job handlers

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT j.id, j.name, j.source_id, s.name as source_name, j.pool_id, p.name as pool_name,
		       j.backup_type, j.schedule_cron, j.retention_days, j.enabled,
		       j.encryption_enabled, j.encryption_key_id,
		       COALESCE(j.compression, 'none') as compression,
		       j.last_run_at, j.next_run_at
		FROM backup_jobs j
		LEFT JOIN backup_sources s ON j.source_id = s.id
		LEFT JOIN tape_pools p ON j.pool_id = p.id
		ORDER BY j.name
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	jobs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var j models.BackupJob
		var sourceName, poolName *string
		var compression string
		if err := rows.Scan(&j.ID, &j.Name, &j.SourceID, &sourceName, &j.PoolID, &poolName,
			&j.BackupType, &j.ScheduleCron, &j.RetentionDays, &j.Enabled,
			&j.EncryptionEnabled, &j.EncryptionKeyID,
			&compression,
			&j.LastRunAt, &j.NextRunAt); err != nil {
			continue
		}
		job := map[string]interface{}{
			"id":                 j.ID,
			"name":               j.Name,
			"source_id":          j.SourceID,
			"source_name":        sourceName,
			"pool_id":            j.PoolID,
			"pool_name":          poolName,
			"backup_type":        j.BackupType,
			"schedule_cron":      j.ScheduleCron,
			"retention_days":     j.RetentionDays,
			"enabled":            j.Enabled,
			"encryption_enabled": j.EncryptionEnabled,
			"encryption_key_id":  j.EncryptionKeyID,
			"compression":        compression,
			"last_run_at":        j.LastRunAt,
			"next_run_at":        j.NextRunAt,
		}
		jobs = append(jobs, job)
	}

	s.respondJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		SourceID        int64  `json:"source_id"`
		PoolID          int64  `json:"pool_id"`
		BackupType      string `json:"backup_type"`
		ScheduleCron    string `json:"schedule_cron"`
		RetentionDays   int    `json:"retention_days"`
		EncryptionKeyID *int64 `json:"encryption_key_id"`
		Compression     string `json:"compression"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate cron expression if provided
	if req.ScheduleCron != "" {
		if err := scheduler.ParseCron(req.ScheduleCron); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid cron expression: "+err.Error())
			return
		}
	}

	// Determine encryption settings
	encryptionEnabled := false
	if req.EncryptionKeyID != nil && *req.EncryptionKeyID > 0 {
		// Validate the key exists
		_, err := s.encryptionService.GetKey(r.Context(), *req.EncryptionKeyID)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "encryption key not found")
			return
		}
		encryptionEnabled = true
	}

	compression := req.Compression
	if compression == "" {
		compression = "none"
	}

	result, err := s.db.Exec(`
		INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, schedule_cron, retention_days, enabled, encryption_enabled, encryption_key_id, compression)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`, req.Name, req.SourceID, req.PoolID, req.BackupType, req.ScheduleCron, req.RetentionDays, encryptionEnabled, req.EncryptionKeyID, compression)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()

	// Add to scheduler if cron is set
	if req.ScheduleCron != "" {
		job := &models.BackupJob{
			ID:           id,
			Name:         req.Name,
			SourceID:     req.SourceID,
			PoolID:       req.PoolID,
			BackupType:   models.BackupType(req.BackupType),
			ScheduleCron: req.ScheduleCron,
			Enabled:      true,
		}
		s.scheduler.AddJob(job)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "job",
			Title:    "Job Created",
			Message:  fmt.Sprintf("Backup job '%s' created", req.Name),
		})
	}

	s.auditLog(r, "create", "backup_job", id, fmt.Sprintf("Created job '%s'", req.Name))

	s.respondJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var j models.BackupJob
	err = s.db.QueryRow(`
		SELECT id, name, source_id, pool_id, backup_type, schedule_cron, retention_days, 
		       enabled, last_run_at, next_run_at, created_at, updated_at
		FROM backup_jobs WHERE id = ?
	`, id).Scan(&j.ID, &j.Name, &j.SourceID, &j.PoolID, &j.BackupType, &j.ScheduleCron, &j.RetentionDays,
		&j.Enabled, &j.LastRunAt, &j.NextRunAt, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "job not found")
		return
	}

	s.respondJSON(w, http.StatusOK, j)
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var req struct {
		Name            *string `json:"name"`
		SourceID        *int64  `json:"source_id"`
		PoolID          *int64  `json:"pool_id"`
		BackupType      *string `json:"backup_type"`
		ScheduleCron    *string `json:"schedule_cron"`
		RetentionDays   *int    `json:"retention_days"`
		Enabled         *bool   `json:"enabled"`
		EncryptionKeyID *int64  `json:"encryption_key_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Compression and encryption settings cannot be changed after creation
	if req.EncryptionKeyID != nil {
		s.respondError(w, http.StatusBadRequest, "encryption settings cannot be changed after job creation")
		return
	}

	if req.ScheduleCron != nil && *req.ScheduleCron != "" {
		if err := scheduler.ParseCron(*req.ScheduleCron); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid cron expression: "+err.Error())
			return
		}
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.SourceID != nil {
		updates = append(updates, "source_id = ?")
		args = append(args, *req.SourceID)
	}
	if req.PoolID != nil {
		updates = append(updates, "pool_id = ?")
		args = append(args, *req.PoolID)
	}
	if req.BackupType != nil {
		updates = append(updates, "backup_type = ?")
		args = append(args, *req.BackupType)
	}
	if req.ScheduleCron != nil {
		updates = append(updates, "schedule_cron = ?")
		args = append(args, *req.ScheduleCron)
	}
	if req.RetentionDays != nil {
		updates = append(updates, "retention_days = ?")
		args = append(args, *req.RetentionDays)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE backup_jobs SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Reload scheduler
	s.scheduler.ReloadJobs()

	s.auditLog(r, "update", "backup_job", id, "Updated job settings")

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	_, err = s.db.Exec("DELETE FROM backup_jobs WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.scheduler.RemoveJob(id)

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "job",
			Title:    "Job Deleted",
			Message:  fmt.Sprintf("Backup job %d deleted", id),
		})
	}

	s.auditLog(r, "delete", "backup_job", id, "Deleted job")

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleRunJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var req struct {
		TapeID     int64  `json:"tape_id"`
		UsePool    *bool  `json:"use_pool"`    // If true, select tape from pool (default behavior)
		BackupType string `json:"backup_type"` // Override job's backup type
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get job details
	var job models.BackupJob
	err = s.db.QueryRow(`
		SELECT id, name, source_id, pool_id, backup_type, retention_days, encryption_enabled, encryption_key_id
		FROM backup_jobs WHERE id = ?
	`, id).Scan(&job.ID, &job.Name, &job.SourceID, &job.PoolID, &job.BackupType, &job.RetentionDays, &job.EncryptionEnabled, &job.EncryptionKeyID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "job not found")
		return
	}

	// Get source details
	var source models.BackupSource
	err = s.db.QueryRow(`
		SELECT id, name, source_type, path, include_patterns, exclude_patterns
		FROM backup_sources WHERE id = ?
	`, job.SourceID).Scan(&source.ID, &source.Name, &source.SourceType, &source.Path, &source.IncludePatterns, &source.ExcludePatterns)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "source not found")
		return
	}

	// Determine backup type
	backupType := job.BackupType
	if req.BackupType != "" {
		backupType = models.BackupType(req.BackupType)
	}

	// Determine tape to use
	tapeID := req.TapeID

	// Default to pool-based selection when no tape_id is provided
	usePool := req.TapeID == 0
	if req.UsePool != nil {
		usePool = *req.UsePool
	}

	if usePool && job.PoolID > 0 {
		// Select best tape from pool
		selectedTapeID, tapeLabel, err := s.selectTapeFromPool(job.PoolID, job.RetentionDays)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("no suitable tape found in pool: %v", err))
			return
		}
		tapeID = selectedTapeID

		// Run backup in background
		go func() {
			ctx := context.Background()
			s.backupService.RunBackup(ctx, &job, &source, tapeID, backupType)
		}()

		s.auditLog(r, "run", "backup_job", id, "Started backup job")

		s.respondJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":     "started",
			"message":    fmt.Sprintf("Backup job started using tape %s from pool", tapeLabel),
			"tape_id":    tapeID,
			"tape_label": tapeLabel,
		})
		return
	}

	if tapeID == 0 {
		s.respondError(w, http.StatusBadRequest, "tape_id is required when not using pool-based selection")
		return
	}

	// Run backup in background with explicit tape
	go func() {
		ctx := context.Background()
		s.backupService.RunBackup(ctx, &job, &source, tapeID, backupType)
	}()

	s.auditLog(r, "run", "backup_job", id, "Started backup job")

	s.respondJSON(w, http.StatusAccepted, map[string]string{
		"status":  "started",
		"message": "Backup job started in background",
	})
}

// selectTapeFromPool picks the best tape from a pool based on status, available space, and retention.
// It prefers active tapes with remaining space, then blank tapes.
func (s *Server) selectTapeFromPool(poolID int64, retentionDays int) (int64, string, error) {
	// First, try to find an active tape in the pool with remaining capacity
	var tapeID int64
	var tapeLabel string
	err := s.db.QueryRow(`
		SELECT id, label FROM tapes
		WHERE pool_id = ? AND status = 'active' AND (capacity_bytes - used_bytes) > 0
		ORDER BY used_bytes ASC
		LIMIT 1
	`, poolID).Scan(&tapeID, &tapeLabel)
	if err == nil {
		return tapeID, tapeLabel, nil
	}

	// Next, try a blank tape in the pool
	err = s.db.QueryRow(`
		SELECT id, label FROM tapes
		WHERE pool_id = ? AND status = 'blank'
		ORDER BY created_at ASC
		LIMIT 1
	`, poolID).Scan(&tapeID, &tapeLabel)
	if err == nil {
		return tapeID, tapeLabel, nil
	}

	// Check if there are expired tapes that can be reused
	var allowReuse bool
	_ = s.db.QueryRow("SELECT allow_reuse FROM tape_pools WHERE id = ?", poolID).Scan(&allowReuse)
	if allowReuse {
		err = s.db.QueryRow(`
			SELECT id, label FROM tapes
			WHERE pool_id = ? AND status = 'expired'
			ORDER BY last_written_at ASC
			LIMIT 1
		`, poolID).Scan(&tapeID, &tapeLabel)
		if err == nil {
			return tapeID, tapeLabel, nil
		}
	}

	return 0, "", errors.New("no available tapes in pool (need blank, active with space, or expired reusable tapes)")
}

// handleRecommendTape recommends the best tape from a job's pool for backup
func (s *Server) handleRecommendTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	// Get job details
	var poolID int64
	var retentionDays int
	err = s.db.QueryRow(`
		SELECT pool_id, retention_days FROM backup_jobs WHERE id = ?
	`, id).Scan(&poolID, &retentionDays)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "job not found")
		return
	}

	// Get pool info
	var poolName string
	_ = s.db.QueryRow("SELECT name FROM tape_pools WHERE id = ?", poolID).Scan(&poolName)

	// Select best tape
	tapeID, tapeLabel, err := s.selectTapeFromPool(poolID, retentionDays)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"found":     false,
			"pool_id":   poolID,
			"pool_name": poolName,
			"message":   fmt.Sprintf("No suitable tape found: %v", err),
		})
		return
	}

	// Get tape details
	var tapeStatus string
	var capacityBytes, usedBytes int64
	_ = s.db.QueryRow("SELECT status, capacity_bytes, used_bytes FROM tapes WHERE id = ?", tapeID).Scan(&tapeStatus, &capacityBytes, &usedBytes)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"found":          true,
		"tape_id":        tapeID,
		"tape_label":     tapeLabel,
		"tape_status":    tapeStatus,
		"capacity_bytes": capacityBytes,
		"used_bytes":     usedBytes,
		"pool_id":        poolID,
		"pool_name":      poolName,
		"message":        fmt.Sprintf("Please load tape %s into the drive", tapeLabel),
	})
}

func (s *Server) handleActiveJobs(w http.ResponseWriter, r *http.Request) {
	activeJobs := s.backupService.GetActiveJobs()
	s.respondJSON(w, http.StatusOK, activeJobs)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	if s.backupService.CancelJob(id) {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "warning",
				Category: "backup",
				Title:    "Job Cancelled",
				Message:  fmt.Sprintf("Backup job %d was cancelled by user", id),
			})
		}
		s.respondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	} else {
		s.respondError(w, http.StatusNotFound, "no active job found with that id")
	}
}

func (s *Server) handlePauseJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	if s.backupService.PauseJob(id) {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "backup",
				Title:    "Job Paused",
				Message:  fmt.Sprintf("Backup job %d was paused by user", id),
			})
		}
		s.respondJSON(w, http.StatusOK, map[string]string{"status": "paused"})
	} else {
		s.respondError(w, http.StatusNotFound, "no active job found with that id")
	}
}

func (s *Server) handleResumeJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	if s.backupService.ResumeJob(id) {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "backup",
				Title:    "Job Resumed",
				Message:  fmt.Sprintf("Backup job %d was resumed by user", id),
			})
		}
		s.respondJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
	} else {
		s.respondError(w, http.StatusNotFound, "no active job found with that id")
	}
}

// handleRetryJob retries a failed or paused backup job, optionally resuming from where it left off
func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var req struct {
		TapeID      int64 `json:"tape_id"`       // Optional: use a different tape
		UsePool     *bool `json:"use_pool"`       // If true, select tape from pool
		FromScratch bool  `json:"from_scratch"`   // If true, start fresh instead of resuming
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Get job details
	var job models.BackupJob
	err = s.db.QueryRow(`
		SELECT id, name, source_id, pool_id, backup_type, retention_days, encryption_enabled, encryption_key_id, compression
		FROM backup_jobs WHERE id = ?
	`, id).Scan(&job.ID, &job.Name, &job.SourceID, &job.PoolID, &job.BackupType, &job.RetentionDays, &job.EncryptionEnabled, &job.EncryptionKeyID, &job.Compression)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "job not found")
		return
	}

	// Get source details
	var source models.BackupSource
	err = s.db.QueryRow(`
		SELECT id, name, source_type, path, include_patterns, exclude_patterns
		FROM backup_sources WHERE id = ?
	`, job.SourceID).Scan(&source.ID, &source.Name, &source.SourceType, &source.Path, &source.IncludePatterns, &source.ExcludePatterns)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "source not found")
		return
	}

	// Look for a resumable execution if not starting from scratch
	var resumeState string
	if !req.FromScratch {
		_ = s.db.QueryRow(`
			SELECT resume_state FROM job_executions
			WHERE job_id = ? AND can_resume = 1 AND status IN ('paused', 'failed')
			ORDER BY created_at DESC LIMIT 1
		`, id).Scan(&resumeState)
	}

	// Determine tape to use
	tapeID := req.TapeID
	usePool := req.TapeID == 0
	if req.UsePool != nil {
		usePool = *req.UsePool
	}

	var tapeLabel string
	if usePool && job.PoolID > 0 {
		selectedTapeID, selectedLabel, err := s.selectTapeFromPool(job.PoolID, job.RetentionDays)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("no suitable tape found in pool: %v", err))
			return
		}
		tapeID = selectedTapeID
		tapeLabel = selectedLabel
	} else if tapeID == 0 {
		s.respondError(w, http.StatusBadRequest, "tape_id is required when not using pool-based selection")
		return
	} else {
		_ = s.db.QueryRow("SELECT label FROM tapes WHERE id = ?", tapeID).Scan(&tapeLabel)
	}

	// Mark previous failed/paused executions as superseded
	s.db.Exec(`
		UPDATE job_executions SET can_resume = 0
		WHERE job_id = ? AND can_resume = 1 AND status IN ('paused', 'failed')
	`, id)

	// Run backup in background with optional resume state
	go func() {
		ctx := context.Background()
		if resumeState != "" {
			s.backupService.RunBackupWithResume(ctx, &job, &source, tapeID, job.BackupType, resumeState)
		} else {
			s.backupService.RunBackup(ctx, &job, &source, tapeID, job.BackupType)
		}
	}()

	s.auditLog(r, "retry", "backup_job", id, "Retried backup job")

	msg := "Backup job retried"
	if resumeState != "" {
		msg = "Backup job resumed from checkpoint"
	}
	if tapeLabel != "" {
		msg += fmt.Sprintf(" using tape %s", tapeLabel)
	}

	s.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":        "started",
		"message":       msg,
		"tape_id":       tapeID,
		"tape_label":    tapeLabel,
		"resumed":       resumeState != "",
	})
}

// handleResumableJobs lists jobs that have paused or failed executions that can be resumed
func (s *Server) handleResumableJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT je.id, je.job_id, j.name as job_name, je.status, je.files_processed, je.bytes_processed,
		       je.error_message, je.can_resume, je.created_at, je.updated_at
		FROM job_executions je
		JOIN backup_jobs j ON je.job_id = j.id
		WHERE je.can_resume = 1 AND je.status IN ('paused', 'failed')
		ORDER BY je.updated_at DESC
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	executions := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, jobID, filesProcessed, bytesProcessed int64
		var jobName, status, errorMessage string
		var canResume bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &jobID, &jobName, &status, &filesProcessed, &bytesProcessed,
			&errorMessage, &canResume, &createdAt, &updatedAt); err != nil {
			continue
		}
		executions = append(executions, map[string]interface{}{
			"id":              id,
			"job_id":          jobID,
			"job_name":        jobName,
			"status":          status,
			"files_processed": filesProcessed,
			"bytes_processed": bytesProcessed,
			"error_message":   errorMessage,
			"can_resume":      canResume,
			"created_at":      createdAt,
			"updated_at":      updatedAt,
		})
	}

	s.respondJSON(w, http.StatusOK, executions)
}

// Backup set handlers

func (s *Server) handleListBackupSets(w http.ResponseWriter, r *http.Request) {
	jobIDStr := r.URL.Query().Get("job_id")
	limit := 50

	query := `
		SELECT bs.id, bs.job_id, j.name as job_name, bs.tape_id, t.label as tape_label,
		       bs.backup_type, bs.start_time, bs.end_time, bs.status, bs.file_count, bs.total_bytes,
		       COALESCE(bs.encrypted, 0) as encrypted, bs.encryption_key_id,
		       COALESCE(bs.compressed, 0) as compressed, COALESCE(bs.compression_type, 'none') as compression_type,
		       tp.name as pool_name
		FROM backup_sets bs
		LEFT JOIN backup_jobs j ON bs.job_id = j.id
		LEFT JOIN tapes t ON bs.tape_id = t.id
		LEFT JOIN tape_pools tp ON t.pool_id = tp.id
	`
	var args []interface{}

	if jobIDStr != "" {
		jobID, _ := strconv.ParseInt(jobIDStr, 10, 64)
		query += " WHERE bs.job_id = ?"
		args = append(args, jobID)
	}

	query += " ORDER BY bs.start_time DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	sets := make([]map[string]interface{}, 0)
	for rows.Next() {
		var bs models.BackupSet
		var jobName, tapeLabel *string
		var encrypted bool
		var encryptionKeyID *int64
		var compressed bool
		var compressionType string
		var poolName *string
		if err := rows.Scan(&bs.ID, &bs.JobID, &jobName, &bs.TapeID, &tapeLabel,
			&bs.BackupType, &bs.StartTime, &bs.EndTime, &bs.Status, &bs.FileCount, &bs.TotalBytes,
			&encrypted, &encryptionKeyID,
			&compressed, &compressionType, &poolName); err != nil {
			continue
		}
		set := map[string]interface{}{
			"id":                bs.ID,
			"job_id":            bs.JobID,
			"job_name":          jobName,
			"tape_id":           bs.TapeID,
			"tape_label":        tapeLabel,
			"backup_type":       bs.BackupType,
			"start_time":        bs.StartTime,
			"end_time":          bs.EndTime,
			"status":            bs.Status,
			"file_count":        bs.FileCount,
			"total_bytes":       bs.TotalBytes,
			"encrypted":         encrypted,
			"encryption_key_id": encryptionKeyID,
			"compressed":        compressed,
			"compression_type":  compressionType,
			"pool_name":         poolName,
		}
		sets = append(sets, set)
	}

	s.respondJSON(w, http.StatusOK, sets)
}

func (s *Server) handleGetBackupSet(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid backup set id")
		return
	}

	var bs models.BackupSet
	err = s.db.QueryRow(`
		SELECT id, job_id, tape_id, backup_type, start_time, end_time, status, 
		       file_count, total_bytes, start_block, end_block, checksum, created_at
		FROM backup_sets WHERE id = ?
	`, id).Scan(&bs.ID, &bs.JobID, &bs.TapeID, &bs.BackupType, &bs.StartTime, &bs.EndTime, &bs.Status,
		&bs.FileCount, &bs.TotalBytes, &bs.StartBlock, &bs.EndBlock, &bs.Checksum, &bs.CreatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "backup set not found")
		return
	}

	s.respondJSON(w, http.StatusOK, bs)
}

func (s *Server) handleListBackupFiles(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid backup set id")
		return
	}

	prefix := r.URL.Query().Get("prefix")
	limit := 100

	ctx := r.Context()
	entries, err := s.restoreService.BrowseCatalog(ctx, id, prefix, limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, entries)
}

func (s *Server) handleDeleteBackupSet(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid backup set id")
		return
	}

	// Check the backup set exists and get its status
	var status string
	err = s.db.QueryRow("SELECT status FROM backup_sets WHERE id = ?", id).Scan(&status)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "backup set not found")
		return
	}

	// Only allow deletion of failed or completed backup sets
	if status != "failed" && status != "completed" {
		s.respondError(w, http.StatusBadRequest, "can only delete failed or completed backup sets")
		return
	}

	// Delete catalog entries first (foreign key)
	if _, err := s.db.Exec("DELETE FROM catalog_entries WHERE backup_set_id = ?", id); err != nil {
		s.logger.Warn("failed to delete catalog entries for backup set", map[string]interface{}{"backup_set_id": id, "error": err.Error()})
	}

	// Delete the backup set
	_, err = s.db.Exec("DELETE FROM backup_sets WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to delete backup set")
		return
	}

	s.auditLog(r, "delete", "backup_set", id, fmt.Sprintf("Deleted backup set #%d (status: %s)", id, status))
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Catalog handlers

func (s *Server) handleSearchCatalog(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("q")
	if pattern == "" {
		s.respondError(w, http.StatusBadRequest, "search pattern required")
		return
	}

	ctx := r.Context()
	entries, err := s.backupService.SearchCatalog(ctx, pattern, 100)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, entries)
}

func (s *Server) handleBrowseCatalog(w http.ResponseWriter, r *http.Request) {
	backupSetIDStr := chi.URLParam(r, "backupSetId")
	backupSetID, err := strconv.ParseInt(backupSetIDStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid backup set id")
		return
	}

	prefix := r.URL.Query().Get("prefix")

	ctx := r.Context()
	entries, err := s.restoreService.BrowseCatalog(ctx, backupSetID, prefix, 100)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, entries)
}

// Restore handlers

func (s *Server) handleRestorePlan(w http.ResponseWriter, r *http.Request) {
	var req restore.RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	tapes, err := s.restoreService.GetRequiredTapes(ctx, &req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"required_tapes": tapes,
		"message":        "Insert the tapes in the order shown to begin restore",
	})
}

func (s *Server) handleRunRestore(w http.ResponseWriter, r *http.Request) {
	var req restore.RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	result, err := s.restoreService.Restore(ctx, &req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, result)
}

// Log handlers

func (s *Server) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		offset, _ = strconv.Atoi(o)
	}

	rows, err := s.db.Query(`
		SELECT al.id, al.user_id, u.username, al.action, al.resource_type, al.resource_id, 
		       al.details, al.ip_address, al.created_at
		FROM audit_logs al
		LEFT JOIN users u ON al.user_id = u.id
		ORDER BY al.created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	logs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var al models.AuditLog
		var username *string
		if err := rows.Scan(&al.ID, &al.UserID, &username, &al.Action, &al.ResourceType, &al.ResourceID,
			&al.Details, &al.IPAddress, &al.CreatedAt); err != nil {
			continue
		}
		log := map[string]interface{}{
			"id":            al.ID,
			"user_id":       al.UserID,
			"username":      username,
			"action":        al.Action,
			"resource_type": al.ResourceType,
			"resource_id":   al.ResourceID,
			"details":       al.Details,
			"ip_address":    al.IPAddress,
			"created_at":    al.CreatedAt,
		}
		logs = append(logs, log)
	}

	s.respondJSON(w, http.StatusOK, logs)
}

func (s *Server) handleExportLogs(w http.ResponseWriter, r *http.Request) {
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	query := `
		SELECT al.id, u.username, al.action, al.resource_type, al.resource_id, 
		       al.details, al.ip_address, al.created_at
		FROM audit_logs al
		LEFT JOIN users u ON al.user_id = u.id
		WHERE 1=1
	`
	args := []interface{}{}

	if startDate != "" {
		query += " AND al.created_at >= ?"
		args = append(args, startDate)
	}
	if endDate != "" {
		query += " AND al.created_at <= ?"
		args = append(args, endDate)
	}

	query += " ORDER BY al.created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	logs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var al models.AuditLog
		var username *string
		if err := rows.Scan(&al.ID, &username, &al.Action, &al.ResourceType, &al.ResourceID,
			&al.Details, &al.IPAddress, &al.CreatedAt); err != nil {
			continue
		}
		log := map[string]interface{}{
			"id":            al.ID,
			"username":      username,
			"action":        al.Action,
			"resource_type": al.ResourceType,
			"resource_id":   al.ResourceID,
			"details":       al.Details,
			"ip_address":    al.IPAddress,
			"created_at":    al.CreatedAt,
		}
		logs = append(logs, log)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit_logs.json")
	json.NewEncoder(w).Encode(logs)
}

// User handlers

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.authService.ListUsers()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, users)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.authService.CreateUser(req.Username, req.Password, models.UserRole(req.Role))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := s.authService.DeleteUser(id); err != nil {
		if errors.Is(err, auth.ErrCannotDeleteAdmin) {
			s.respondError(w, http.StatusForbidden, "cannot delete the default admin account")
			return
		}
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Documentation handlers

// handleListDocs returns a list of available documentation files
func (s *Server) handleListDocs(w http.ResponseWriter, r *http.Request) {
	docs := []map[string]string{
		{"id": "usage", "title": "Usage Guide", "description": "Complete guide to using TapeBackarr"},
		{"id": "api", "title": "API Reference", "description": "REST API documentation"},
		{"id": "operator", "title": "Operator Guide", "description": "Quick reference for operators"},
		{"id": "recovery", "title": "Manual Recovery", "description": "Recover data without TapeBackarr"},
		{"id": "architecture", "title": "Architecture", "description": "System design and data flows"},
		{"id": "database", "title": "Database Schema", "description": "Database table definitions"},
	}
	s.respondJSON(w, http.StatusOK, docs)
}

// handleGetDoc returns a specific documentation file content
func (s *Server) handleGetDoc(w http.ResponseWriter, r *http.Request) {
	docID := chi.URLParam(r, "id")

	docFiles := map[string]string{
		"usage":        "USAGE_GUIDE.md",
		"api":          "API_REFERENCE.md",
		"operator":     "OPERATOR_GUIDE.md",
		"recovery":     "MANUAL_RECOVERY.md",
		"architecture": "ARCHITECTURE.md",
		"database":     "DATABASE_SCHEMA.md",
	}

	filename, ok := docFiles[docID]
	if !ok {
		s.respondError(w, http.StatusNotFound, "documentation not found")
		return
	}

	// Read documentation from embedded files or docs directory
	content, err := s.readDocFile(filename)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "documentation file not found")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"id":      docID,
		"title":   filename,
		"content": content,
	})
}

// readDocFile reads documentation file content
func (s *Server) readDocFile(filename string) (string, error) {
	// Try to read from docs directory relative to working directory
	paths := []string{
		"docs/" + filename,
		"../docs/" + filename,
		"/opt/tapebackarr/docs/" + filename,
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err == nil {
			return string(content), nil
		}
	}

	// Fall back to embedded documentation
	content, err := embeddedDocs.Content.ReadFile(filepath.Base(filename))
	if err == nil {
		return string(content), nil
	}

	return "", os.ErrNotExist
}

// Database backup handlers

// handleBackupDatabase backs up the TapeBackarr database to tape
func (s *Server) handleBackupDatabase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TapeID int64 `json:"tape_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get tape device path
	var devicePath string
	err := s.db.QueryRow("SELECT device_path FROM tape_drives WHERE current_tape_id = ?", req.TapeID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "tape not loaded in any drive")
		return
	}

	// Create database backup record
	result, err := s.db.Exec(`
		INSERT INTO database_backups (tape_id, status, backup_time)
		VALUES (?, 'pending', CURRENT_TIMESTAMP)
	`, req.TapeID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	backupID, _ := result.LastInsertId()

	// Run backup in background
	go s.runDatabaseBackup(backupID, req.TapeID, devicePath)

	s.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"id":      backupID,
		"status":  "started",
		"message": "Database backup started",
	})
}

// runDatabaseBackup performs the actual database backup to tape
func (s *Server) runDatabaseBackup(backupID, tapeID int64, devicePath string) {
	ctx := context.Background()

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "system",
			Title:    "Database Backup Started",
			Message:  fmt.Sprintf("Starting database backup (id=%d) to tape (id=%d) on %s", backupID, tapeID, devicePath),
		})
	}

	// Get database path from config
	var dbPath string
	err := s.db.QueryRow("SELECT path FROM pragma_database_list WHERE name='main'").Scan(&dbPath)
	if err != nil {
		// Use default path
		dbPath = "/var/lib/tapebackarr/tapebackarr.db"
	}

	// Create a backup copy of the database
	tempDir := "/tmp/tapebackarr-backup"
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	backupPath := tempDir + "/tapebackarr.db"

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "system",
			Title:    "Database Copy",
			Message:  fmt.Sprintf("Creating database copy using VACUUM INTO from %s", dbPath),
		})
	}

	// Use SQLite backup command
	_, err = s.db.Exec("VACUUM INTO ?", backupPath)
	if err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "system",
				Title:    "Database Backup Failed",
				Message:  fmt.Sprintf("Failed to create database copy: %s", err.Error()),
			})
		}
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", err.Error(), backupID)
		return
	}

	// Get file info
	info, err := os.Stat(backupPath)
	if err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "system",
				Title:    "Database Backup Failed",
				Message:  fmt.Sprintf("Failed to stat backup file: %s", err.Error()),
			})
		}
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", err.Error(), backupID)
		return
	}

	// Calculate checksum
	checksum, _ := calculateFileChecksum(backupPath)

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "system",
			Title:    "Database Copy Complete",
			Message:  fmt.Sprintf("Database copy created: %d bytes, checksum: %s", info.Size(), checksum),
		})
	}

	// Position tape and write
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Rewinding Tape",
			Message:  "Rewinding tape for database backup write...",
		})
	}
	if err := s.tapeService.Rewind(ctx); err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Database Backup Failed",
				Message:  fmt.Sprintf("Failed to rewind tape: %s", err.Error()),
			})
		}
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", "failed to rewind: "+err.Error(), backupID)
		return
	}

	// Skip past tape label to first file position
	// Database backups are written after the label block (file 0)
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Seeking Tape",
			Message:  "Seeking to file position 1 (after label block)...",
		})
	}
	s.tapeService.SeekToFileNumber(ctx, 1)

	// Stream database backup to tape using tar
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Writing to Tape",
			Message:  fmt.Sprintf("Streaming database backup to %s using tar...", devicePath),
		})
	}
	tarArgs := []string{"-c", "-f", devicePath, "-C", tempDir, "tapebackarr.db"}
	cmd := exec.CommandContext(ctx, "tar", tarArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Database Backup Failed",
				Message:  fmt.Sprintf("tar command failed: %s", string(output)),
			})
		}
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", "tar failed: "+string(output), backupID)
		return
	}

	// Write file mark
	s.tapeService.WriteFileMark(ctx)

	// Update backup record
	s.db.Exec(`
		UPDATE database_backups 
		SET status = 'completed', file_size = ?, checksum = ?
		WHERE id = ?
	`, info.Size(), checksum, backupID)

	// Log audit entry
	s.db.Exec(`
		INSERT INTO audit_logs (action, resource_type, resource_id, details)
		VALUES (?, ?, ?, ?)
	`, "database_backup", "database_backup", backupID, "Database backed up to tape")

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "system",
			Title:    "Database Backup Complete",
			Message:  fmt.Sprintf("Database backup (id=%d) completed successfully: %d bytes written to tape", backupID, info.Size()),
			Details:  map[string]interface{}{"backup_id": backupID, "file_size": info.Size(), "checksum": checksum},
		})
	}
}

// handleListDatabaseBackups returns list of database backups
func (s *Server) handleListDatabaseBackups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT db.id, db.tape_id, t.label as tape_label, db.backup_time, db.file_size, 
		       db.checksum, db.status, db.error_message, db.created_at
		FROM database_backups db
		LEFT JOIN tapes t ON db.tape_id = t.id
		ORDER BY db.backup_time DESC
		LIMIT 50
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var backups []map[string]interface{}
	for rows.Next() {
		var id, tapeID, fileSize int64
		var tapeLabel, checksum, status, errorMsg *string
		var backupTime, createdAt time.Time

		if err := rows.Scan(&id, &tapeID, &tapeLabel, &backupTime, &fileSize, &checksum, &status, &errorMsg, &createdAt); err != nil {
			continue
		}

		backup := map[string]interface{}{
			"id":            id,
			"tape_id":       tapeID,
			"tape_label":    tapeLabel,
			"backup_time":   backupTime,
			"file_size":     fileSize,
			"checksum":      checksum,
			"status":        status,
			"error_message": errorMsg,
			"created_at":    createdAt,
		}
		backups = append(backups, backup)
	}

	s.respondJSON(w, http.StatusOK, backups)
}

// handleRestoreDatabaseBackup restores database from tape
func (s *Server) handleRestoreDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupID int64  `json:"backup_id"`
		DestPath string `json:"dest_path"` // Optional: path to restore to
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get backup info
	var tapeID, blockOffset int64
	err := s.db.QueryRow(`
		SELECT tape_id, COALESCE(block_offset, 0)
		FROM database_backups WHERE id = ?
	`, req.BackupID).Scan(&tapeID, &blockOffset)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "database backup not found")
		return
	}

	// Get tape device
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE current_tape_id = ?", tapeID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "required tape not loaded")
		return
	}

	destPath := req.DestPath
	if destPath == "" {
		destPath = "/tmp/tapebackarr-restore"
	}
	os.MkdirAll(destPath, 0755)

	ctx := r.Context()

	// Position tape
	if err := s.tapeService.Rewind(ctx); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to rewind tape")
		return
	}

	if blockOffset > 0 {
		s.tapeService.SeekToBlock(ctx, blockOffset)
	}

	// Extract database
	tarArgs := []string{"-x", "-f", devicePath, "-C", destPath, "tapebackarr.db"}
	cmd := exec.CommandContext(ctx, "tar", tarArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "restore failed: "+string(output))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":    "restored",
		"dest_path": destPath + "/tapebackarr.db",
	})
}

// Drive management handlers

// handleCreateDrive adds a new tape drive
func (s *Server) handleCreateDrive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DevicePath   string `json:"device_path"`
		DisplayName  string `json:"display_name"`
		Vendor       string `json:"vendor"`
		SerialNumber string `json:"serial_number"`
		Model        string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Probe the drive to get initial status and fill in missing info
	initialStatus := "offline"
	ctx := r.Context()
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	driveSvc := tape.NewServiceForDevice(req.DevicePath, s.tapeService.GetBlockSize())
	if hwStatus, err := driveSvc.GetStatus(probeCtx); err == nil && hwStatus.Error == "" && hwStatus.Online {
		initialStatus = "ready"
	}
	cancel()

	// Try to fill vendor/model/serial from hardware if not provided
	if req.Vendor == "" || req.Model == "" || req.SerialNumber == "" {
		infoCtx, infoCancel := context.WithTimeout(ctx, 3*time.Second)
		if info, err := driveSvc.GetDriveInfo(infoCtx); err == nil {
			if v, ok := info["Vendor identification"]; ok && req.Vendor == "" {
				req.Vendor = v
			}
			if v, ok := info["Product identification"]; ok && req.Model == "" {
				req.Model = v
			}
			if v, ok := info["Unit serial number"]; ok && req.SerialNumber == "" {
				req.SerialNumber = v
			}
		}
		infoCancel()
	}

	result, err := s.db.Exec(`
		INSERT INTO tape_drives (device_path, display_name, vendor, serial_number, model, status, enabled)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, req.DevicePath, req.DisplayName, req.Vendor, req.SerialNumber, req.Model, initialStatus)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "drive",
			Title:    "Drive Added",
			Message:  fmt.Sprintf("Tape drive '%s' at %s has been added", req.DisplayName, req.DevicePath),
		})
	}

	s.respondJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// handleUpdateDrive updates a tape drive configuration
func (s *Server) handleUpdateDrive(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	var req struct {
		DisplayName *string `json:"display_name"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.DisplayName != nil {
		updates = append(updates, "display_name = ?")
		args = append(args, *req.DisplayName)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	query := "UPDATE tape_drives SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteDrive removes a tape drive
func (s *Server) handleDeleteDrive(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	_, err = s.db.Exec("DELETE FROM tape_drives WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleSelectDrive selects which drive to use for operations
func (s *Server) handleSelectDrive(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	// Get drive device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ?", id).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found")
		return
	}

	// Update the tape service to use this drive
	// Note: In a full implementation, this would update the active drive configuration

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "selected",
		"drive_id":    id,
		"device_path": devicePath,
	})
}

// Helper function to calculate file checksum
func calculateFileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// generateUUID generates a random UUID v4
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(cryptoRand, b); err != nil {
		// Fallback: use timestamp-based pseudo-UUID if crypto/rand fails
		ts := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(ts >> (i * 4))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// handleFormatTape erases/formats a tape, removing all data including labels
func (s *Server) handleFormatTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var req struct {
		DriveID int64 `json:"drive_id"`
		Confirm bool  `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Require explicit confirmation for destructive action
	if !req.Confirm {
		s.respondError(w, http.StatusBadRequest, "destructive action requires confirm=true")
		return
	}

	// Check tape status - refuse to format exported tapes
	var status string
	err = s.db.QueryRow("SELECT status FROM tapes WHERE id = ?", id).Scan(&status)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}
	if status == "exported" {
		s.respondError(w, http.StatusConflict, "cannot format exported tape - import it first")
		return
	}

	// Get drive device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", req.DriveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "drive not found or not enabled")
		return
	}

	ctx := r.Context()
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Format Operation Started",
			Message:  fmt.Sprintf("Formatting tape (id=%d) on drive %s — all data will be erased", id, devicePath),
			Details:  map[string]interface{}{"tape_id": id, "device": devicePath},
		})
	}

	// Verify tape is loaded
	loaded, err := driveSvc.IsTapeLoaded(ctx)
	if err != nil || !loaded {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Format Failed",
				Message:  "No tape loaded in drive " + devicePath,
			})
		}
		s.respondError(w, http.StatusConflict, "no tape loaded in drive")
		return
	}

	// Check write protection
	driveStatus, err := driveSvc.GetStatus(ctx)
	if err == nil && driveStatus.WriteProtect {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Format Failed",
				Message:  "Tape is write-protected — cannot format",
			})
		}
		s.respondError(w, http.StatusConflict, "tape is write-protected")
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Erasing Tape",
			Message:  fmt.Sprintf("Running erase command on drive %s...", devicePath),
		})
	}

	// Erase the tape
	if err := driveSvc.EraseTape(ctx); err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Format Failed",
				Message:  fmt.Sprintf("Erase command failed: %s", err.Error()),
			})
		}
		s.respondError(w, http.StatusInternalServerError, "failed to format tape: "+err.Error())
		return
	}

	// Reset tape in database to blank state
	_, err = s.db.Exec(`
		UPDATE tapes SET status = 'blank', used_bytes = 0, write_count = 0,
		       last_written_at = NULL, labeled_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Formatted",
			Message:  fmt.Sprintf("Tape (id=%d) has been formatted and reset to blank state on drive %s", id, devicePath),
			Details:  map[string]interface{}{"tape_id": id, "device": devicePath},
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "formatted"})
}

// handleExportTape marks a tape as exported/offsite
func (s *Server) handleExportTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var req struct {
		OffsiteLocation string `json:"offsite_location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Verify tape exists and is not already exported
	var status string
	err = s.db.QueryRow("SELECT status FROM tapes WHERE id = ?", id).Scan(&status)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}
	if status == "exported" {
		s.respondError(w, http.StatusConflict, "tape is already exported")
		return
	}
	if status == "blank" {
		s.respondError(w, http.StatusConflict, "cannot export a blank tape")
		return
	}

	_, err = s.db.Exec(`
		UPDATE tapes SET status = 'exported', offsite_location = ?, export_time = CURRENT_TIMESTAMP,
		       updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, req.OffsiteLocation, id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "exported"})
}

// handleImportTape imports an exported tape back into the system
func (s *Server) handleImportTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var req struct {
		DriveID *int64 `json:"drive_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Verify tape is exported
	var status string
	var tapeUUID, tapeLabel string
	var poolID *int64
	err = s.db.QueryRow("SELECT status, uuid, label, pool_id FROM tapes WHERE id = ?", id).Scan(&status, &tapeUUID, &tapeLabel, &poolID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "tape not found")
		return
	}
	if status != "exported" {
		s.respondError(w, http.StatusConflict, "tape is not exported")
		return
	}

	// If drive is specified, verify label on physical tape matches
	if req.DriveID != nil {
		var devicePath string
		err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", *req.DriveID).Scan(&devicePath)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "drive not found or not enabled")
			return
		}

		ctx := r.Context()
		driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

		labelData, err := driveSvc.ReadTapeLabel(ctx)
		if err != nil {
			s.logger.Warn("Could not read tape label during import", map[string]interface{}{"error": err.Error()})
		} else if labelData != nil && labelData.UUID != "" && strings.ToLower(labelData.UUID) != strings.ToLower(tapeUUID) {
			s.respondError(w, http.StatusConflict, "tape label UUID mismatch - loaded tape does not match database record")
			return
		}
	}

	// Restore tape to previous usable state (full if it had data, active otherwise)
	newStatus := "full"

	_, err = s.db.Exec(`
		UPDATE tapes SET status = ?, import_time = CURRENT_TIMESTAMP,
		       updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, newStatus, id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "imported", "new_status": newStatus})
}

// handleReadTapeLabel reads the label from a physical tape in the drive
func (s *Server) handleReadTapeLabel(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	driveIDStr := r.URL.Query().Get("drive_id")
	if driveIDStr == "" {
		s.respondError(w, http.StatusBadRequest, "drive_id query parameter is required")
		return
	}
	driveID, err := strconv.ParseInt(driveIDStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive_id")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", driveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "drive not found or not enabled")
		return
	}

	ctx := r.Context()
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	labelData, err := driveSvc.ReadTapeLabel(ctx)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to read tape label: "+err.Error())
		return
	}

	if labelData == nil {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"tape_id": id,
			"labeled": false,
			"message": "no TapeBackarr label found on tape",
		})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"tape_id":   id,
		"labeled":   true,
		"label":     labelData.Label,
		"uuid":      labelData.UUID,
		"pool":      labelData.Pool,
		"timestamp": labelData.Timestamp,
	})
}

// handleScanDrives scans the system for available tape drives
func (s *Server) handleScanDrives(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	drives, err := tape.ScanDrives(ctx)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to scan drives: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, drives)
}

// handleFormatTapeInDrive formats whatever tape is loaded in a specific drive
// This works even if the tape is not in our database
func (s *Server) handleFormatTapeInDrive(w http.ResponseWriter, r *http.Request) {
	driveID, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !req.Confirm {
		s.respondError(w, http.StatusBadRequest, "format operation must be confirmed")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", driveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found or not enabled")
		return
	}

	ctx := r.Context()
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Format Started",
			Message:  fmt.Sprintf("Formatting tape in drive %s — checking tape status...", devicePath),
			Details:  map[string]interface{}{"drive_id": driveID, "device": devicePath},
		})
	}

	// Check if tape is loaded
	loaded, err := driveSvc.IsTapeLoaded(ctx)
	if err != nil || !loaded {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Format Failed",
				Message:  "No tape loaded in drive " + devicePath,
			})
		}
		s.respondError(w, http.StatusConflict, "no tape loaded in drive")
		return
	}

	// Read current label before formatting (for audit/notification)
	var oldLabel, oldUUID string
	if labelData, err := driveSvc.ReadTapeLabel(ctx); err == nil && labelData != nil {
		oldLabel = labelData.Label
		oldUUID = labelData.UUID
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "tape",
				Title:    "Tape Identified",
				Message:  fmt.Sprintf("Found tape '%s' (UUID: %s) — proceeding with erase", oldLabel, oldUUID),
			})
		}
	} else if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Tape Label Unreadable",
			Message:  "Could not read existing label — proceeding with erase",
		})
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Erasing Tape",
			Message:  fmt.Sprintf("Running erase command on drive %s...", devicePath),
		})
	}

	// Perform the format/erase
	if err := driveSvc.EraseTape(ctx); err != nil {
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Format Failed",
				Message:  fmt.Sprintf("Erase command failed on drive %s: %s", devicePath, err.Error()),
			})
		}
		s.respondError(w, http.StatusInternalServerError, "failed to format tape: "+err.Error())
		return
	}

	// If the tape was in our database, update its status
	if oldUUID != "" {
		if _, err := s.db.Exec("UPDATE tapes SET status = 'blank', used_bytes = 0, labeled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE uuid = ?", oldUUID); err != nil {
			s.logger.Warn("Failed to update tape status by UUID after format", map[string]interface{}{"error": err.Error(), "uuid": oldUUID})
		}
	}
	if oldLabel != "" {
		if _, err := s.db.Exec("UPDATE tapes SET status = 'blank', used_bytes = 0, labeled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE label = ?", oldLabel); err != nil {
			s.logger.Warn("Failed to update tape status by label after format", map[string]interface{}{"error": err.Error(), "label": oldLabel})
		}
	}

	// Publish event
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Formatted",
			Message:  fmt.Sprintf("Tape '%s' has been formatted/erased in drive %s", oldLabel, devicePath),
			Details: map[string]interface{}{
				"drive_id":  driveID,
				"device":    devicePath,
				"old_label": oldLabel,
				"old_uuid":  oldUUID,
			},
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "formatted",
		"old_label": oldLabel,
		"old_uuid":  oldUUID,
	})
}

// handleGetLTOTypes returns the LTO capacity mapping
func (s *Server) handleGetLTOTypes(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, models.LTOCapacities)
}

// handleChangePassword allows any authenticated user to change their own password
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value("claims").(*auth.Claims)

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		s.respondError(w, http.StatusBadRequest, "old_password and new_password are required")
		return
	}

	if len(req.NewPassword) < 6 {
		s.respondError(w, http.StatusBadRequest, "new password must be at least 6 characters")
		return
	}

	if err := s.authService.UpdatePassword(claims.UserID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.respondError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

// handleGetConfig returns the current application configuration (sensitive fields masked)
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		s.respondError(w, http.StatusInternalServerError, "configuration not available")
		return
	}

	// Return config with sensitive fields masked
	safeConfig := *s.config
	if safeConfig.Auth.JWTSecret != "" {
		safeConfig.Auth.JWTSecret = "********"
	}
	if safeConfig.Notifications.Telegram.BotToken != "" {
		safeConfig.Notifications.Telegram.BotToken = "********"
	}
	if safeConfig.Notifications.Email.Password != "" {
		safeConfig.Notifications.Email.Password = "********"
	}
	if safeConfig.Proxmox.Password != "" {
		safeConfig.Proxmox.Password = "********"
	}
	if safeConfig.Proxmox.TokenSecret != "" {
		safeConfig.Proxmox.TokenSecret = "********"
	}

	s.respondJSON(w, http.StatusOK, safeConfig)
}

// handleUpdateConfig updates the application configuration
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if s.config == nil || s.configPath == "" {
		s.respondError(w, http.StatusInternalServerError, "configuration not available")
		return
	}

	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Preserve sensitive fields if they were masked (not changed)
	if newCfg.Auth.JWTSecret == "********" {
		newCfg.Auth.JWTSecret = s.config.Auth.JWTSecret
	}
	if newCfg.Notifications.Telegram.BotToken == "********" {
		newCfg.Notifications.Telegram.BotToken = s.config.Notifications.Telegram.BotToken
	}
	if newCfg.Notifications.Email.Password == "********" {
		newCfg.Notifications.Email.Password = s.config.Notifications.Email.Password
	}
	if newCfg.Proxmox.Password == "********" {
		newCfg.Proxmox.Password = s.config.Proxmox.Password
	}
	if newCfg.Proxmox.TokenSecret == "********" {
		newCfg.Proxmox.TokenSecret = s.config.Proxmox.TokenSecret
	}

	// Save to disk
	if err := newCfg.Save(s.configPath); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save configuration: "+err.Error())
		return
	}

	// Update in-memory config
	*s.config = newCfg

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "configuration saved", "note": "some changes require a restart to take effect"})
}

// handleTestTelegram sends a test message via Telegram
func (s *Server) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		s.respondError(w, http.StatusInternalServerError, "configuration not available")
		return
	}

	tgConfig := s.config.Notifications.Telegram
	if !tgConfig.Enabled || tgConfig.BotToken == "" || tgConfig.ChatID == "" {
		s.respondError(w, http.StatusBadRequest, "Telegram notifications are not configured. Please enable and configure bot token and chat ID first.")
		return
	}

	svc := notifications.NewTelegramService(notifications.TelegramConfig{
		Enabled:  tgConfig.Enabled,
		BotToken: tgConfig.BotToken,
		ChatID:   tgConfig.ChatID,
	})

	if err := svc.SendTestMessage(r.Context()); err != nil {
		s.respondError(w, http.StatusInternalServerError, "Failed to send test message: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "Test message sent successfully"})
}

// ==================== Proxmox Handlers ====================

// handleProxmoxListNodes returns all Proxmox nodes
func (s *Server) handleProxmoxListNodes(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxClient == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	nodes, err := s.proxmoxClient.GetNodes(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, nodes)
}

// handleProxmoxListGuests returns all VMs and LXCs across all nodes
func (s *Server) handleProxmoxListGuests(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxClient == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	node := r.URL.Query().Get("node")
	guestType := r.URL.Query().Get("type")

	var vms []proxmox.VMInfo
	var lxcs []proxmox.LXCInfo
	var err error

	if node != "" {
		// Get guests from specific node
		if guestType != "lxc" {
			vms, err = s.proxmoxClient.GetNodeVMs(r.Context(), node)
			if err != nil {
				s.respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if guestType != "qemu" {
			lxcs, err = s.proxmoxClient.GetNodeLXCs(r.Context(), node)
			if err != nil {
				s.respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	} else {
		// Get guests from all nodes
		vms, lxcs, err = s.proxmoxClient.GetAllGuests(r.Context())
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Filter by type if requested
	if guestType == "qemu" {
		lxcs = nil
	} else if guestType == "lxc" {
		vms = nil
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"vms":  vms,
		"lxcs": lxcs,
	})
}

// handleProxmoxGetGuest returns details of a specific guest
func (s *Server) handleProxmoxGetGuest(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxClient == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	vmidStr := chi.URLParam(r, "vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	node := r.URL.Query().Get("node")
	if node == "" {
		// Try to find the node
		nodes, err := s.proxmoxClient.GetNodes(r.Context())
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		for _, n := range nodes {
			if n.Status != "online" {
				continue
			}
			// Check VMs
			vms, _ := s.proxmoxClient.GetNodeVMs(r.Context(), n.Node)
			for _, vm := range vms {
				if vm.VMID == vmid {
					s.respondJSON(w, http.StatusOK, vm)
					return
				}
			}
			// Check LXCs
			lxcs, _ := s.proxmoxClient.GetNodeLXCs(r.Context(), n.Node)
			for _, lxc := range lxcs {
				if lxc.VMID == vmid {
					s.respondJSON(w, http.StatusOK, lxc)
					return
				}
			}
		}
		s.respondError(w, http.StatusNotFound, "guest not found")
		return
	}

	// Check VMs first
	vms, _ := s.proxmoxClient.GetNodeVMs(r.Context(), node)
	for _, vm := range vms {
		if vm.VMID == vmid {
			s.respondJSON(w, http.StatusOK, vm)
			return
		}
	}

	// Check LXCs
	lxcs, _ := s.proxmoxClient.GetNodeLXCs(r.Context(), node)
	for _, lxc := range lxcs {
		if lxc.VMID == vmid {
			s.respondJSON(w, http.StatusOK, lxc)
			return
		}
	}

	s.respondError(w, http.StatusNotFound, "guest not found")
}

// handleProxmoxGetGuestConfig returns the configuration of a guest
func (s *Server) handleProxmoxGetGuestConfig(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxClient == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	vmidStr := chi.URLParam(r, "vmid")
	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid vmid")
		return
	}

	node := r.URL.Query().Get("node")
	guestType := r.URL.Query().Get("type")

	if node == "" {
		s.respondError(w, http.StatusBadRequest, "node parameter required")
		return
	}

	if guestType == "lxc" {
		config, err := s.proxmoxClient.GetLXCConfig(r.Context(), node, vmid)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, config)
	} else {
		config, err := s.proxmoxClient.GetVMConfig(r.Context(), node, vmid)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, config)
	}
}

// handleProxmoxClusterStatus returns cluster status information
func (s *Server) handleProxmoxClusterStatus(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxClient == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	isCluster, err := s.proxmoxClient.IsClusterMode(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	nodes, err := s.proxmoxClient.GetNodes(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"is_cluster": isCluster,
		"node_count": len(nodes),
		"nodes":      nodes,
	})
}

// handleProxmoxListBackups returns all Proxmox backups
func (s *Server) handleProxmoxListBackups(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxBackupService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	backups, err := s.proxmoxBackupService.ListBackups(r.Context(), limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, backups)
}

// handleProxmoxGetBackup returns details of a specific backup
func (s *Server) handleProxmoxGetBackup(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxBackupService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	backup, err := s.proxmoxBackupService.GetBackup(r.Context(), id)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "backup not found")
		return
	}

	s.respondJSON(w, http.StatusOK, backup)
}

// handleProxmoxCreateBackup creates a backup of a single guest
func (s *Server) handleProxmoxCreateBackup(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxBackupService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	var req proxmox.ProxmoxBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Node == "" || req.VMID == 0 || req.TapeID == 0 {
		s.respondError(w, http.StatusBadRequest, "node, vmid, and tape_id are required")
		return
	}

	// Set defaults
	if req.BackupMode == "" {
		req.BackupMode = proxmox.BackupModeSnapshot
	}
	if req.GuestType == "" {
		req.GuestType = proxmox.GuestTypeVM
	}

	result, err := s.proxmoxBackupService.BackupGuest(r.Context(), &req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, result)
}

// handleProxmoxBackupAll backs up all guests
func (s *Server) handleProxmoxBackupAll(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxBackupService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	var req struct {
		Node     string `json:"node,omitempty"` // Empty = all nodes
		TapeID   int64  `json:"tape_id"`
		Mode     string `json:"mode"`
		Compress string `json:"compress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TapeID == 0 {
		s.respondError(w, http.StatusBadRequest, "tape_id is required")
		return
	}

	mode := proxmox.BackupModeSnapshot
	if req.Mode != "" {
		mode = proxmox.BackupMode(req.Mode)
	}

	results, err := s.proxmoxBackupService.BackupAllGuests(r.Context(), req.Node, req.TapeID, mode, req.Compress)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, results)
}

// handleProxmoxListRestores returns all Proxmox restores
func (s *Server) handleProxmoxListRestores(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxRestoreService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	restores, err := s.proxmoxRestoreService.ListRestores(r.Context(), limit)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, restores)
}

// handleProxmoxCreateRestore restores a guest from a backup
func (s *Server) handleProxmoxCreateRestore(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxRestoreService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	var req proxmox.RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BackupID == 0 {
		s.respondError(w, http.StatusBadRequest, "backup_id is required")
		return
	}

	result, err := s.proxmoxRestoreService.RestoreGuest(r.Context(), &req)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, result)
}

// handleProxmoxRestorePlan returns the tapes needed for a restore
func (s *Server) handleProxmoxRestorePlan(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxRestoreService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	var req struct {
		BackupID int64 `json:"backup_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tapes, err := s.proxmoxRestoreService.GetRequiredTapes(r.Context(), req.BackupID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"required_tapes": tapes,
	})
}

// handleProxmoxListJobs returns all Proxmox backup jobs
func (s *Server) handleProxmoxListJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT j.id, j.name, j.description, j.node, j.vmid_filter, j.guest_type_filter, j.tag_filter,
		       j.pool_id, j.backup_mode, j.compress, j.schedule_cron, j.retention_days,
		       j.enabled, j.last_run_at, j.next_run_at, j.created_at,
		       COALESCE(j.notify_on_success, 0), COALESCE(j.notify_on_failure, 1), COALESCE(j.notes, ''),
		       tp.name as pool_name
		FROM proxmox_backup_jobs j
		LEFT JOIN tape_pools tp ON j.pool_id = tp.id
		ORDER BY j.created_at DESC
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var jobs []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, backupMode, compress, scheduleCron string
		var description, node, vmidFilter, guestTypeFilter, tagFilter *string
		var poolID *int64
		var retentionDays int
		var enabled, notifyOnSuccess, notifyOnFailure bool
		var notes string
		var lastRunAt, nextRunAt *time.Time
		var createdAt time.Time
		var poolName *string

		if err := rows.Scan(&id, &name, &description, &node, &vmidFilter, &guestTypeFilter, &tagFilter,
			&poolID, &backupMode, &compress, &scheduleCron, &retentionDays,
			&enabled, &lastRunAt, &nextRunAt, &createdAt,
			&notifyOnSuccess, &notifyOnFailure, &notes, &poolName); err != nil {
			continue
		}

		job := map[string]interface{}{
			"id":                id,
			"name":              name,
			"backup_mode":       backupMode,
			"compression":       compress,
			"schedule_cron":     scheduleCron,
			"retention_days":    retentionDays,
			"enabled":           enabled,
			"notify_on_success": notifyOnSuccess,
			"notify_on_failure": notifyOnFailure,
			"notes":             notes,
			"created_at":        createdAt,
		}
		if description != nil {
			job["description"] = *description
		}
		if node != nil {
			job["node"] = *node
		}
		if vmidFilter != nil {
			job["vmids"] = *vmidFilter
		}
		if guestTypeFilter != nil {
			job["guest_type_filter"] = *guestTypeFilter
		}
		if tagFilter != nil {
			job["tag_filter"] = *tagFilter
		}
		if poolID != nil {
			job["pool_id"] = *poolID
		}
		if poolName != nil {
			job["pool_name"] = *poolName
		}
		if lastRunAt != nil {
			job["last_run_at"] = *lastRunAt
		}
		if nextRunAt != nil {
			job["next_run_at"] = *nextRunAt
		}

		jobs = append(jobs, job)
	}

	s.respondJSON(w, http.StatusOK, jobs)
}

// handleProxmoxCreateJob creates a new Proxmox backup job
func (s *Server) handleProxmoxCreateJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string `json:"name"`
		Description     string `json:"description,omitempty"`
		Node            string `json:"node,omitempty"`
		VMIDs           string `json:"vmids,omitempty"`
		VMIDFilter      string `json:"vmid_filter,omitempty"`
		GuestTypeFilter string `json:"guest_type_filter,omitempty"`
		TagFilter       string `json:"tag_filter,omitempty"`
		PoolID          *int64 `json:"pool_id,omitempty"`
		BackupMode      string `json:"backup_mode"`
		Compress        string `json:"compress"`
		Compression     string `json:"compression"`
		ScheduleCron    string `json:"schedule_cron"`
		RetentionDays   int    `json:"retention_days"`
		Enabled         bool   `json:"enabled"`
		NotifyOnSuccess bool   `json:"notify_on_success"`
		NotifyOnFailure bool   `json:"notify_on_failure"`
		Notes           string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.BackupMode == "" {
		req.BackupMode = "snapshot"
	}
	// Accept either compress or compression field
	if req.Compress == "" && req.Compression != "" {
		req.Compress = req.Compression
	}
	if req.Compress == "" {
		req.Compress = "zstd"
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = 30
	}
	// Accept either vmids or vmid_filter field
	vmidFilter := req.VMIDFilter
	if vmidFilter == "" && req.VMIDs != "" {
		vmidFilter = req.VMIDs
	}

	result, err := s.db.Exec(`
		INSERT INTO proxmox_backup_jobs (
			name, description, node, vmid_filter, guest_type_filter, tag_filter,
			pool_id, backup_mode, compress, schedule_cron, retention_days, enabled,
			notify_on_success, notify_on_failure, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Description, req.Node, vmidFilter, req.GuestTypeFilter, req.TagFilter,
		req.PoolID, req.BackupMode, req.Compress, req.ScheduleCron, req.RetentionDays, req.Enabled,
		req.NotifyOnSuccess, req.NotifyOnFailure, req.Notes)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      id,
		"message": "Proxmox backup job created",
	})
}

// handleProxmoxGetJob returns a specific Proxmox backup job
func (s *Server) handleProxmoxGetJob(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var name, backupMode, compress, scheduleCron string
	var description, node, vmidFilter, guestTypeFilter, tagFilter *string
	var poolID *int64
	var retentionDays int
	var enabled bool
	var lastRunAt, nextRunAt *time.Time
	var createdAt time.Time

	err = s.db.QueryRow(`
		SELECT name, description, node, vmid_filter, guest_type_filter, tag_filter,
		       pool_id, backup_mode, compress, schedule_cron, retention_days,
		       enabled, last_run_at, next_run_at, created_at
		FROM proxmox_backup_jobs
		WHERE id = ?
	`, id).Scan(&name, &description, &node, &vmidFilter, &guestTypeFilter, &tagFilter,
		&poolID, &backupMode, &compress, &scheduleCron, &retentionDays,
		&enabled, &lastRunAt, &nextRunAt, &createdAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "job not found")
		return
	}

	job := map[string]interface{}{
		"id":             id,
		"name":           name,
		"backup_mode":    backupMode,
		"compress":       compress,
		"schedule_cron":  scheduleCron,
		"retention_days": retentionDays,
		"enabled":        enabled,
		"created_at":     createdAt,
	}
	if description != nil {
		job["description"] = *description
	}
	if node != nil {
		job["node"] = *node
	}
	if vmidFilter != nil {
		job["vmid_filter"] = *vmidFilter
	}
	if guestTypeFilter != nil {
		job["guest_type_filter"] = *guestTypeFilter
	}
	if tagFilter != nil {
		job["tag_filter"] = *tagFilter
	}
	if poolID != nil {
		job["pool_id"] = *poolID
	}
	if lastRunAt != nil {
		job["last_run_at"] = *lastRunAt
	}
	if nextRunAt != nil {
		job["next_run_at"] = *nextRunAt
	}

	s.respondJSON(w, http.StatusOK, job)
}

// handleProxmoxUpdateJob updates a Proxmox backup job
func (s *Server) handleProxmoxUpdateJob(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var req struct {
		Name            string `json:"name,omitempty"`
		Description     string `json:"description,omitempty"`
		Node            string `json:"node,omitempty"`
		VMIDs           string `json:"vmids,omitempty"`
		VMIDFilter      string `json:"vmid_filter,omitempty"`
		GuestTypeFilter string `json:"guest_type_filter,omitempty"`
		TagFilter       string `json:"tag_filter,omitempty"`
		PoolID          *int64 `json:"pool_id,omitempty"`
		BackupMode      string `json:"backup_mode,omitempty"`
		Compress        string `json:"compress,omitempty"`
		Compression     string `json:"compression,omitempty"`
		ScheduleCron    string `json:"schedule_cron,omitempty"`
		RetentionDays   *int   `json:"retention_days,omitempty"`
		Enabled         *bool  `json:"enabled,omitempty"`
		NotifyOnSuccess *bool  `json:"notify_on_success,omitempty"`
		NotifyOnFailure *bool  `json:"notify_on_failure,omitempty"`
		Notes           *string `json:"notes,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}

	if req.Name != "" {
		updates = append(updates, "name = ?")
		args = append(args, req.Name)
	}
	if req.Description != "" {
		updates = append(updates, "description = ?")
		args = append(args, req.Description)
	}
	if req.Node != "" {
		updates = append(updates, "node = ?")
		args = append(args, req.Node)
	}
	// Accept either vmids or vmid_filter
	vmidFilter := req.VMIDFilter
	if vmidFilter == "" && req.VMIDs != "" {
		vmidFilter = req.VMIDs
	}
	if vmidFilter != "" {
		updates = append(updates, "vmid_filter = ?")
		args = append(args, vmidFilter)
	}
	if req.GuestTypeFilter != "" {
		updates = append(updates, "guest_type_filter = ?")
		args = append(args, req.GuestTypeFilter)
	}
	if req.TagFilter != "" {
		updates = append(updates, "tag_filter = ?")
		args = append(args, req.TagFilter)
	}
	if req.PoolID != nil {
		updates = append(updates, "pool_id = ?")
		args = append(args, *req.PoolID)
	}
	if req.BackupMode != "" {
		updates = append(updates, "backup_mode = ?")
		args = append(args, req.BackupMode)
	}
	// Accept either compress or compression
	compress := req.Compress
	if compress == "" && req.Compression != "" {
		compress = req.Compression
	}
	if compress != "" {
		updates = append(updates, "compress = ?")
		args = append(args, compress)
	}
	if req.ScheduleCron != "" {
		updates = append(updates, "schedule_cron = ?")
		args = append(args, req.ScheduleCron)
	}
	if req.RetentionDays != nil {
		updates = append(updates, "retention_days = ?")
		args = append(args, *req.RetentionDays)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}
	if req.NotifyOnSuccess != nil {
		updates = append(updates, "notify_on_success = ?")
		args = append(args, *req.NotifyOnSuccess)
	}
	if req.NotifyOnFailure != nil {
		updates = append(updates, "notify_on_failure = ?")
		args = append(args, *req.NotifyOnFailure)
	}
	if req.Notes != nil {
		updates = append(updates, "notes = ?")
		args = append(args, *req.Notes)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	// Note: This query construction is safe because:
	// 1. Column names in 'updates' are hardcoded strings, not user input
	// 2. All values are properly parameterized with '?' placeholders
	args = append(args, id)
	query := "UPDATE proxmox_backup_jobs SET " + strings.Join(updates, ", ") + " WHERE id = ?"

	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": "Proxmox backup job updated",
	})
}

// handleProxmoxDeleteJob deletes a Proxmox backup job
func (s *Server) handleProxmoxDeleteJob(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	_, err = s.db.Exec("DELETE FROM proxmox_backup_jobs WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": "Proxmox backup job deleted",
	})
}

// handleProxmoxRunJob manually runs a Proxmox backup job
func (s *Server) handleProxmoxRunJob(w http.ResponseWriter, r *http.Request) {
	if s.proxmoxBackupService == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Proxmox integration not configured")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var req struct {
		TapeID int64 `json:"tape_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TapeID == 0 {
		s.respondError(w, http.StatusBadRequest, "tape_id is required")
		return
	}

	// Get job details
	var node *string
	var backupMode, compress string
	err = s.db.QueryRow(`
		SELECT node, backup_mode, compress 
		FROM proxmox_backup_jobs 
		WHERE id = ?
	`, id).Scan(&node, &backupMode, &compress)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "job not found")
		return
	}

	nodeStr := ""
	if node != nil {
		nodeStr = *node
	}

	// Run backup for all guests matching the job criteria
	results, err := s.proxmoxBackupService.BackupAllGuests(
		r.Context(),
		nodeStr,
		req.TapeID,
		proxmox.BackupMode(backupMode),
		compress,
	)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update job last run time
	s.db.Exec("UPDATE proxmox_backup_jobs SET last_run_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Proxmox backup job executed",
		"job_id":  id,
		"results": results,
	})
}

// Encryption Key Handlers

func (s *Server) handleListEncryptionKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.encryptionService.ListKeys(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"keys": keys,
	})
}

func (s *Server) handleCreateEncryptionKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	key, keyBase64, err := s.encryptionService.GenerateKey(r.Context(), req.Name, req.Description)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Log the audit
	claims := r.Context().Value("claims").(*auth.Claims)
	s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details)
		VALUES (?, ?, ?, ?, ?)
	`, claims.UserID, "create", "encryption_key", key.ID, "Created encryption key: "+req.Name)

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"key":        key,
		"key_base64": keyBase64,
		"message":    "IMPORTANT: Save this key securely. It will not be shown again.",
	})
}

func (s *Server) handleImportEncryptionKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		KeyBase64   string `json:"key_base64"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.KeyBase64 == "" {
		s.respondError(w, http.StatusBadRequest, "name and key_base64 are required")
		return
	}

	key, err := s.encryptionService.ImportKey(r.Context(), req.Name, req.KeyBase64, req.Description)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log the audit
	claims := r.Context().Value("claims").(*auth.Claims)
	s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details)
		VALUES (?, ?, ?, ?, ?)
	`, claims.UserID, "import", "encryption_key", key.ID, "Imported encryption key: "+req.Name)

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"key":     key,
		"message": "Encryption key imported successfully",
	})
}

func (s *Server) handleDeleteEncryptionKey(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid key ID")
		return
	}

	if err := s.encryptionService.DeleteKey(r.Context(), id); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log the audit
	claims := r.Context().Value("claims").(*auth.Claims)
	s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details)
		VALUES (?, ?, ?, ?, ?)
	`, claims.UserID, "delete", "encryption_key", id, "Deleted encryption key")

	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": "Encryption key deleted successfully",
	})
}

func (s *Server) handleGetKeySheet(w http.ResponseWriter, r *http.Request) {
	sheet, err := s.encryptionService.GenerateKeySheet(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Log the audit
	claims := r.Context().Value("claims").(*auth.Claims)
	s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, details)
		VALUES (?, ?, ?, ?)
	`, claims.UserID, "export", "encryption_keys", "Generated key sheet for paper backup")

	s.respondJSON(w, http.StatusOK, sheet)
}

func (s *Server) handleGetKeySheetText(w http.ResponseWriter, r *http.Request) {
	text, err := s.encryptionService.GenerateKeySheetText(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Log the audit
	claims := r.Context().Value("claims").(*auth.Claims)
	s.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, details)
		VALUES (?, ?, ?, ?)
	`, claims.UserID, "export", "encryption_keys", "Generated key sheet text for printing")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=tapebackarr-keysheet.txt")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(text))
}

// API Key handlers

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.authService.ListAPIKeys()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []models.APIKey{}
	}
	s.respondJSON(w, http.StatusOK, keys)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		Role      string `json:"role"`
		ExpiresIn *int   `json:"expires_in_days"` // Optional: days until expiry
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	role := models.UserRole(req.Role)
	if role != models.RoleAdmin && role != models.RoleOperator && role != models.RoleReadOnly {
		s.respondError(w, http.StatusBadRequest, "invalid role: must be admin, operator, or readonly")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().AddDate(0, 0, *req.ExpiresIn)
		expiresAt = &t
	}

	rawKey, apiKey, err := s.authService.GenerateAPIKey(req.Name, role, expiresAt)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLog(r, "create", "api_key", apiKey.ID, fmt.Sprintf("Created API key '%s' with role '%s'", req.Name, req.Role))

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"key":     rawKey,
		"api_key": apiKey,
		"message": "Store this key securely - it will not be shown again",
	})
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid API key id")
		return
	}

	if err := s.authService.DeleteAPIKey(id); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLog(r, "delete", "api_key", id, "Deleted API key")

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleBatchLabel starts a batch tape labelling operation (legacy endpoint under /drives)
func (s *Server) handleBatchLabel(w http.ResponseWriter, r *http.Request) {
	driveID, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	var req struct {
		Prefix   string `json:"prefix"`       // e.g., "NAS-OFF-"
		StartNum int    `json:"start_number"` // e.g., 1
		Count    int    `json:"count"`        // How many tapes to label
		Digits   int    `json:"digits"`       // e.g., 3 for 001, 002
		PoolID   *int64 `json:"pool_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Prefix == "" {
		s.respondError(w, http.StatusBadRequest, "prefix is required")
		return
	}
	if req.Count <= 0 || req.Count > 1000 {
		s.respondError(w, http.StatusBadRequest, "count must be between 1 and 1000")
		return
	}
	if req.Digits < 1 || req.Digits > 6 {
		req.Digits = 3
	}
	if req.StartNum < 0 {
		req.StartNum = 1
	}

	s.batchLabel.mu.Lock()
	if s.batchLabel.running {
		s.batchLabel.mu.Unlock()
		s.respondError(w, http.StatusConflict, "batch labelling is already running")
		return
	}
	s.batchLabel.mu.Unlock()

	// Get drive device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", driveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found or not enabled")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	s.batchLabel.mu.Lock()
	s.batchLabel.running = true
	s.batchLabel.cancel = cancel
	s.batchLabel.progress = 0
	s.batchLabel.total = req.Count
	s.batchLabel.current = ""
	s.batchLabel.message = "Starting batch labelling..."
	s.batchLabel.completed = 0
	s.batchLabel.failed = 0
	s.batchLabel.mu.Unlock()

	// Start batch labelling in background
	go s.runBatchLabel(ctx, devicePath, driveID, req.Prefix, req.StartNum, req.Count, req.Digits, req.PoolID)

	s.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":  "started",
		"message": fmt.Sprintf("Batch labelling started: %s%0*d through %s%0*d", req.Prefix, req.Digits, req.StartNum, req.Prefix, req.Digits, req.StartNum+req.Count-1),
	})
}

func (s *Server) runBatchLabel(ctx context.Context, devicePath string, driveID int64, prefix string, startNum, count, digits int, poolID *int64) {
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	defer func() {
		s.batchLabel.mu.Lock()
		s.batchLabel.running = false
		s.batchLabel.cancel = nil
		s.batchLabel.mu.Unlock()
	}()

	poolName := ""
	if poolID != nil {
		_ = s.db.QueryRow("SELECT name FROM tape_pools WHERE id = ?", *poolID).Scan(&poolName)
	}

	for i := 0; i < count; i++ {
		// Check for cancellation
		select {
		case <-ctx.Done():
			s.batchLabel.mu.Lock()
			s.batchLabel.message = "Batch labelling cancelled by user"
			s.batchLabel.mu.Unlock()
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "warning",
					Category: "tape",
					Title:    "Batch Label",
					Message:  fmt.Sprintf("Batch labelling cancelled after %d/%d tapes", i, count),
				})
			}
			return
		default:
		}

		num := startNum + i
		label := fmt.Sprintf("%s%0*d", prefix, digits, num)

		s.batchLabel.mu.Lock()
		s.batchLabel.progress = i
		s.batchLabel.current = label
		s.batchLabel.message = fmt.Sprintf("Waiting for tape to label as '%s'... Insert tape and close drive.", label)
		s.batchLabel.mu.Unlock()

		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "info",
				Category: "tape",
				Title:    "Batch Label",
				Message:  fmt.Sprintf("[%d/%d] Waiting for tape to label as '%s'... Insert tape and close drive.", i+1, count, label),
				Details:  map[string]interface{}{"label": label, "progress": i, "total": count},
			})
		}

		// Wait for tape to be inserted (up to 5 minutes)
		if err := driveSvc.WaitForTape(ctx, 5*time.Minute); err != nil {
			if ctx.Err() != nil {
				s.batchLabel.mu.Lock()
				s.batchLabel.message = "Batch labelling cancelled by user"
				s.batchLabel.mu.Unlock()
				if s.eventBus != nil {
					s.eventBus.Publish(SystemEvent{
						Type:     "warning",
						Category: "tape",
						Title:    "Batch Label",
						Message:  fmt.Sprintf("Batch labelling cancelled after %d/%d tapes", i, count),
					})
				}
				return
			}
			s.batchLabel.mu.Lock()
			s.batchLabel.message = fmt.Sprintf("Timeout waiting for tape %d/%d", i+1, count)
			s.batchLabel.failed++
			s.batchLabel.mu.Unlock()
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "error",
					Category: "tape",
					Title:    "Batch Label",
					Message:  fmt.Sprintf("[%d/%d] Timeout waiting for tape. Batch labelling stopped.", i+1, count),
				})
			}
			return
		}

		// Check if tape already has a label
		existingLabel, err := driveSvc.ReadTapeLabel(ctx)
		if err == nil && existingLabel != nil && existingLabel.Label != "" {
			s.batchLabel.mu.Lock()
			s.batchLabel.message = fmt.Sprintf("Tape already labelled as '%s', skipping. Eject and insert a blank tape.", existingLabel.Label)
			s.batchLabel.mu.Unlock()
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "warning",
					Category: "tape",
					Title:    "Batch Label",
					Message:  fmt.Sprintf("[%d/%d] Tape already labelled as '%s', skipping. Eject and insert a blank tape.", i+1, count, existingLabel.Label),
				})
			}
			// Eject the already-labelled tape
			driveSvc.Eject(ctx)
			i-- // Retry this number
			continue
		}

		// Generate UUID for the tape
		uuidBytes := make([]byte, 16)
		cryptoRand.Read(uuidBytes)
		tapeUUID := hex.EncodeToString(uuidBytes)

		s.batchLabel.mu.Lock()
		s.batchLabel.message = fmt.Sprintf("Writing label '%s' to tape...", label)
		s.batchLabel.mu.Unlock()

		// Write label
		if err := driveSvc.WriteTapeLabel(ctx, label, tapeUUID, poolName); err != nil {
			s.batchLabel.mu.Lock()
			s.batchLabel.message = fmt.Sprintf("Failed to write label '%s': %s", label, err.Error())
			s.batchLabel.failed++
			s.batchLabel.mu.Unlock()
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "error",
					Category: "tape",
					Title:    "Batch Label",
					Message:  fmt.Sprintf("[%d/%d] Failed to write label '%s': %s", i+1, count, label, err.Error()),
				})
			}
			return
		}

		// Detect tape type
		ltoType := ""
		if detected, err := driveSvc.DetectTapeType(ctx); err == nil && detected != "" {
			ltoType = detected
		}
		capacityBytes := int64(0)
		if cap, ok := models.LTOCapacities[ltoType]; ok {
			capacityBytes = cap
		}

		// Create tape record in database
		now := time.Now()
		s.db.Exec(`
			INSERT INTO tapes (uuid, barcode, label, lto_type, pool_id, status, capacity_bytes, used_bytes, write_count, labeled_at)
			VALUES (?, ?, ?, ?, ?, 'blank', ?, 0, 0, ?)
		`, tapeUUID, label, label, ltoType, poolID, capacityBytes, now)

		s.batchLabel.mu.Lock()
		s.batchLabel.completed++
		s.batchLabel.message = fmt.Sprintf("Successfully labelled tape as '%s'. Ejecting...", label)
		s.batchLabel.mu.Unlock()

		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "success",
				Category: "tape",
				Title:    "Batch Label",
				Message:  fmt.Sprintf("[%d/%d] Successfully labelled tape as '%s'. Ejecting...", i+1, count, label),
				Details:  map[string]interface{}{"label": label, "uuid": tapeUUID, "lto_type": ltoType},
			})
		}

		// Eject tape
		if err := driveSvc.Eject(ctx); err != nil {
			if s.eventBus != nil {
				s.eventBus.Publish(SystemEvent{
					Type:     "warning",
					Category: "tape",
					Title:    "Batch Label",
					Message:  fmt.Sprintf("[%d/%d] Label written but eject failed: %s. Please eject manually.", i+1, count, err.Error()),
				})
			}
		}

		// Invalidate label cache
		if cache := s.tapeService.GetLabelCache(); cache != nil {
			cache.Invalidate(devicePath)
		}
	}

	s.batchLabel.mu.Lock()
	s.batchLabel.message = fmt.Sprintf("Batch labelling complete: %d tapes labelled", s.batchLabel.completed)
	s.batchLabel.mu.Unlock()

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Batch Label Complete",
			Message:  fmt.Sprintf("Batch labelling complete: %d tapes labelled (%s%0*d through %s%0*d)", count, prefix, digits, startNum, prefix, digits, startNum+count-1),
		})
	}
}

// handleTapesBatchLabel starts a batch tape labelling operation (under /tapes route)
func (s *Server) handleTapesBatchLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DriveID  int64  `json:"drive_id"`
		Prefix   string `json:"prefix"`
		StartNum int    `json:"start_number"`
		Count    int    `json:"count"`
		Digits   int    `json:"digits"`
		PoolID   *int64 `json:"pool_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DriveID <= 0 {
		s.respondError(w, http.StatusBadRequest, "drive_id is required")
		return
	}
	if req.Prefix == "" {
		s.respondError(w, http.StatusBadRequest, "prefix is required")
		return
	}
	if req.Count <= 0 || req.Count > 1000 {
		s.respondError(w, http.StatusBadRequest, "count must be between 1 and 1000")
		return
	}
	if req.Digits < 1 || req.Digits > 6 {
		req.Digits = 3
	}
	if req.StartNum < 0 {
		req.StartNum = 1
	}

	s.batchLabel.mu.Lock()
	if s.batchLabel.running {
		s.batchLabel.mu.Unlock()
		s.respondError(w, http.StatusConflict, "batch labelling is already running")
		return
	}
	s.batchLabel.mu.Unlock()

	// Get drive device path
	var devicePath string
	err := s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", req.DriveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found or not enabled")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	s.batchLabel.mu.Lock()
	s.batchLabel.running = true
	s.batchLabel.cancel = cancel
	s.batchLabel.progress = 0
	s.batchLabel.total = req.Count
	s.batchLabel.current = ""
	s.batchLabel.message = "Starting batch labelling..."
	s.batchLabel.completed = 0
	s.batchLabel.failed = 0
	s.batchLabel.mu.Unlock()

	go s.runBatchLabel(ctx, devicePath, req.DriveID, req.Prefix, req.StartNum, req.Count, req.Digits, req.PoolID)

	s.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":  "started",
		"message": fmt.Sprintf("Batch labelling started: %s%0*d through %s%0*d", req.Prefix, req.Digits, req.StartNum, req.Prefix, req.Digits, req.StartNum+req.Count-1),
	})
}

// handleBatchLabelStatus returns the current batch label operation status
func (s *Server) handleBatchLabelStatus(w http.ResponseWriter, r *http.Request) {
	s.batchLabel.mu.Lock()
	status := map[string]interface{}{
		"running":   s.batchLabel.running,
		"progress":  s.batchLabel.progress,
		"total":     s.batchLabel.total,
		"current":   s.batchLabel.current,
		"message":   s.batchLabel.message,
		"completed": s.batchLabel.completed,
		"failed":    s.batchLabel.failed,
	}
	s.batchLabel.mu.Unlock()
	s.respondJSON(w, http.StatusOK, status)
}

// handleBatchLabelCancel cancels the current batch label operation
func (s *Server) handleBatchLabelCancel(w http.ResponseWriter, r *http.Request) {
	s.batchLabel.mu.Lock()
	if !s.batchLabel.running || s.batchLabel.cancel == nil {
		s.batchLabel.mu.Unlock()
		s.respondError(w, http.StatusBadRequest, "no batch labelling operation is running")
		return
	}
	s.batchLabel.cancel()
	s.batchLabel.mu.Unlock()
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "cancelling"})
}

// handleHealthCheck returns detailed health status
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"components": map[string]interface{}{
			"database": s.checkDatabaseHealth(),
			"tape":     s.checkTapeHealth(),
		},
	}

	// Check if any component is unhealthy
	components := health["components"].(map[string]interface{})
	for _, v := range components {
		if comp, ok := v.(map[string]interface{}); ok {
			if status, ok := comp["status"].(string); ok && status != "ok" {
				health["status"] = "degraded"
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if health["status"] == "ok" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(health)
}

// checkDatabaseHealth verifies database connectivity
func (s *Server) checkDatabaseHealth() map[string]interface{} {
	result := map[string]interface{}{
		"status": "ok",
	}

	// Try a simple query
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		result["status"] = "error"
		result["error"] = "database query failed"
		return result
	}

	result["users"] = count
	return result
}

// checkTapeHealth returns tape drive status
func (s *Server) checkTapeHealth() map[string]interface{} {
	result := map[string]interface{}{
		"status": "ok",
	}

	// Get configured drives count
	var driveCount int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tape_drives").Scan(&driveCount)
	if err != nil {
		result["status"] = "unknown"
		result["drives"] = 0
		return result
	}

	result["drives"] = driveCount
	return result
}

func (s *Server) handleInspectTape(w http.ResponseWriter, r *http.Request) {
	driveID, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", driveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found or not enabled")
		return
	}

	ctx := r.Context()
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Tape Inspection Started",
			Message:  fmt.Sprintf("Inspecting tape in drive %s...", devicePath),
		})
	}

	result := map[string]interface{}{
		"drive_id":    driveID,
		"device_path": devicePath,
		"status":      "inspecting",
	}

	// Check drive status first
	hwStatus, statusErr := driveSvc.GetStatus(ctx)
	if statusErr != nil {
		result["status"] = "error"
		result["error"] = "Failed to query drive status: " + statusErr.Error()
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "error",
				Category: "tape",
				Title:    "Tape Inspection Failed",
				Message:  "Could not query drive status: " + statusErr.Error(),
			})
		}
		s.respondJSON(w, http.StatusOK, result)
		return
	}
	if !hwStatus.Online {
		result["status"] = "no_tape"
		result["message"] = "No tape loaded in drive"
		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "warning",
				Category: "tape",
				Title:    "Tape Inspection",
				Message:  "No tape loaded in drive " + devicePath,
			})
		}
		s.respondJSON(w, http.StatusOK, result)
		return
	}

	result["drive_online"] = true

	// Try to detect tape type
	if ltoType, err := driveSvc.DetectTapeType(ctx); err == nil && ltoType != "" {
		result["lto_type"] = ltoType
		if cap, ok := models.LTOCapacities[ltoType]; ok {
			result["capacity_bytes"] = cap
		}
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Tape Inspection",
			Message:  "Reading tape label...",
		})
	}

	// Read label
	labelData, labelErr := driveSvc.ReadTapeLabel(ctx)

	if labelData != nil && labelData.Label != "" {
		result["label"] = labelData.Label
		result["uuid"] = labelData.UUID
		result["pool"] = labelData.Pool
		result["timestamp"] = labelData.Timestamp
		result["has_tapebackarr_label"] = true
		if labelData.EncryptionKeyFingerprint != "" {
			result["encryption_key_fingerprint"] = labelData.EncryptionKeyFingerprint
			result["encrypted"] = true
		}
		if labelData.CompressionType != "" {
			result["compression_type"] = labelData.CompressionType
			result["compressed"] = true
		}

		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "success",
				Category: "tape",
				Title:    "Tape Inspection",
				Message:  fmt.Sprintf("Found TapeBackarr label: '%s' (UUID: %s)", labelData.Label, labelData.UUID),
			})
		}
	} else {
		result["has_tapebackarr_label"] = false
		labelMsg := "Tape does not have a TapeBackarr label (foreign or blank tape)"
		if labelErr != nil {
			labelMsg += ": " + labelErr.Error()
		}
		result["label_message"] = labelMsg

		if s.eventBus != nil {
			s.eventBus.Publish(SystemEvent{
				Type:     "warning",
				Category: "tape",
				Title:    "Tape Inspection",
				Message:  labelMsg,
			})
		}
	}

	// Try to list contents regardless of label status
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Tape Inspection",
			Message:  "Scanning tape contents...",
		})
	}

	entries, listErr := driveSvc.ListTapeContents(ctx, 1000)

	if listErr != nil {
		contentsErrMsg := listErr.Error()
		if labelData != nil && labelData.EncryptionKeyFingerprint != "" {
			contentsErrMsg = fmt.Sprintf("Cannot read tape contents — data appears to be encrypted (key fingerprint: %s). Use the matching encryption key to decrypt.", labelData.EncryptionKeyFingerprint)
		}
		result["contents_error"] = contentsErrMsg
	}

	if entries != nil {
		result["contents"] = entries
	} else {
		result["contents"] = []interface{}{}
	}

	result["status"] = "complete"

	if s.eventBus != nil {
		entryCount := 0
		if entries != nil {
			entryCount = len(entries)
		}
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "tape",
			Title:    "Tape Inspection Complete",
			Message:  fmt.Sprintf("Inspection complete: %d file entries found", entryCount),
		})
	}

	s.auditLog(r, "inspect", "tape_drive", driveID, fmt.Sprintf("Inspected tape in drive %s", devicePath))

	s.respondJSON(w, http.StatusOK, result)
}

// handleScanForDBBackup scans a tape for TapeBackarr database backup files
func (s *Server) handleScanForDBBackup(w http.ResponseWriter, r *http.Request) {
	driveID, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid drive id")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1", driveID).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "drive not found or not enabled")
		return
	}

	ctx := r.Context()
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "system",
			Title:    "DB Recovery Scan",
			Message:  "Scanning tape for database backup files...",
		})
	}

	// Rewind
	if err := driveSvc.Rewind(ctx); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to rewind: "+err.Error())
		return
	}

	// Read label if present
	labelData, _ := driveSvc.ReadTapeLabel(ctx)

	result := map[string]interface{}{
		"drive_id":    driveID,
		"device_path": devicePath,
	}

	if labelData != nil {
		result["tape_label"] = labelData.Label
		result["tape_uuid"] = labelData.UUID
	}

	// Try to list contents and find database backup files
	driveSvc.Rewind(ctx)
	if labelData != nil && labelData.Label != "" {
		driveSvc.SeekToFileNumber(ctx, 1)
	}

	entries, err := driveSvc.ListTapeContents(ctx, 5000)

	dbBackups := []map[string]interface{}{}
	if entries != nil {
		for _, entry := range entries {
			name := entry.Path
			if strings.Contains(strings.ToLower(name), "tapebackarr-db") ||
				strings.Contains(strings.ToLower(name), "tapebackarr_db") ||
				strings.HasSuffix(strings.ToLower(name), ".sql") ||
				strings.HasSuffix(strings.ToLower(name), ".db") {
				dbBackups = append(dbBackups, map[string]interface{}{
					"name":  name,
					"entry": entry,
				})
			}
		}
	}

	result["db_backups_found"] = len(dbBackups)
	result["db_backups"] = dbBackups
	result["total_entries"] = 0
	if entries != nil {
		result["total_entries"] = len(entries)
	}

	if err != nil {
		result["scan_error"] = err.Error()
	}

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "success",
			Category: "system",
			Title:    "DB Recovery Scan Complete",
			Message:  fmt.Sprintf("Found %d database backup file(s) on tape", len(dbBackups)),
		})
	}

	s.auditLog(r, "scan_db_backup", "tape_drive", driveID, fmt.Sprintf("Scanned for DB backups, found %d", len(dbBackups)))

	s.respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("Restart requested via API", nil)

	s.respondJSON(w, http.StatusOK, map[string]string{
		"status":  "restarting",
		"message": "TapeBackarr is restarting. The page will reload automatically.",
	})

	// Perform restart asynchronously after response is sent
	go func() {
		time.Sleep(500 * time.Millisecond)
		// Try systemctl first (standard deployment)
		cmd := exec.Command("systemctl", "restart", "tapebackarr")
		if err := cmd.Run(); err != nil {
			// Fallback: send interrupt to self to trigger graceful shutdown
			s.logger.Warn("systemctl restart failed, sending interrupt", map[string]interface{}{"error": err.Error()})
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				p.Signal(os.Interrupt)
			}
		}
	}()
}

// Tape Library (Autochanger) handlers

func (s *Server) handleListLibraries(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT id, name, device_path, vendor, model, serial_number,
		       num_slots, num_drives, num_import_export, barcode_reader,
		       enabled, last_inventory_at, created_at
		FROM tape_libraries
		ORDER BY created_at DESC
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	libraries := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, numSlots, numDrives, numIE int64
		var name, devicePath, vendor, model, serial string
		var barcodeReader, enabled bool
		var lastInventoryAt *time.Time
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &devicePath, &vendor, &model, &serial,
			&numSlots, &numDrives, &numIE, &barcodeReader,
			&enabled, &lastInventoryAt, &createdAt); err != nil {
			continue
		}

		lib := map[string]interface{}{
			"id":                id,
			"name":              name,
			"device_path":       devicePath,
			"vendor":            vendor,
			"model":             model,
			"serial_number":     serial,
			"num_slots":         numSlots,
			"num_drives":        numDrives,
			"num_import_export": numIE,
			"barcode_reader":    barcodeReader,
			"enabled":           enabled,
			"created_at":        createdAt,
		}
		if lastInventoryAt != nil {
			lib["last_inventory_at"] = *lastInventoryAt
		}
		libraries = append(libraries, lib)
	}

	s.respondJSON(w, http.StatusOK, libraries)
}

func (s *Server) handleCreateLibrary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		DevicePath   string `json:"device_path"`
		Vendor       string `json:"vendor"`
		Model        string `json:"model"`
		SerialNumber string `json:"serial_number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.DevicePath == "" {
		s.respondError(w, http.StatusBadRequest, "name and device_path are required")
		return
	}

	result, err := s.db.Exec(`
		INSERT INTO tape_libraries (name, device_path, vendor, model, serial_number)
		VALUES (?, ?, ?, ?, ?)
	`, req.Name, req.DevicePath, req.Vendor, req.Model, req.SerialNumber)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	s.auditLog(r, "create", "tape_library", id, fmt.Sprintf("Created tape library: %s (%s)", req.Name, req.DevicePath))
	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":      id,
		"message": "Tape library created",
	})
}

func (s *Server) handleScanLibraries(w http.ResponseWriter, r *http.Request) {
	// Scan for SCSI medium changer devices using lsscsi
	cmd := exec.Command("lsscsi", "--generic")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Warn("lsscsi scan failed", map[string]interface{}{"error": err.Error()})
		s.respondJSON(w, http.StatusOK, []map[string]string{})
		return
	}

	var changers []map[string]string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for "mediumx" (medium changer) type devices
		if strings.Contains(line, "mediumx") || strings.Contains(line, "changer") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				changer := map[string]string{
					"device_path": fields[len(fields)-1],
					"type":        "medium_changer",
				}
				// Try to extract vendor/model from lsscsi output
				if len(fields) >= 5 {
					changer["vendor"] = fields[2]
					changer["model"] = fields[3]
				}
				changers = append(changers, changer)
			}
		}
	}

	if changers == nil {
		changers = []map[string]string{}
	}
	s.respondJSON(w, http.StatusOK, changers)
}

func (s *Server) handleGetLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	var name, devicePath, vendor, model, serial string
	var numSlots, numDrives, numIE int64
	var barcodeReader, enabled bool
	var lastInventoryAt *time.Time
	var createdAt time.Time

	err = s.db.QueryRow(`
		SELECT name, device_path, vendor, model, serial_number,
		       num_slots, num_drives, num_import_export, barcode_reader,
		       enabled, last_inventory_at, created_at
		FROM tape_libraries WHERE id = ?
	`, id).Scan(&name, &devicePath, &vendor, &model, &serial,
		&numSlots, &numDrives, &numIE, &barcodeReader,
		&enabled, &lastInventoryAt, &createdAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "library not found")
		return
	}

	lib := map[string]interface{}{
		"id":                id,
		"name":              name,
		"device_path":       devicePath,
		"vendor":            vendor,
		"model":             model,
		"serial_number":     serial,
		"num_slots":         numSlots,
		"num_drives":        numDrives,
		"num_import_export": numIE,
		"barcode_reader":    barcodeReader,
		"enabled":           enabled,
		"created_at":        createdAt,
	}
	if lastInventoryAt != nil {
		lib["last_inventory_at"] = *lastInventoryAt
	}

	s.respondJSON(w, http.StatusOK, lib)
}

func (s *Server) handleUpdateLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	var req struct {
		Name    *string `json:"name,omitempty"`
		Enabled *bool   `json:"enabled,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}

	if len(updates) == 0 {
		s.respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	args = append(args, id)
	query := "UPDATE tape_libraries SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.db.Exec(query, args...)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"message": "Library updated"})
}

func (s *Server) handleDeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	// Clean up slots first
	s.db.Exec("DELETE FROM tape_library_slots WHERE library_id = ?", id)
	// Unlink drives
	s.db.Exec("UPDATE tape_drives SET library_id = NULL, library_drive_number = NULL WHERE library_id = ?", id)

	_, err = s.db.Exec("DELETE FROM tape_libraries WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.auditLog(r, "delete", "tape_library", id, fmt.Sprintf("Deleted tape library #%d", id))
	s.respondJSON(w, http.StatusOK, map[string]string{"message": "Library deleted"})
}

func (s *Server) handleLibraryInventory(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	// Get the library device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_libraries WHERE id = ?", id).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "library not found")
		return
	}

	// Run mtx status command to get inventory
	cmd := exec.Command("mtx", "-f", devicePath, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("mtx status failed: %s - %s", err.Error(), string(output)))
		return
	}

	// Parse mtx output and update database
	slots := parseMtxStatus(string(output))

	// Update library metadata
	numStorage := 0
	numDrives := 0
	numIE := 0
	for _, slot := range slots {
		switch slot["slot_type"] {
		case "storage":
			numStorage++
		case "drive":
			numDrives++
		case "import_export":
			numIE++
		}
	}

	s.db.Exec(`
		UPDATE tape_libraries SET num_slots = ?, num_drives = ?, num_import_export = ?,
		       last_inventory_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, numStorage+numIE, numDrives, numIE, id)

	// Clear existing slots and re-populate
	s.db.Exec("DELETE FROM tape_library_slots WHERE library_id = ?", id)

	for _, slot := range slots {
		slotNum := slot["slot_number"]
		slotType := slot["slot_type"]
		barcode := slot["barcode"]
		isEmpty := slot["is_empty"] == "true"

		s.db.Exec(`
			INSERT INTO tape_library_slots (library_id, slot_number, slot_type, barcode, is_empty)
			VALUES (?, ?, ?, ?, ?)
		`, id, slotNum, slotType, barcode, isEmpty)
	}

	s.auditLog(r, "inventory", "tape_library", id, fmt.Sprintf("Inventory completed: %d storage slots, %d drives, %d I/E slots", numStorage, numDrives, numIE))

	// Publish SSE event
	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Library Inventory Complete",
			Message:  fmt.Sprintf("Found %d storage slots, %d drives, %d I/E slots", numStorage, numDrives, numIE),
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"slots":         slots,
		"num_storage":   numStorage,
		"num_drives":    numDrives,
		"num_ie":        numIE,
		"message":       "Inventory completed",
	})
}

// parseMtxStatus parses the output of `mtx -f /dev/sgX status`
func parseMtxStatus(output string) []map[string]string {
	var slots []map[string]string
	lines := strings.Split(output, "\n")

	extractBarcode := func(line, prefix string) string {
		if idx := strings.Index(line, prefix); idx >= 0 {
			bc := strings.TrimSpace(line[idx+len(prefix):])
			// Remove any trailing whitespace or non-printable chars
			bc = strings.Fields(bc)[0]
			return bc
		}
		return ""
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Data Transfer Element") {
			// Drive slot: "Data Transfer Element 0:Full (Storage Element 3 Loaded):VolumeTag = TAPE001"
			slot := map[string]string{
				"slot_type": "drive",
				"is_empty":  "true",
				"barcode":   "",
			}
			// Extract drive number
			parts := strings.SplitN(line, ":", 2)
			numStr := strings.TrimPrefix(parts[0], "Data Transfer Element ")
			slot["slot_number"] = strings.TrimSpace(numStr)

			if strings.Contains(line, "Full") {
				slot["is_empty"] = "false"
			}
			slot["barcode"] = extractBarcode(line, "VolumeTag = ")
			if slot["barcode"] == "" {
				slot["barcode"] = extractBarcode(line, "VolumeTag=")
			}
			slots = append(slots, slot)

		} else if strings.Contains(line, "Storage Element") && strings.Contains(line, "IMPORT/EXPORT") {
			// Import/Export slot
			slot := map[string]string{
				"slot_type": "import_export",
				"is_empty":  "true",
				"barcode":   "",
			}
			parts := strings.SplitN(line, ":", 2)
			numStr := strings.TrimPrefix(parts[0], "      Storage Element ")
			numStr = strings.Split(numStr, " ")[0]
			slot["slot_number"] = strings.TrimSpace(numStr)

			if strings.Contains(line, "Full") {
				slot["is_empty"] = "false"
			}
			slot["barcode"] = extractBarcode(line, "VolumeTag=")
			if slot["barcode"] == "" {
				slot["barcode"] = extractBarcode(line, "VolumeTag = ")
			}
			slots = append(slots, slot)

		} else if strings.Contains(line, "Storage Element") {
			// Normal storage slot: "      Storage Element 1:Full :VolumeTag=TAPE001"
			slot := map[string]string{
				"slot_type": "storage",
				"is_empty":  "true",
				"barcode":   "",
			}
			parts := strings.SplitN(line, ":", 2)
			numStr := strings.TrimPrefix(parts[0], "      Storage Element ")
			slot["slot_number"] = strings.TrimSpace(numStr)

			if strings.Contains(line, "Full") {
				slot["is_empty"] = "false"
			}
			slot["barcode"] = extractBarcode(line, "VolumeTag=")
			if slot["barcode"] == "" {
				slot["barcode"] = extractBarcode(line, "VolumeTag = ")
			}
			slots = append(slots, slot)
		}
	}

	return slots
}

func (s *Server) handleListLibrarySlots(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	rows, err := s.db.Query(`
		SELECT s.id, s.slot_number, s.slot_type, s.tape_id, s.barcode, s.is_empty, s.drive_id,
		       t.label as tape_label
		FROM tape_library_slots s
		LEFT JOIN tapes t ON s.tape_id = t.id
		WHERE s.library_id = ?
		ORDER BY s.slot_type, s.slot_number
	`, id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	slots := make([]map[string]interface{}, 0)
	for rows.Next() {
		var slotID, slotNumber int64
		var slotType, barcode string
		var tapeID, driveID *int64
		var isEmpty bool
		var tapeLabel *string

		if err := rows.Scan(&slotID, &slotNumber, &slotType, &tapeID, &barcode, &isEmpty, &driveID, &tapeLabel); err != nil {
			continue
		}

		slot := map[string]interface{}{
			"id":          slotID,
			"slot_number": slotNumber,
			"slot_type":   slotType,
			"barcode":     barcode,
			"is_empty":    isEmpty,
		}
		if tapeID != nil {
			slot["tape_id"] = *tapeID
		}
		if driveID != nil {
			slot["drive_id"] = *driveID
		}
		if tapeLabel != nil {
			slot["tape_label"] = *tapeLabel
		}
		slots = append(slots, slot)
	}

	s.respondJSON(w, http.StatusOK, slots)
}

func (s *Server) handleLibraryLoad(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	var req struct {
		SlotNumber  int `json:"slot_number"`
		DriveNumber int `json:"drive_number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get library device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_libraries WHERE id = ?", id).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "library not found")
		return
	}

	// Run mtx load command
	cmd := exec.Command("mtx", "-f", devicePath, "load", strconv.Itoa(req.SlotNumber), strconv.Itoa(req.DriveNumber))
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("mtx load failed: %s - %s", err.Error(), string(output)))
		return
	}

	s.auditLog(r, "load", "tape_library", id, fmt.Sprintf("Loaded tape from slot %d to drive %d", req.SlotNumber, req.DriveNumber))

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Tape Loaded",
			Message:  fmt.Sprintf("Loaded tape from slot %d to drive %d", req.SlotNumber, req.DriveNumber),
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Tape loaded from slot %d to drive %d", req.SlotNumber, req.DriveNumber),
	})
}

func (s *Server) handleLibraryUnload(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	var req struct {
		SlotNumber  int `json:"slot_number"`
		DriveNumber int `json:"drive_number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_libraries WHERE id = ?", id).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "library not found")
		return
	}

	cmd := exec.Command("mtx", "-f", devicePath, "unload", strconv.Itoa(req.SlotNumber), strconv.Itoa(req.DriveNumber))
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("mtx unload failed: %s - %s", err.Error(), string(output)))
		return
	}

	s.auditLog(r, "unload", "tape_library", id, fmt.Sprintf("Unloaded tape from drive %d to slot %d", req.DriveNumber, req.SlotNumber))

	if s.eventBus != nil {
		s.eventBus.Publish(SystemEvent{
			Type:     "info",
			Category: "tape",
			Title:    "Tape Unloaded",
			Message:  fmt.Sprintf("Unloaded tape from drive %d to slot %d", req.DriveNumber, req.SlotNumber),
		})
	}

	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Tape unloaded from drive %d to slot %d", req.DriveNumber, req.SlotNumber),
	})
}

func (s *Server) handleLibraryTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid library id")
		return
	}

	var req struct {
		SourceSlot int `json:"source_slot"`
		DestSlot   int `json:"dest_slot"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_libraries WHERE id = ?", id).Scan(&devicePath)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "library not found")
		return
	}

	cmd := exec.Command("mtx", "-f", devicePath, "transfer", strconv.Itoa(req.SourceSlot), strconv.Itoa(req.DestSlot))
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("mtx transfer failed: %s - %s", err.Error(), string(output)))
		return
	}

	s.auditLog(r, "transfer", "tape_library", id, fmt.Sprintf("Transferred tape from slot %d to slot %d", req.SourceSlot, req.DestSlot))
	s.respondJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Tape transferred from slot %d to slot %d", req.SourceSlot, req.DestSlot),
	})
}
