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

// autoSecretFile and autoAdminPassFile are persisted alongside the SQLite
// database so a zero-config `docker run` (no .env, no env vars) can boot: the
// KEK/session key and the initial admin password are generated once on first
// boot and reused on every restart. Both are the KEK-must-be-stable kind of
// value — a fresh random each boot would make previously encrypted provider
// credentials unrecoverable (secret) and lock the operator out (password), so
// they are written to disk, not held in memory.
const (
	autoSecretFile    = "octarq.secret"
	autoAdminPassFile = "octarq-admin-password.txt"
)

// stateDir returns the directory used to persist auto-generated secrets. For
// the default SQLite deployment it is the directory holding the database file
// (the mounted /data volume in the Docker image), so the secrets share the
// database's lifecycle and backups. For any other driver it falls back to the
// current working directory.
func (c *Config) stateDir() string {
	if c.DBDriver != "sqlite" {
		return "."
	}
	// The SQLite DSN may carry a "file:" prefix and "?_pragma=..." query; strip
	// both to recover the on-disk path before taking its directory.
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

// loadOrCreate reads name from dir, or generates a value with gen, writes it
// 0600, and returns it. created reports whether the value was freshly minted.
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

// randHex returns n random bytes hex-encoded (2n characters).
func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randPassword returns a URL-safe-ish random password of roughly n characters,
// drawn from an unambiguous alphabet (no 0/O/1/l/I) so it can be read off a log
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

// ensureAutoSecrets fills in a missing SecretKey and/or AdminPassword by
// generating and persisting them, so the binary boots with no configuration at
// all. Values supplied via the environment always win and are never written to
// disk. It must run after DBDriver/DBDSN are resolved.
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
			log.Printf("octarq: generated an initial admin login — user %q, password %q (saved to %s)", c.AdminUser, pass, filepath.Join(dir, autoAdminPassFile))
		}
	}

	return nil
}
