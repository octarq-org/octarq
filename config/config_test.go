package config

import (
	"os"
	"testing"
)

func TestLoadRequiresSecretKey(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when LED_SECRET_KEY is unset")
	}
}

func TestLoadRequiresAdminPassword(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when LED_ADMIN_PASSWORD is unset")
	}
}

func TestLoadRejectsBadDriver(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	t.Setenv("LED_DB_DRIVER", "mysql")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	t.Setenv("LED_DB_DRIVER", "sqlite")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("Listen default = %q want :8080", cfg.Listen)
	}
	if cfg.AdminUser != "admin" {
		t.Errorf("AdminUser default = %q want admin", cfg.AdminUser)
	}
	if cfg.DBDSN != "led.db" {
		t.Errorf("DBDSN default = %q want led.db", cfg.DBDSN)
	}
}

func TestLoadDotEnv(t *testing.T) {
	content := `
# A comment line
export KEY1=value1
KEY2="value2"
KEY3='value3'
KEY4=value4 # another comment
`
	tmpfile, err := os.CreateTemp("", "dotenv")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	tmpfile.Close()

	os.Unsetenv("KEY1")
	os.Unsetenv("KEY2")
	os.Unsetenv("KEY3")
	os.Unsetenv("KEY4")

	if err := loadDotEnv(tmpfile.Name()); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	if os.Getenv("KEY1") != "value1" {
		t.Errorf("KEY1 = %q, want value1", os.Getenv("KEY1"))
	}
	if os.Getenv("KEY2") != "value2" {
		t.Errorf("KEY2 = %q, want value2", os.Getenv("KEY2"))
	}
	if os.Getenv("KEY3") != "value3" {
		t.Errorf("KEY3 = %q, want value3", os.Getenv("KEY3"))
	}
	if os.Getenv("KEY4") != "value4" {
		t.Errorf("KEY4 = %q, want value4", os.Getenv("KEY4"))
	}

	if err := loadDotEnv("nonexistent_dotenv_file"); err != nil {
		t.Errorf("loadDotEnv on missing file returned error: %v", err)
	}
}
