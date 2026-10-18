package cron

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/db"
)

// BackupJobName is the identifier for the scheduled automated remote backup job.
const BackupJobName = "backup.s3"

// JobRegistrar is the minimal interface required to register a scheduled cron job.
type JobRegistrar interface {
	Register(name string, spec string, handler func(ctx context.Context) error) error
}

// RegisterBackupJob checks if remote S3 backup credentials are fully configured.
// If not configured, it silently skips without noise (Fail-Closed / No-Noise constraint).
// If configured, it registers the automated backup task into the cron engine.
func RegisterBackupJob(engine JobRegistrar, cfg *config.Config) error {
	if cfg == nil || !cfg.BackupS3Configured() {
		return nil
	}

	cronSpec := cfg.BackupCron
	if cronSpec == "" {
		cronSpec = "0 3 * * *"
	}

	return engine.Register(BackupJobName, cronSpec, func(ctx context.Context) error {
		return ExecuteBackupWorkflow(ctx, cfg)
	})
}

// ExecuteBackupWorkflow executes the 5-step automated backup pipeline:
// 1. db.Backup(cfg, tmpPath)
// 2. crypto.EncryptBackup(tmpPath, encPath, cfg.SecretKey)
// 3. uploader.Upload(ctx, encPath)
// 4. uploader.PruneOlderThan(ctx, cfg.BackupRetentionDays)
// 5. Audit log and delete local temporary snapshots.
func ExecuteBackupWorkflow(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || !cfg.BackupS3Configured() {
		return fmt.Errorf("cron backup: S3 backup credentials not configured")
	}

	uploader, err := db.NewS3Uploader(cfg)
	if err != nil {
		slog.Error("cron backup: failed to initialize s3 uploader", "err", err)
		return err
	}

	tmpDir := os.TempDir()
	ts := time.Now().UTC().Format("20060102-150405")
	baseName := fmt.Sprintf("octarq-backup-%s", ts)
	rawFilename := baseName + ".db"
	if cfg.DBDriver == "postgres" || cfg.DBDriver == "mysql" {
		rawFilename = baseName + ".sql"
	}
	encFilename := baseName + ".db.enc"

	rawPath := filepath.Join(tmpDir, rawFilename)
	encPath := filepath.Join(tmpDir, encFilename)

	defer func() {
		_ = os.Remove(rawPath)
		_ = os.Remove(encPath)
	}()

	// Step 1: Online non-locking database backup
	slog.Info("cron backup: starting database backup", "out", rawPath)
	if err := db.Backup(cfg, rawPath); err != nil {
		slog.Error("cron backup: database backup failed", "err", err)
		return fmt.Errorf("database backup failed: %w", err)
	}

	// Step 2: Zero-knowledge AES-256-GCM encryption
	slog.Info("cron backup: encrypting backup", "enc", encPath)
	if err := crypto.EncryptBackup(rawPath, encPath, cfg.SecretKey); err != nil {
		slog.Error("cron backup: encryption failed", "err", err)
		return fmt.Errorf("backup encryption failed: %w", err)
	}

	// Step 3: Remote upload to S3 / Cloudflare R2
	slog.Info("cron backup: uploading encrypted snapshot to S3", "key", encFilename, "bucket", cfg.BackupS3Bucket)
	if err := uploader.Upload(ctx, encPath); err != nil {
		slog.Error("cron backup: upload failed", "err", err)
		return fmt.Errorf("backup upload failed: %w", err)
	}

	// Step 4: Prune remote snapshots older than retention days
	if cfg.BackupRetentionDays > 0 {
		pruned, err := uploader.PruneOlderThan(ctx, cfg.BackupRetentionDays)
		if err != nil {
			slog.Warn("cron backup: pruning older snapshots encountered error", "err", err)
		} else if pruned > 0 {
			slog.Info("cron backup: pruned old snapshots", "count", pruned, "retentionDays", cfg.BackupRetentionDays)
		}
	}

	// Step 5: Audit log
	slog.Info("cron backup: successfully completed automated encrypted backup", "key", encFilename, "bucket", cfg.BackupS3Bucket)
	return nil
}
