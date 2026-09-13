package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

// TestRunVersionAndOpenAPI covers the two read-only stdout subcommands: build
// metadata and the OpenAPI document. Both must exit 0 and write to stdout.
func TestRunVersionAndOpenAPI(t *testing.T) {
	var out, errb bytes.Buffer

	for _, flag := range []string{"--version", "-version"} {
		out.Reset()
		errb.Reset()
		if code := run(context.Background(), []string{flag}, &out, &errb); code != 0 {
			t.Fatalf("run %s = %d, want 0", flag, code)
		}
		if !strings.HasPrefix(out.String(), "octarq ") {
			t.Errorf("run %s output = %q, want prefix \"octarq \"", flag, out.String())
		}
		if errb.Len() != 0 {
			t.Errorf("run %s wrote to stderr: %q", flag, errb.String())
		}
	}

	out.Reset()
	errb.Reset()
	if code := run(context.Background(), []string{"openapi"}, &out, &errb); code != 0 {
		t.Fatalf("run openapi = %d, want 0: %s", code, errb.String())
	}
	doc := out.String()
	if !strings.Contains(doc, `"openapi": "3.0.3"`) {
		t.Error("openapi document missing the version key")
	}
	if !strings.Contains(doc, "/api/auth/login") || !strings.Contains(doc, "/api/health") {
		t.Error("openapi document missing expected paths")
	}
}

// TestRunDispatchExitCodes exercises the subcommand dispatch lines that route
// to the exit-code-returning command bodies: bad flags and usage errors must
// propagate their codes out of run.
func TestRunDispatchExitCodes(t *testing.T) {
	// Point config.Load at a temp sqlite DB so the restore dispatch case never
	// auto-generates secret files next to the working directory.
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "dispatch.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "dispatch-test-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "dispatch-test-admin-pass")
	var out, errb bytes.Buffer

	if code := run(context.Background(), []string{"backup", "--nope"}, &out, &errb); code != 1 {
		t.Errorf("run backup --nope = %d, want 1", code)
	}

	if code := run(context.Background(), []string{"restore"}, &out, &errb); code != 1 {
		t.Errorf("run restore (no --in) = %d, want 1", code)
	}
	if code := run(context.Background(), []string{"restore", "--in", "x", "--nope"}, &out, &errb); code != 1 {
		t.Errorf("run restore bad flag = %d, want 1", code)
	}

	if code := run(context.Background(), []string{"plugin"}, &out, &errb); code != 2 {
		t.Errorf("run plugin (no subcommand) = %d, want 2", code)
	}
	if code := run(context.Background(), []string{"plugin", "unknown"}, &out, &errb); code != 2 {
		t.Errorf("run plugin unknown = %d, want 2", code)
	}
}

// TestRunBackupConfigLoadFailure covers the config-load error branch of the
// backup command: an invalid driver aborts before touching the database.
func TestRunBackupConfigLoadFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	if code := runBackupCommand([]string{"--out", filepath.Join(t.TempDir(), "x.db")}); code != 1 {
		t.Fatalf("runBackupCommand with bad driver = %d, want 1", code)
	}
}

// TestRunBackupDBFailure covers the db.Backup error branch: writing the copy
// into a non-existent directory fails and must produce exit code 1.
func TestRunBackupDBFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "src.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "backup-failure-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "backup-failure-pw")

	// Touch a valid source database so backup has something to copy.
	if f, err := os.Create(dbPath); err != nil {
		t.Fatalf("create source db: %v", err)
	} else {
		f.Close()
	}
	// A regular file in the way of the target directory defeats the mkdir-all
	// the backup does, so db.Backup must fail.
	blocker := filepath.Join(tempDir, "blocker")
	if f, err := os.Create(blocker); err != nil {
		t.Fatalf("create blocker: %v", err)
	} else {
		f.Close()
	}
	badOut := filepath.Join(blocker, "out.db")
	if code := runBackupCommand([]string{"--out", badOut}); code != 1 {
		t.Fatalf("runBackupCommand to unwritable path = %d, want 1", code)
	}
}

// TestRunMCPDispatchFailure covers the mcp dispatch branch without starting a
// stdio server: config.Load rejects the driver before any transport is opened.
func TestRunMCPDispatchFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"mcp"}, &out, &errb); code != 1 {
		t.Fatalf("run mcp with bad driver = %d, want 1", code)
	}
}

// TestRunDefaultFailureAfterOpen covers the default branch's "run failed" path
// (a.Run returning an error) once app.New has succeeded: a pre-seeded registered
// domain plus a too-short secret key trips enforceSecretKeyFloor inside Run.
func TestRunDefaultFailureAfterOpen(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "run_fail.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "short")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "run-fail-admin-pass")

	// Seed a registered domain before run() opens the database, so the
	// secret-key floor (which only engages once a domain is registered) fires.
	seed, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if err := seed.AutoMigrate(&dns.Domain{}); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	seed.Create(&dns.Domain{Name: "example.com", ForLink: true})
	seedSQL, _ := seed.DB()
	seedSQL.Close()

	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, &out, &errb); code != 1 {
		t.Fatalf("run default with registered domain + short secret = %d, want 1", code)
	}
}

// TestRunDefaultNewFailure covers the default-branch "init failed" path: a
// rejected DB driver makes app.New return an error, and run must map it to 1.
func TestRunDefaultNewFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, &out, &errb); code != 1 {
		t.Fatalf("run default with bad driver = %d, want 1", code)
	}
}

// TestRunDefaultBootsAndShutsDown boots the full open-core composition root
// (builtin core plugins + the hello example) against a scratch SQLite database
// with an already-cancelled context, so Run starts, migrates, and then shuts
// down as soon as it observes the cancellation. It is the standing guard that
// the default plugin set actually wires up without a live server.
func TestRunDefaultBootsAndShutsDown(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "run_boot.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "run-boot-secret-key-32-bytes-long!!!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "run-boot-admin-pass")
	t.Setenv("OCTARQ_LISTEN", "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := run(ctx, nil, &out, &errb); code != 0 {
		t.Fatalf("run default = %d, want 0: %s", code, errb.String())
	}

	// Run() migrates the schema exactly once on boot; prove it happened by
	// opening the scratch database and counting real tables.
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("reopen boot db: %v", err)
	}
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	var tables int
	if err := gdb.Raw(`SELECT count(*) FROM sqlite_master WHERE type='table'`).Scan(&tables).Error; err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if tables == 0 {
		t.Error("boot migrated no tables; Run() never ran its migration pass")
	}
}

// TestRunPluginNewFlagOrderings covers the two accepted `<name>` orderings and
// the error exits of runPluginNew: usage errors (2) on bad flags or surplus
// positionals, and a scaffolding failure (1) on an invalid plugin name.
func TestRunPluginNewFlagOrderings(t *testing.T) {
	// name after flags (resolved from fs.Arg(0)).
	dirA := filepath.Join(t.TempDir(), "first")
	if code := runPluginNew([]string{"--dir", dirA, "plugin-a"}); code != 0 {
		t.Fatalf("name-after-flags = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dirA, "go.mod")); err != nil {
		t.Fatalf("name-after-flags did not scaffold a module: %v", err)
	}

	// surplus positional after the name is a usage error.
	if code := runPluginNew([]string{"plugin-b", "extra"}); code != 2 {
		t.Errorf("surplus positional = %d, want 2", code)
	}

	// unknown flag is a parse error -> 2.
	if code := runPluginNew([]string{"--nope"}); code != 2 {
		t.Errorf("unknown flag = %d, want 2", code)
	}

	// the parse-error path when the name comes first.
	if code := runPluginNew([]string{"plugin-c", "--nope"}); code != 2 {
		t.Errorf("name-first parse error = %d, want 2", code)
	}

	// flags present but the name missing -> usage error 2.
	if code := runPluginNew([]string{"--dir", t.TempDir()}); code != 2 {
		t.Errorf("no-name-with-flags = %d, want 2", code)
	}

	// a name the scaffold rejects returns 1 (user-facing error, not usage).
	if code := runPluginNew([]string{"Bad Name!"}); code != 1 {
		t.Errorf("invalid name = %d, want 1", code)
	}
}

// TestFormatBytes pins the human-readable byte rendering across every unit
// step, including the <1024 passthrough and the KB/MB/GB suffixes.
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.in); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// errWriter fails every Write, turning the openapi serializer's bare write
// error into a command failure.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, os.ErrInvalid }

// TestRunOpenAPIFailure covers the openapi branch's error exit: when the
// output writer is broken, Generate returns an error and run must map it to 1.
func TestRunOpenAPIFailure(t *testing.T) {
	var errb bytes.Buffer
	if code := run(context.Background(), []string{"openapi"}, errWriter{}, &errb); code != 1 {
		t.Fatalf("run openapi with failing writer = %d, want 1", code)
	}
}

// TestRunRestoreRestoreFailure covers the db.Restore error branch: a junk
// backup file fails to restore (after the safety backup succeeded), returning 1.
func TestRunRestoreRestoreFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "restore_fail.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "restore-failure-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "restore-failure-pw")

	// A valid current database so the safety backup succeeds before the
	// (broken) restore input is read.
	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	type Dummy struct {
		ID uint `gorm:"primaryKey"`
	}
	gdb.AutoMigrate(&Dummy{})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	junk := filepath.Join(tempDir, "junk.backup")
	if err := os.WriteFile(junk, []byte("this is not a sqlite database"), 0o600); err != nil {
		t.Fatalf("write junk backup: %v", err)
	}

	if code := runRestoreCommandWithInput([]string{"--in", junk, "--yes"}, strings.NewReader("yes\n")); code != 1 {
		t.Fatalf("restore from junk input = %d, want 1", code)
	}

	removeSafetyBackups(t)
}

// TestRunRestoreSafetyBackupFailure covers the pre-restore safety-backup error:
// an unreadable current database makes the safety copy fail before any restore.
func TestRunRestoreSafetyBackupFailure(t *testing.T) {
	tempDir := t.TempDir()
	junkDB := filepath.Join(tempDir, "current_not_a_db")
	if err := os.WriteFile(junkDB, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("write junk current db: %v", err)
	}
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", junkDB)
	t.Setenv("OCTARQ_SECRET_KEY", "restore-safety-failure-secret-123456")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "restore-safety-failure-pw")

	if code := runRestoreCommandWithInput([]string{"--in", "x.db", "--yes"}, strings.NewReader("yes\n")); code != 1 {
		t.Fatalf("restore with broken current db = %d, want 1", code)
	}
}

// removeSafetyBackups deletes the pre-restore safety backup files the restore
// flow writes into the working directory.
func removeSafetyBackups(t *testing.T) {
	t.Helper()
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "octarq-backup-before-restore-") {
			_ = os.Remove(e.Name())
		}
	}
}

// TestRunRestoreCancellableCoversInteractivePath drives the interactive
// confirm branch both ways: declining aborts with 1 and typing "yes" runs the
// full restore against a scratch database (the confirm path the --yes flag
// skips).
func TestRunRestoreCancellableCoversInteractivePath(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "restore_interactive.db")
	backupOut := filepath.Join(tempDir, "interactive_backup.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "restore-interactive-secret-123456")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "restore-interactive-pw")

	// Declining the prompt must abort without touching anything.
	if code := runRestoreCommandWithInput([]string{"--in", "whatever.db"}, strings.NewReader("nope\n")); code != 1 {
		t.Fatalf("declined confirm = %d, want 1", code)
	}

	// A rejected configuration aborts before the prompt.
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	if code := runRestoreCommandWithInput([]string{"--in", "x.db", "--yes"}, strings.NewReader("yes\n")); code != 1 {
		t.Fatalf("restore with bad driver = %d, want 1", code)
	}
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")

	gdb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	type Dummy struct {
		ID    uint `gorm:"primaryKey"`
		Value string
	}
	if err := gdb.AutoMigrate(&Dummy{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gdb.Create(&Dummy{Value: "orig"})
	sqlDB, _ := gdb.DB()
	sqlDB.Close()

	if code := runBackupCommand([]string{"--out", backupOut}); code != 0 {
		t.Fatalf("backup = %d, want 0", code)
	}

	if code := runRestoreCommandWithInput([]string{"--in", backupOut}, strings.NewReader("yes\n")); code != 0 {
		t.Fatalf("interactive restore = %d, want 0", code)
	}

	gdb2, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("reopen restored db: %v", err)
	}
	defer func() {
		sqlDB2, _ := gdb2.DB()
		sqlDB2.Close()
	}()
	var d Dummy
	if err := gdb2.First(&d).Error; err != nil {
		t.Fatalf("row missing after restore: %v", err)
	}
	if d.Value != "orig" {
		t.Errorf("value = %q, want orig", d.Value)
	}

	// The pre-restore safety backup is written to the working directory; clean it.
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "octarq-backup-before-restore-") {
			_ = os.Remove(e.Name())
		}
	}
}
