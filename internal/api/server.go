package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/auth"
	"github.com/RoseOO/TapeBackarr/internal/backup"
	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/encryption"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/models"
	"github.com/RoseOO/TapeBackarr/internal/proxmox"
	"github.com/RoseOO/TapeBackarr/internal/restore"
	"github.com/RoseOO/TapeBackarr/internal/scheduler"
	"github.com/RoseOO/TapeBackarr/internal/tape"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

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
	}

	s.setupRoutes()
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
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
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
			r.Post("/", s.handleCreateTape)
			r.Get("/{id}", s.handleGetTape)
			r.Put("/{id}", s.handleUpdateTape)
			r.Delete("/{id}", s.handleDeleteTape)
			r.Post("/{id}/label", s.handleLabelTape)
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
			r.Get("/{id}/status", s.handleDriveStatus)
			r.Put("/{id}", s.handleUpdateDrive)
			r.Delete("/{id}", s.handleDeleteDrive)
			r.Post("/{id}/eject", s.handleEjectTape)
			r.Post("/{id}/rewind", s.handleRewindTape)
			r.Post("/{id}/select", s.handleSelectDrive)
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
			r.Get("/{id}", s.handleGetJob)
			r.Put("/{id}", s.handleUpdateJob)
			r.Delete("/{id}", s.handleDeleteJob)
			r.Post("/{id}/run", s.handleRunJob)
		})

		// Backup Sets
		r.Route("/api/v1/backup-sets", func(r chi.Router) {
			r.Get("/", s.handleListBackupSets)
			r.Get("/{id}", s.handleGetBackupSet)
			r.Get("/{id}/files", s.handleListBackupFiles)
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
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}

// Middleware

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.respondError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.respondError(w, http.StatusUnauthorized, "invalid authorization header")
			return
		}

		claims, err := s.authService.ValidateToken(parts[1])
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
	var stats struct {
		TotalTapes     int    `json:"total_tapes"`
		ActiveTapes    int    `json:"active_tapes"`
		TotalJobs      int    `json:"total_jobs"`
		RunningJobs    int    `json:"running_jobs"`
		RecentBackups  int    `json:"recent_backups"`
		DriveStatus    string `json:"drive_status"`
		TotalDataBytes int64  `json:"total_data_bytes"`
	}

	s.db.QueryRow("SELECT COUNT(*) FROM tapes").Scan(&stats.TotalTapes)
	s.db.QueryRow("SELECT COUNT(*) FROM tapes WHERE status = 'active'").Scan(&stats.ActiveTapes)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_jobs").Scan(&stats.TotalJobs)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE status = 'running'").Scan(&stats.RunningJobs)
	s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE start_time > datetime('now', '-24 hours')").Scan(&stats.RecentBackups)
	s.db.QueryRow("SELECT COALESCE(SUM(total_bytes), 0) FROM backup_sets WHERE status = 'completed'").Scan(&stats.TotalDataBytes)

	// Get drive status
	ctx := r.Context()
	status, err := s.tapeService.GetStatus(ctx)
	if err != nil {
		stats.DriveStatus = "error"
	} else if status.Online {
		stats.DriveStatus = "online"
	} else {
		stats.DriveStatus = "offline"
	}

	s.respondJSON(w, http.StatusOK, stats)
}

// Tape handlers

func (s *Server) handleListTapes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT t.id, t.barcode, t.label, t.pool_id, tp.name as pool_name, t.status, 
		       t.capacity_bytes, t.used_bytes, t.write_count, t.last_written_at, t.created_at
		FROM tapes t
		LEFT JOIN tape_pools tp ON t.pool_id = tp.id
		ORDER BY t.label
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var tapes []map[string]interface{}
	for rows.Next() {
		var t models.Tape
		var poolName *string
		if err := rows.Scan(&t.ID, &t.Barcode, &t.Label, &t.PoolID, &poolName, &t.Status,
			&t.CapacityBytes, &t.UsedBytes, &t.WriteCount, &t.LastWrittenAt, &t.CreatedAt); err != nil {
			continue
		}
		tape := map[string]interface{}{
			"id":              t.ID,
			"barcode":         t.Barcode,
			"label":           t.Label,
			"pool_id":         t.PoolID,
			"pool_name":       poolName,
			"status":          t.Status,
			"capacity_bytes":  t.CapacityBytes,
			"used_bytes":      t.UsedBytes,
			"write_count":     t.WriteCount,
			"last_written_at": t.LastWrittenAt,
			"created_at":      t.CreatedAt,
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
		CapacityBytes int64  `json:"capacity_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.db.Exec(`
		INSERT INTO tapes (barcode, label, pool_id, status, capacity_bytes)
		VALUES (?, ?, ?, 'blank', ?)
	`, req.Barcode, req.Label, req.PoolID, req.CapacityBytes)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	s.respondJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleGetTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var t models.Tape
	err = s.db.QueryRow(`
		SELECT id, barcode, label, pool_id, status, capacity_bytes, used_bytes, 
		       write_count, last_written_at, offsite_location, created_at, updated_at
		FROM tapes WHERE id = ?
	`, id).Scan(&t.ID, &t.Barcode, &t.Label, &t.PoolID, &t.Status, &t.CapacityBytes, &t.UsedBytes,
		&t.WriteCount, &t.LastWrittenAt, &t.OffsiteLocation, &t.CreatedAt, &t.UpdatedAt)
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
		PoolID          *int64             `json:"pool_id"`
		Status          *models.TapeStatus `json:"status"`
		OffsiteLocation *string            `json:"offsite_location"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}

	if req.Label != nil {
		updates = append(updates, "label = ?")
		args = append(args, *req.Label)
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

	_, err = s.db.Exec("DELETE FROM tapes WHERE id = ?", id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleLabelTape(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid tape id")
		return
	}

	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()

	// Write label to tape
	if err := s.tapeService.WriteTapeLabel(ctx, req.Label); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to write tape label: "+err.Error())
		return
	}

	// Update database
	_, err = s.db.Exec("UPDATE tapes SET label = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.Label, id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "labeled"})
}

// Pool handlers

func (s *Server) handleListPools(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id, name, description, retention_days, created_at FROM tape_pools ORDER BY name")
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var pools []models.TapePool
	for rows.Next() {
		var p models.TapePool
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RetentionDays, &p.CreatedAt); err != nil {
			continue
		}
		pools = append(pools, p)
	}

	s.respondJSON(w, http.StatusOK, pools)
}

func (s *Server) handleCreatePool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		RetentionDays int    `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.db.Exec(`
		INSERT INTO tape_pools (name, description, retention_days)
		VALUES (?, ?, ?)
	`, req.Name, req.Description, req.RetentionDays)
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
		SELECT id, name, description, retention_days, created_at, updated_at
		FROM tape_pools WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.RetentionDays, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "pool not found")
		return
	}

	s.respondJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdatePool(w http.ResponseWriter, r *http.Request) {
	id, err := s.getIDParam(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid pool id")
		return
	}

	var req struct {
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		RetentionDays *int    `json:"retention_days"`
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
		SELECT id, device_path, serial_number, model, status, current_tape_id, created_at
		FROM tape_drives ORDER BY device_path
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var drives []models.TapeDrive
	for rows.Next() {
		var d models.TapeDrive
		if err := rows.Scan(&d.ID, &d.DevicePath, &d.SerialNumber, &d.Model, &d.Status, &d.CurrentTapeID, &d.CreatedAt); err != nil {
			continue
		}
		drives = append(drives, d)
	}

	s.respondJSON(w, http.StatusOK, drives)
}

func (s *Server) handleDriveStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := s.tapeService.GetStatus(ctx)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleEjectTape(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.tapeService.Eject(ctx); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to eject tape: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ejected"})
}

func (s *Server) handleRewindTape(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.tapeService.Rewind(ctx); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to rewind tape: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "rewound"})
}

// Source handlers

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT id, name, source_type, path, include_patterns, exclude_patterns, enabled, created_at
		FROM backup_sources ORDER BY name
	`)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var sources []models.BackupSource
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

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// Job handlers

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT j.id, j.name, j.source_id, s.name as source_name, j.pool_id, p.name as pool_name,
		       j.backup_type, j.schedule_cron, j.retention_days, j.enabled, j.last_run_at, j.next_run_at
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

	var jobs []map[string]interface{}
	for rows.Next() {
		var j models.BackupJob
		var sourceName, poolName *string
		if err := rows.Scan(&j.ID, &j.Name, &j.SourceID, &sourceName, &j.PoolID, &poolName,
			&j.BackupType, &j.ScheduleCron, &j.RetentionDays, &j.Enabled, &j.LastRunAt, &j.NextRunAt); err != nil {
			continue
		}
		job := map[string]interface{}{
			"id":             j.ID,
			"name":           j.Name,
			"source_id":      j.SourceID,
			"source_name":    sourceName,
			"pool_id":        j.PoolID,
			"pool_name":      poolName,
			"backup_type":    j.BackupType,
			"schedule_cron":  j.ScheduleCron,
			"retention_days": j.RetentionDays,
			"enabled":        j.Enabled,
			"last_run_at":    j.LastRunAt,
			"next_run_at":    j.NextRunAt,
		}
		jobs = append(jobs, job)
	}

	s.respondJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string `json:"name"`
		SourceID      int64  `json:"source_id"`
		PoolID        int64  `json:"pool_id"`
		BackupType    string `json:"backup_type"`
		ScheduleCron  string `json:"schedule_cron"`
		RetentionDays int    `json:"retention_days"`
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

	result, err := s.db.Exec(`
		INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, schedule_cron, retention_days, enabled)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, req.Name, req.SourceID, req.PoolID, req.BackupType, req.ScheduleCron, req.RetentionDays)
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
		Name          *string `json:"name"`
		SourceID      *int64  `json:"source_id"`
		PoolID        *int64  `json:"pool_id"`
		BackupType    *string `json:"backup_type"`
		ScheduleCron  *string `json:"schedule_cron"`
		RetentionDays *int    `json:"retention_days"`
		Enabled       *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
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
		BackupType string `json:"backup_type"` // Override job's backup type
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get job details
	var job models.BackupJob
	err = s.db.QueryRow(`
		SELECT id, name, source_id, pool_id, backup_type, retention_days
		FROM backup_jobs WHERE id = ?
	`, id).Scan(&job.ID, &job.Name, &job.SourceID, &job.PoolID, &job.BackupType, &job.RetentionDays)
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

	// Run backup in background
	go func() {
		ctx := context.Background()
		s.backupService.RunBackup(ctx, &job, &source, req.TapeID, backupType)
	}()

	s.respondJSON(w, http.StatusAccepted, map[string]string{
		"status":  "started",
		"message": "Backup job started in background",
	})
}

// Backup set handlers

func (s *Server) handleListBackupSets(w http.ResponseWriter, r *http.Request) {
	jobIDStr := r.URL.Query().Get("job_id")
	limit := 50

	query := `
		SELECT bs.id, bs.job_id, j.name as job_name, bs.tape_id, t.label as tape_label,
		       bs.backup_type, bs.start_time, bs.end_time, bs.status, bs.file_count, bs.total_bytes
		FROM backup_sets bs
		LEFT JOIN backup_jobs j ON bs.job_id = j.id
		LEFT JOIN tapes t ON bs.tape_id = t.id
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

	var sets []map[string]interface{}
	for rows.Next() {
		var bs models.BackupSet
		var jobName, tapeLabel *string
		if err := rows.Scan(&bs.ID, &bs.JobID, &jobName, &bs.TapeID, &tapeLabel,
			&bs.BackupType, &bs.StartTime, &bs.EndTime, &bs.Status, &bs.FileCount, &bs.TotalBytes); err != nil {
			continue
		}
		set := map[string]interface{}{
			"id":          bs.ID,
			"job_id":      bs.JobID,
			"job_name":    jobName,
			"tape_id":     bs.TapeID,
			"tape_label":  tapeLabel,
			"backup_type": bs.BackupType,
			"start_time":  bs.StartTime,
			"end_time":    bs.EndTime,
			"status":      bs.Status,
			"file_count":  bs.FileCount,
			"total_bytes": bs.TotalBytes,
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

	var logs []map[string]interface{}
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

	var logs []map[string]interface{}
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

	// Use SQLite backup command
	_, err = s.db.Exec("VACUUM INTO ?", backupPath)
	if err != nil {
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", err.Error(), backupID)
		return
	}

	// Get file info
	info, err := os.Stat(backupPath)
	if err != nil {
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", err.Error(), backupID)
		return
	}

	// Calculate checksum
	checksum, _ := calculateFileChecksum(backupPath)

	// Position tape and write
	if err := s.tapeService.Rewind(ctx); err != nil {
		s.db.Exec("UPDATE database_backups SET status = 'failed', error_message = ? WHERE id = ?", "failed to rewind: "+err.Error(), backupID)
		return
	}

	// Skip past tape label to first file position
	// Database backups are written after the label block (file 0)
	s.tapeService.SeekToFileNumber(ctx, 1)

	// Stream database backup to tape using tar
	tarArgs := []string{"-c", "-f", devicePath, "-C", tempDir, "tapebackarr.db"}
	cmd := exec.CommandContext(ctx, "tar", tarArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
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
		SerialNumber string `json:"serial_number"`
		Model        string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.db.Exec(`
		INSERT INTO tape_drives (device_path, display_name, serial_number, model, status, enabled)
		VALUES (?, ?, ?, ?, 'offline', 1)
	`, req.DevicePath, req.DisplayName, req.SerialNumber, req.Model)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
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
		SELECT id, name, description, node, vmid_filter, guest_type_filter, tag_filter,
		       pool_id, backup_mode, compress, schedule_cron, retention_days,
		       enabled, last_run_at, next_run_at, created_at
		FROM proxmox_backup_jobs
		ORDER BY created_at DESC
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
		var enabled bool
		var lastRunAt, nextRunAt *time.Time
		var createdAt time.Time

		if err := rows.Scan(&id, &name, &description, &node, &vmidFilter, &guestTypeFilter, &tagFilter,
			&poolID, &backupMode, &compress, &scheduleCron, &retentionDays,
			&enabled, &lastRunAt, &nextRunAt, &createdAt); err != nil {
			continue
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
		VMIDFilter      string `json:"vmid_filter,omitempty"`
		GuestTypeFilter string `json:"guest_type_filter,omitempty"`
		TagFilter       string `json:"tag_filter,omitempty"`
		PoolID          *int64 `json:"pool_id,omitempty"`
		BackupMode      string `json:"backup_mode"`
		Compress        string `json:"compress"`
		ScheduleCron    string `json:"schedule_cron"`
		RetentionDays   int    `json:"retention_days"`
		Enabled         bool   `json:"enabled"`
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
	if req.Compress == "" {
		req.Compress = "zstd"
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = 30
	}

	result, err := s.db.Exec(`
		INSERT INTO proxmox_backup_jobs (
			name, description, node, vmid_filter, guest_type_filter, tag_filter,
			pool_id, backup_mode, compress, schedule_cron, retention_days, enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Description, req.Node, req.VMIDFilter, req.GuestTypeFilter, req.TagFilter,
		req.PoolID, req.BackupMode, req.Compress, req.ScheduleCron, req.RetentionDays, req.Enabled)
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
		VMIDFilter      string `json:"vmid_filter,omitempty"`
		GuestTypeFilter string `json:"guest_type_filter,omitempty"`
		TagFilter       string `json:"tag_filter,omitempty"`
		PoolID          *int64 `json:"pool_id,omitempty"`
		BackupMode      string `json:"backup_mode,omitempty"`
		Compress        string `json:"compress,omitempty"`
		ScheduleCron    string `json:"schedule_cron,omitempty"`
		RetentionDays   *int   `json:"retention_days,omitempty"`
		Enabled         *bool  `json:"enabled,omitempty"`
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
	if req.VMIDFilter != "" {
		updates = append(updates, "vmid_filter = ?")
		args = append(args, req.VMIDFilter)
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
	if req.Compress != "" {
		updates = append(updates, "compress = ?")
		args = append(args, req.Compress)
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
