package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/db"
)

func runBackupCommand(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	outPath := fs.String("out", "", "Output file path for backup (optional)")
	fs.StringVar(outPath, "o", "", "Output file path for backup (shorthand)")
	remote := fs.Bool("remote", false, "Upload encrypted backup to remote S3/R2 storage")
	encrypt := fs.Bool("encrypt", false, "Encrypt backup with AES-256-GCM")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "err", err)
		return 1
	}

	shouldEncrypt := *encrypt
	if *remote && !flagIsExplicitlyFalse(args, "encrypt") {
		shouldEncrypt = true
	}

	if *remote && !cfg.BackupS3Configured() {
		slog.Error("remote backup requested but S3 bucket/credentials are not configured in OCTARQ_BACKUP_S3_*")
		fmt.Fprintln(os.Stderr, "Error: remote backup requested but S3 bucket/credentials are not configured in OCTARQ_BACKUP_S3_*")
		return 1
	}

	finalOut := *outPath
	if finalOut == "" {
		if shouldEncrypt {
			ts := time.Now().Format("20060102-150405")
			finalOut = fmt.Sprintf("octarq-backup-%s.db.enc", ts)
		} else {
			finalOut = db.DefaultBackupFilename(cfg.DBDriver, time.Now())
		}
	}

	var backupFile string
	if shouldEncrypt {
		tmpRaw := filepath.Join(os.TempDir(), fmt.Sprintf("octarq-raw-%d.db", time.Now().UnixNano()))
		defer os.Remove(tmpRaw)
		backupFile = tmpRaw
	} else {
		backupFile = finalOut
	}

	slog.Info("starting database backup", "driver", cfg.DBDriver, "out", backupFile)
	if err := db.Backup(cfg, backupFile); err != nil {
		slog.Error("backup failed", "err", err)
		return 1
	}

	if shouldEncrypt {
		slog.Info("encrypting backup with AES-256-GCM", "out", finalOut)
		if err := crypto.EncryptBackup(backupFile, finalOut, cfg.SecretKey); err != nil {
			slog.Error("backup encryption failed", "err", err)
			fmt.Fprintf(os.Stderr, "Error: backup encryption failed: %v\n", err)
			return 1
		}
	}

	info, err := os.Stat(finalOut)
	var sizeStr string
	if err == nil {
		sizeStr = formatBytes(info.Size())
	}

	if *remote {
		uploader, err := db.NewS3Uploader(cfg)
		if err != nil {
			slog.Error("s3 uploader initialization failed", "err", err)
			fmt.Fprintf(os.Stderr, "Error: S3 initialization failed: %v\n", err)
			return 1
		}
		slog.Info("uploading backup to S3", "key", filepath.Base(finalOut), "bucket", cfg.BackupS3Bucket)
		if err := uploader.Upload(context.Background(), finalOut); err != nil {
			slog.Error("s3 upload failed", "err", err)
			fmt.Fprintf(os.Stderr, "Error: S3 upload failed: %v\n", err)
			return 1
		}
		fmt.Printf("Uploaded backup to s3://%s/%s (%s)\n", cfg.BackupS3Bucket, filepath.Base(finalOut), sizeStr)
	}

	fmt.Printf("Backup complete: %s (%s)\n", finalOut, sizeStr)
	return 0
}

func flagIsExplicitlyFalse(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name+"=false" || a == "-"+name+"=false" {
			return true
		}
	}
	return false
}

func runRestoreCommand(args []string) int {
	return runRestoreCommandWithInput(args, os.Stdin)
}

// runRestoreCommandWithInput is the body of runRestoreCommand with the prompt's
// reader injected, so the interactive confirm path is testable without tying up
// a real stdin. runRestoreCommand passes os.Stdin, preserving the CLI behaviour.
func runRestoreCommandWithInput(args []string, in io.Reader) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	inPath := fs.String("in", "", "Input backup file path to restore from (required unless --from-s3 is specified)")
	fs.StringVar(inPath, "i", "", "Input backup file path (shorthand)")
	fromS3 := fs.String("from-s3", "", "Restore directly from remote S3 snapshot key (or 'latest')")
	decrypt := fs.Bool("decrypt", false, "Decrypt backup file using OCTARQ_SECRET_KEY before restoring")
	confirm := fs.Bool("yes", false, "Confirm restore operation without prompt")
	fs.BoolVar(confirm, "confirm", false, "Confirm restore operation without prompt")
	fs.BoolVar(confirm, "y", false, "Confirm restore operation without prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *inPath == "" && *fromS3 == "" {
		fmt.Println("Error: either --in <path> or --from-s3 <key> is required for restore")
		fs.Usage()
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "err", err)
		return 1
	}

	var restoreSource string
	var cleanupFiles []string
	defer func() {
		for _, f := range cleanupFiles {
			_ = os.Remove(f)
		}
	}()

	remoteRestoreKey := *fromS3
	if *fromS3 != "" {
		if !cfg.BackupS3Configured() {
			slog.Error("remote restore requested but S3 bucket/credentials are not configured")
			fmt.Fprintln(os.Stderr, "Error: remote restore requested but S3 bucket/credentials are not configured in OCTARQ_BACKUP_S3_*")
			return 1
		}
		uploader, err := db.NewS3Uploader(cfg)
		if err != nil {
			slog.Error("s3 uploader initialization failed", "err", err)
			fmt.Fprintf(os.Stderr, "Error: S3 initialization failed: %v\n", err)
			return 1
		}

		key := *fromS3
		if key == "latest" || key == "" {
			latestKey, err := uploader.LatestBackupKey(context.Background())
			if err != nil {
				slog.Error("failed to find latest backup in S3", "err", err)
				fmt.Fprintf(os.Stderr, "Error: failed to find latest backup in S3: %v\n", err)
				return 1
			}
			key = latestKey
		}
		remoteRestoreKey = key

		tmpDownloaded := filepath.Join(os.TempDir(), fmt.Sprintf("octarq-download-%d-%s", time.Now().UnixNano(), filepath.Base(key)))
		cleanupFiles = append(cleanupFiles, tmpDownloaded)
		slog.Info("downloading backup from S3", "key", key, "bucket", cfg.BackupS3Bucket)
		if err := uploader.Download(context.Background(), key, tmpDownloaded); err != nil {
			slog.Error("download backup from S3 failed", "err", err)
			fmt.Fprintf(os.Stderr, "Error: download backup from S3 failed: %v\n", err)
			return 1
		}
		restoreSource = tmpDownloaded
		if strings.HasSuffix(key, ".enc") {
			*decrypt = true
		}
	} else {
		restoreSource = *inPath
		if strings.HasSuffix(*inPath, ".enc") {
			*decrypt = true
		}
	}

	if *decrypt {
		tmpDecrypted := filepath.Join(os.TempDir(), fmt.Sprintf("octarq-decrypted-%d.db", time.Now().UnixNano()))
		cleanupFiles = append(cleanupFiles, tmpDecrypted)
		slog.Info("decrypting backup before restore", "in", restoreSource, "out", tmpDecrypted)
		if err := crypto.DecryptBackup(restoreSource, tmpDecrypted, cfg.SecretKey); err != nil {
			slog.Error("decrypting backup failed", "err", err)
			fmt.Fprintf(os.Stderr, "Error: decrypting backup failed: %v\n", err)
			return 1
		}
		restoreSource = tmpDecrypted
	}

	if !*confirm {
		targetDesc := *inPath
		if *fromS3 != "" {
			targetDesc = fmt.Sprintf("s3://%s/%s", cfg.BackupS3Bucket, remoteRestoreKey)
		}
		fmt.Printf("WARNING: Restoring database from '%s' will overwrite existing data.\n", targetDesc)
		fmt.Printf("An automatic safety backup of the current database will be created first.\n")
		fmt.Print("Type 'yes' to confirm and proceed: ")

		reader := bufio.NewReader(in)
		input, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "yes" {
			fmt.Println("Restore cancelled by user.")
			return 1
		}
	}

	// Pre-restore safety net: the current database is backed up before it is overwritten.
	autoBackupPath := fmt.Sprintf("octarq-backup-before-restore-%s", db.DefaultBackupFilename(cfg.DBDriver, time.Now()))
	slog.Info("creating safety backup of current database before restore", "out", autoBackupPath)
	if err := db.Backup(cfg, autoBackupPath); err != nil {
		slog.Error("safety backup before restore failed; aborting restore", "err", err)
		return 1
	}
	if info, err := os.Stat(autoBackupPath); err == nil {
		fmt.Printf("Created safety backup of current database: %s (%s)\n", autoBackupPath, formatBytes(info.Size()))
	}

	slog.Info("restoring database from backup", "driver", cfg.DBDriver, "in", restoreSource)
	if err := db.Restore(cfg, restoreSource); err != nil {
		slog.Error("restore failed", "err", err)
		return 1
	}

	if *fromS3 != "" {
		fmt.Printf("Restored from s3://%s/%s.\n", cfg.BackupS3Bucket, remoteRestoreKey)
	} else {
		fmt.Printf("Restored from '%s'.\n", *inPath)
	}
	if cfg.DBDriver == "sqlite" {
		fmt.Println("Note: if the Octarq server is running, restart it so the restored database is reloaded.")
	}
	return 0
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
