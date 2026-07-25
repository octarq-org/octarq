package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/db"
)

func runBackupCommand(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	outPath := fs.String("out", "", "Output file path for backup (optional)")
	fs.StringVar(outPath, "o", "", "Output file path for backup (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "err", err)
		return 1
	}

	finalOut := *outPath
	if finalOut == "" {
		finalOut = db.DefaultBackupFilename(cfg.DBDriver, time.Now())
	}

	slog.Info("starting database backup", "driver", cfg.DBDriver, "out", finalOut)
	if err := db.Backup(cfg, finalOut); err != nil {
		slog.Error("backup failed", "err", err)
		return 1
	}

	info, err := os.Stat(finalOut)
	var sizeStr string
	if err == nil {
		sizeStr = formatBytes(info.Size())
	}

	fmt.Printf("Backup completed successfully: %s (%s)\n", finalOut, sizeStr)
	return 0
}

func runRestoreCommand(args []string) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	inPath := fs.String("in", "", "Input backup file path to restore from (required)")
	fs.StringVar(inPath, "i", "", "Input backup file path (shorthand)")
	confirm := fs.Bool("yes", false, "Confirm restore operation without prompt")
	fs.BoolVar(confirm, "confirm", false, "Confirm restore operation without prompt")
	fs.BoolVar(confirm, "y", false, "Confirm restore operation without prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *inPath == "" {
		fmt.Println("Error: --in <path> is required for restore")
		fs.Usage()
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "err", err)
		return 1
	}

	// 1. Confirmation check
	if !*confirm {
		fmt.Printf("WARNING: Restoring database from '%s' will overwrite existing data.\n", *inPath)
		fmt.Printf("An automatic safety backup of the current database will be created first.\n")
		fmt.Print("Type 'yes' to confirm and proceed: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(input)) != "yes" {
			fmt.Println("Restore cancelled by user.")
			return 1
		}
	}

	// 2. Pre-restore Safety Net: Auto-backup current database first
	autoBackupPath := fmt.Sprintf("octarq-backup-before-restore-%s", db.DefaultBackupFilename(cfg.DBDriver, time.Now()))
	slog.Info("creating safety backup of current database before restore", "out", autoBackupPath)
	if err := db.Backup(cfg, autoBackupPath); err != nil {
		slog.Error("safety backup before restore failed; aborting restore", "err", err)
		return 1
	}
	if info, err := os.Stat(autoBackupPath); err == nil {
		fmt.Printf("Created safety backup of current database: %s (%s)\n", autoBackupPath, formatBytes(info.Size()))
	}

	// 3. Execute Restore
	slog.Info("restoring database from backup", "driver", cfg.DBDriver, "in", *inPath)
	if err := db.Restore(cfg, *inPath); err != nil {
		slog.Error("restore failed", "err", err)
		return 1
	}

	fmt.Printf("Restore completed successfully from '%s'.\n", *inPath)
	if cfg.DBDriver == "sqlite" {
		fmt.Println("Note: If the Octarq server process is currently running, please restart it to ensure the restored database is reloaded.")
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
