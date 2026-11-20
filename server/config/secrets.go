package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	autoSecretFile    = "octarq.secret"
	autoAdminPassFile = "octarq-admin-password.txt"
)

func (c *Config) stateDir() string {
	if c.DBDriver != "sqlite" {
		return "."
	}
	dsn := c.DBDSN
	dsn = strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(dsn, '?'); i >= 0 {
		dsn = dsn[:i]
	}
	dsn = strings.TrimSpace(dsn)
	if dsn == "" || dsn == ":memory:" {
		return "."
	}
	dir := filepath.Dir(dsn)
	if dir == "" {
		return "."
	}
	return dir
}

func loadOrCreate(dir, name string, gen func() (string, error)) (value string, created bool, err error) {
	path := filepath.Join(dir, name)
	if b, rerr := os.ReadFile(path); rerr == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, false, nil
		}
	} else if !os.IsNotExist(rerr) {
		return "", false, fmt.Errorf("reading %s: %w", path, rerr)
	}
	v, gerr := gen()
	if gerr != nil {
		return "", false, gerr
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("creating state dir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(v+"\n"), 0o600); err != nil {
		return "", false, fmt.Errorf("writing %s: %w", path, err)
	}
	return v, true, nil
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// line and typed by hand.
func randPassword(n int) (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func (c *Config) ensureAutoSecrets() error {
	dir := c.stateDir()

	if c.SecretKey == "" {
		key, created, err := loadOrCreate(dir, autoSecretFile, func() (string, error) { return randHex(32) })
		if err != nil {
			return fmt.Errorf("auto secret key: %w", err)
		}
		c.SecretKey = key
		if created {
			log.Printf("octarq: generated a new secret key at %s (keep this file — it encrypts stored credentials)", filepath.Join(dir, autoSecretFile))
		}
	}

	if c.AdminPassword == "" {
		pass, created, err := loadOrCreate(dir, autoAdminPassFile, func() (string, error) { return randPassword(20) })
		if err != nil {
			return fmt.Errorf("auto admin password: %w", err)
		}
		c.AdminPassword = pass
		if created {
			log.Printf("octarq: generated an initial admin login for user %q and saved the password to %s", c.AdminUser, filepath.Join(dir, autoAdminPassFile))
		}
	}

	return nil
}
