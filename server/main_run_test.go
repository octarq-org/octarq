package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

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

func TestRunDispatchExitCodes(t *testing.T) {
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

func TestRunBackupConfigLoadFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	if code := runBackupCommand([]string{"--out", filepath.Join(t.TempDir(), "x.db")}); code != 1 {
		t.Fatalf("runBackupCommand with bad driver = %d, want 1", code)
	}
}

func TestRunBackupDBFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "src.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "backup-failure-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "backup-failure-pw")

	if f, err := os.Create(dbPath); err != nil {
		t.Fatalf("create source db: %v", err)
	} else {
		f.Close()
	}
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

func TestRunMCPDispatchFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"mcp"}, &out, &errb); code != 1 {
		t.Fatalf("run mcp with bad driver = %d, want 1", code)
	}
}

type refusingMCPPlugin struct {
	refuse     error
	registered bool
}

func (p *refusingMCPPlugin) Name() string                      { return "refusing-mcp" }
func (p *refusingMCPPlugin) Models() []any                     { return nil }
func (p *refusingMCPPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p *refusingMCPPlugin) MCPTier() string                   { return "" }
func (p *refusingMCPPlugin) RegisterMCP(*mcp.Server)           { p.registered = true }

func (p *refusingMCPPlugin) Validate(context.Context, plugin.Host) error { return p.refuse }

type envProbeMCPPlugin struct {
	validated bool
	key       string
}

func (p *envProbeMCPPlugin) Name() string                      { return "env-probe" }
func (p *envProbeMCPPlugin) Models() []any                     { return nil }
func (p *envProbeMCPPlugin) Mount(plugin.Mux, *plugin.Context) {}
func (p *envProbeMCPPlugin) MCPTier() string                   { return "" }
func (p *envProbeMCPPlugin) RegisterMCP(*mcp.Server)           {}

func (p *envProbeMCPPlugin) Validate(context.Context, plugin.Host) error {
	p.validated = true
	p.key = os.Getenv("OCTARQ_TEST_VAULT_KEY")
	return nil
}

func TestRunMCPServerValidatesWithDotEnvApplied(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OCTARQ_TEST_VAULT_KEY=from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Chdir(dir)
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "mcp-env.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "app-boot-secret-key-32-bytes-long!!!")

	p := &envProbeMCPPlugin{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = runMCPServerWith(ctx, []plugin.Plugin{p})

	if !p.validated {
		t.Fatal("runMCPServerWith did not validate the plugin")
	}
	if p.key != "from-dotenv" {
		t.Fatalf("validator saw OCTARQ_TEST_VAULT_KEY=%q, want the .env value", p.key)
	}
}

func TestRunMCPServerStopsOnAStartupRefusal(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "mcp-refuse.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "app-boot-secret-key-32-bytes-long!!!")

	p := &refusingMCPPlugin{refuse: errors.New("refusing-mcp: no vault")}
	if code := runMCPServerWith(context.Background(), []plugin.Plugin{p}); code != 1 {
		t.Fatalf("runMCPServerWith = %d, want 1", code)
	}
	if p.registered {
		t.Fatal("tools were registered after the plugin refused to start")
	}
}

func TestRunDefaultFailureAfterOpen(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "run_fail.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "short")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "run-fail-admin-pass")

	seed, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	type seedDomain struct {
		ID      uint `gorm:"primaryKey"`
		Name    string
		ForLink bool
	}
	if err := seed.Table("domains").AutoMigrate(&seedDomain{}); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	seed.Table("domains").Create(&seedDomain{Name: "example.com", ForLink: true})
	seedSQL, _ := seed.DB()
	seedSQL.Close()

	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, &out, &errb); code != 1 {
		t.Fatalf("run default with registered domain + short secret = %d, want 1", code)
	}
}

func TestRunDefaultNewFailure(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "invalid-driver")
	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, &out, &errb); code != 1 {
		t.Fatalf("run default with bad driver = %d, want 1", code)
	}
}

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

func TestRunPluginNewFlagOrderings(t *testing.T) {
	dirA := filepath.Join(t.TempDir(), "first")
	if code := runPluginNew([]string{"--dir", dirA, "plugin-a"}); code != 0 {
		t.Fatalf("name-after-flags = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dirA, "go.mod")); err != nil {
		t.Fatalf("name-after-flags did not scaffold a module: %v", err)
	}

	if code := runPluginNew([]string{"plugin-b", "extra"}); code != 2 {
		t.Errorf("surplus positional = %d, want 2", code)
	}

	if code := runPluginNew([]string{"--nope"}); code != 2 {
		t.Errorf("unknown flag = %d, want 2", code)
	}

	if code := runPluginNew([]string{"plugin-c", "--nope"}); code != 2 {
		t.Errorf("name-first parse error = %d, want 2", code)
	}

	if code := runPluginNew([]string{"--dir", t.TempDir()}); code != 2 {
		t.Errorf("no-name-with-flags = %d, want 2", code)
	}

	if code := runPluginNew([]string{"Bad Name!"}); code != 1 {
		t.Errorf("invalid name = %d, want 1", code)
	}
}

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

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, os.ErrInvalid }

func TestRunOpenAPIFailure(t *testing.T) {
	var errb bytes.Buffer
	if code := run(context.Background(), []string{"openapi"}, errWriter{}, &errb); code != 1 {
		t.Fatalf("run openapi with failing writer = %d, want 1", code)
	}
}

func TestRunRestoreRestoreFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "restore_fail.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "restore-failure-secret-1234567890")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "restore-failure-pw")

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

func removeSafetyBackups(t *testing.T) {
	t.Helper()
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "octarq-backup-before-restore-") {
			_ = os.Remove(e.Name())
		}
	}
}

func TestRunRestoreCancellableCoversInteractivePath(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "restore_interactive.db")
	backupOut := filepath.Join(tempDir, "interactive_backup.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "restore-interactive-secret-123456")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "restore-interactive-pw")

	if code := runRestoreCommandWithInput([]string{"--in", "whatever.db"}, strings.NewReader("nope\n")); code != 1 {
		t.Fatalf("declined confirm = %d, want 1", code)
	}

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

	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "octarq-backup-before-restore-") {
			_ = os.Remove(e.Name())
		}
	}
}
