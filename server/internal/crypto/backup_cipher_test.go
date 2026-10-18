package crypto_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/internal/crypto"
)

func TestBackupEncryptDecryptRoundTrip(t *testing.T) {
	secretKey := "super-secure-octarq-backup-key-32-chars!"
	originalData := []byte("SQLite format 3\x00 test database payload with random data: 1234567890abcdef")

	encrypted, err := crypto.EncryptBackupBytes(originalData, secretKey)
	if err != nil {
		t.Fatalf("EncryptBackupBytes failed: %v", err)
	}

	// Ciphertext must start with 12-byte nonce, and total length must be len(original) + 12 (nonce) + 16 (tag)
	expectedLen := 12 + len(originalData) + 16
	if len(encrypted) != expectedLen {
		t.Errorf("expected encrypted length %d, got %d", expectedLen, len(encrypted))
	}

	// Encrypted data must not equal plaintext
	if bytes.Equal(encrypted, originalData) {
		t.Fatal("encrypted data is identical to plaintext!")
	}

	decrypted, err := crypto.DecryptBackupBytes(encrypted, secretKey)
	if err != nil {
		t.Fatalf("DecryptBackupBytes failed: %v", err)
	}

	if !bytes.Equal(decrypted, originalData) {
		t.Errorf("decrypted data does not match original; got %q, want %q", decrypted, originalData)
	}
}

func TestBackupDecryptWrongKeyFails(t *testing.T) {
	key1 := "secret-key-number-one-1234567890"
	key2 := "secret-key-number-two-0987654321"

	data := []byte("sensitive database backup contents")
	encrypted, err := crypto.EncryptBackupBytes(data, key1)
	if err != nil {
		t.Fatalf("EncryptBackupBytes failed: %v", err)
	}

	_, err = crypto.DecryptBackupBytes(encrypted, key2)
	if err == nil {
		t.Fatal("expected decryption with wrong key to fail, but it succeeded!")
	}
}

func TestBackupDecryptTamperedCiphertextFails(t *testing.T) {
	secretKey := "secure-key-12345678901234567890"
	data := []byte("database content to verify integrity")

	encrypted, err := crypto.EncryptBackupBytes(data, secretKey)
	if err != nil {
		t.Fatalf("EncryptBackupBytes failed: %v", err)
	}

	// Tamper with one byte in the ciphertext portion
	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[len(tampered)-5] ^= 0xFF

	_, err = crypto.DecryptBackupBytes(tampered, secretKey)
	if err == nil {
		t.Fatal("expected decryption of tampered ciphertext to fail, but it succeeded!")
	}

	// Tamper with one byte in the nonce portion
	tamperedNonce := make([]byte, len(encrypted))
	copy(tamperedNonce, encrypted)
	tamperedNonce[0] ^= 0xFF

	_, err = crypto.DecryptBackupBytes(tamperedNonce, secretKey)
	if err == nil {
		t.Fatal("expected decryption with tampered nonce to fail, but it succeeded!")
	}
}

func TestBackupDecryptTruncatedCiphertextFails(t *testing.T) {
	secretKey := "secure-key-12345678901234567890"

	// Data shorter than 12 (nonce) + 16 (tag) = 28 bytes
	shortData := []byte("too short")
	_, err := crypto.DecryptBackupBytes(shortData, secretKey)
	if err == nil {
		t.Fatal("expected decryption of truncated data to fail, but it succeeded!")
	}
}

func TestBackupEmptySecretKeyFails(t *testing.T) {
	data := []byte("some backup payload")

	if _, err := crypto.EncryptBackupBytes(data, ""); err == nil {
		t.Fatal("expected encrypt with empty key to fail")
	}

	if _, err := crypto.DecryptBackupBytes(data, ""); err == nil {
		t.Fatal("expected decrypt with empty key to fail")
	}
}

func TestBackupFileOperations(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "source.db")
	encFile := filepath.Join(tmpDir, "nested", "backup.db.enc")
	decFile := filepath.Join(tmpDir, "restored.db")

	secretKey := "my-database-master-key-32-chars!"
	payload := []byte("SQLite format 3\x00 table definitions and row contents")

	if err := os.WriteFile(srcFile, payload, 0644); err != nil {
		t.Fatalf("write srcFile failed: %v", err)
	}

	// Test EncryptBackup
	if err := crypto.EncryptBackup(srcFile, encFile, secretKey); err != nil {
		t.Fatalf("EncryptBackup failed: %v", err)
	}

	encData, err := os.ReadFile(encFile)
	if err != nil {
		t.Fatalf("read encFile failed: %v", err)
	}
	if bytes.Equal(encData, payload) {
		t.Fatal("encrypted file matches plaintext")
	}

	// Test DecryptBackup
	if err := crypto.DecryptBackup(encFile, decFile, secretKey); err != nil {
		t.Fatalf("DecryptBackup failed: %v", err)
	}

	decData, err := os.ReadFile(decFile)
	if err != nil {
		t.Fatalf("read decFile failed: %v", err)
	}
	if !bytes.Equal(decData, payload) {
		t.Fatalf("decrypted file content does not match original")
	}

	// Test DecryptBackup with wrong key fails
	wrongDecFile := filepath.Join(tmpDir, "wrong.db")
	if err := crypto.DecryptBackup(encFile, wrongDecFile, "wrong-key-1234567890"); err == nil {
		t.Fatal("expected DecryptBackup with wrong key to fail")
	}

	// Test reading non-existent file
	if err := crypto.EncryptBackup(filepath.Join(tmpDir, "nonexistent.db"), encFile, secretKey); err == nil {
		t.Fatal("expected EncryptBackup on nonexistent file to fail")
	}
}
