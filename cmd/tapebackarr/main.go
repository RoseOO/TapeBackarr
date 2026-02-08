package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/api"
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
)

var (
	version   = "0.1.0"
	buildTime = "development"
)

func main() {
	// Command line flags
	configPath := flag.String("config", "/etc/tapebackarr/config.json", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	initConfig := flag.Bool("init-config", false, "Create default configuration file")
	flag.Parse()

	if *showVersion {
		fmt.Printf("TapeBackarr v%s (built: %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if *initConfig {
		if err := cfg.Save(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Configuration saved to %s\n", *configPath)
		os.Exit(0)
	}

	// Initialize logger
	logger, err := logging.NewLogger(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.OutputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	logger.Info("Starting TapeBackarr", map[string]interface{}{
		"version": version,
		"config":  *configPath,
	})

	// Initialize database
	db, err := database.New(cfg.Database.Path)
	if err != nil {
		logger.Error("Failed to initialize database", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		logger.Error("Failed to run migrations", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	logger.Info("Database initialized", map[string]interface{}{"path": cfg.Database.Path})

	// Initialize services
	tapeService := tape.NewService(cfg.Tape.DefaultDevice, cfg.Tape.BlockSize)
	authService := auth.NewService(db, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiration)

	// Initialize notification service
	telegramService := notifications.NewTelegramService(notifications.TelegramConfig{
		Enabled:  cfg.Notifications.Telegram.Enabled,
		BotToken: cfg.Notifications.Telegram.BotToken,
		ChatID:   cfg.Notifications.Telegram.ChatID,
	})

	if telegramService.IsEnabled() {
		logger.Info("Telegram notifications enabled", nil)
	}

	// Create backup service
	backupService := backup.NewService(db, tapeService, logger, cfg.Tape.BlockSize)

	// Create restore service
	restoreService := restore.NewService(db, tapeService, logger, cfg.Tape.BlockSize)

	// Create encryption service
	encryptionService := encryption.NewService(db, logger)

	// Create job runner for scheduler
	jobRunner := func(ctx context.Context, job *models.BackupJob) error {
		// Get source
		var source models.BackupSource
		err := db.QueryRow(`
			SELECT id, name, source_type, path, include_patterns, exclude_patterns
			FROM backup_sources WHERE id = ?
		`, job.SourceID).Scan(&source.ID, &source.Name, &source.SourceType, &source.Path,
			&source.IncludePatterns, &source.ExcludePatterns)
		if err != nil {
			// Notify on failure
			telegramService.NotifyBackupFailed(ctx, job.Name, fmt.Sprintf("source not found: %v", err))
			return fmt.Errorf("source not found: %w", err)
		}

		// Get an available tape from the pool
		var tapeID int64
		err = db.QueryRow(`
			SELECT id FROM tapes 
			WHERE pool_id = ? AND status IN ('blank', 'active')
			ORDER BY used_bytes ASC LIMIT 1
		`, job.PoolID).Scan(&tapeID)
		if err != nil {
			// Notify that tape change is required
			telegramService.NotifyTapeChangeRequired(ctx, job.Name, "", "no available tape in pool")
			return fmt.Errorf("no available tape in pool: %w", err)
		}

		// Notify backup started
		telegramService.NotifyBackupStarted(ctx, job.Name, 1, string(job.BackupType))

		startTime := time.Now()
		result, err := backupService.RunBackup(ctx, job, &source, tapeID, job.BackupType)
		if err != nil {
			telegramService.NotifyBackupFailed(ctx, job.Name, err.Error())
			return err
		}

		// Notify backup completed
		duration := time.Since(startTime)
		telegramService.NotifyBackupCompleted(ctx, job.Name, result.FileCount, result.TotalBytes, duration)

		return nil
	}

	// Create scheduler
	schedulerService := scheduler.NewService(db, logger, jobRunner)

	// Initialize Proxmox services if configured
	var proxmoxClient *proxmox.Client
	var proxmoxBackupService *proxmox.BackupService
	var proxmoxRestoreService *proxmox.RestoreService

	if cfg.Proxmox.Enabled && cfg.Proxmox.Host != "" {
		logger.Info("Initializing Proxmox integration", map[string]interface{}{
			"host": cfg.Proxmox.Host,
			"port": cfg.Proxmox.Port,
		})

		proxmoxCfg := &proxmox.ClientConfig{
			Host:          cfg.Proxmox.Host,
			Port:          cfg.Proxmox.Port,
			SkipTLSVerify: cfg.Proxmox.SkipTLSVerify,
			Username:      cfg.Proxmox.Username,
			Password:      cfg.Proxmox.Password,
			Realm:         cfg.Proxmox.Realm,
			TokenID:       cfg.Proxmox.TokenID,
			TokenSecret:   cfg.Proxmox.TokenSecret,
		}

		var err error
		proxmoxClient, err = proxmox.NewClient(proxmoxCfg)
		if err != nil {
			logger.Warn("Failed to initialize Proxmox client", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			proxmoxBackupService = proxmox.NewBackupService(proxmoxClient, db, tapeService, logger, cfg.Tape.BlockSize)
			proxmoxRestoreService = proxmox.NewRestoreService(proxmoxClient, db, tapeService, logger, cfg.Tape.BlockSize)

			if cfg.Proxmox.TempDir != "" {
				proxmoxBackupService.SetTempDir(cfg.Proxmox.TempDir)
				proxmoxRestoreService.SetTempDir(cfg.Proxmox.TempDir)
			}

			logger.Info("Proxmox integration initialized successfully", nil)
		}
	}

	// Create API server
	server := api.NewServer(
		db,
		authService,
		tapeService,
		backupService,
		restoreService,
		encryptionService,
		schedulerService,
		logger,
		proxmoxClient,
		proxmoxBackupService,
		proxmoxRestoreService,
		cfg.Server.StaticDir,
	)

	// Start scheduler
	if err := schedulerService.Start(); err != nil {
		logger.Error("Failed to start scheduler", map[string]interface{}{"error": err.Error()})
	}

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long timeout for tape operations
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting HTTP server", map[string]interface{}{"address": addr})
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("HTTP server error", map[string]interface{}{"error": err.Error()})
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("Received shutdown signal", map[string]interface{}{"signal": sig.String()})

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop scheduler
	schedulerService.Stop()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", map[string]interface{}{"error": err.Error()})
	}

	logger.Info("TapeBackarr shutdown complete", nil)
}
