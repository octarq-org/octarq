package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func EncryptBackup(srcPath, dstPath, secretKey string) error {
	if secretKey == "" {
		return errors.New("crypto: secret key is required for backup encryption")
	}

	plaintext, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("crypto: read backup file: %w", err)
	}

	encrypted, err := EncryptBackupBytes(plaintext, secretKey)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(dstPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("crypto: create directory: %w", err)
		}
	}

	tmpDst := dstPath + ".tmp"
	if err := os.WriteFile(tmpDst, encrypted, 0600); err != nil {
		return fmt.Errorf("crypto: write encrypted backup: %w", err)
	}

	if err := os.Rename(tmpDst, dstPath); err != nil {
		_ = os.Remove(tmpDst)
		return fmt.Errorf("crypto: finalize encrypted backup: %w", err)
	}

	return nil
}

func DecryptBackup(srcPath, dstPath, secretKey string) error {
	if secretKey == "" {
		return errors.New("crypto: secret key is required for backup decryption")
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("crypto: read encrypted backup: %w", err)
	}

	plaintext, err := DecryptBackupBytes(data, secretKey)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(dstPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("crypto: create directory: %w", err)
		}
	}

	tmpDst := dstPath + ".tmp"
	if err := os.WriteFile(tmpDst, plaintext, 0600); err != nil {
		return fmt.Errorf("crypto: write decrypted backup: %w", err)
	}

	if err := os.Rename(tmpDst, dstPath); err != nil {
		_ = os.Remove(tmpDst)
		return fmt.Errorf("crypto: finalize decrypted backup: %w", err)
	}

	return nil
}

func EncryptBackupBytes(plaintext []byte, secretKey string) ([]byte, error) {
	if secretKey == "" {
		return nil, errors.New("crypto: secret key is required for backup encryption")
	}

	key := sha256.Sum256([]byte(secretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func DecryptBackupBytes(data []byte, secretKey string) ([]byte, error) {
	if secretKey == "" {
		return nil, errors.New("crypto: secret key is required for backup decryption")
	}

	key := sha256.Sum256([]byte(secretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize+gcm.Overhead() {
		return nil, errors.New("crypto: backup ciphertext too short or corrupted")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: backup decryption failed: authentication failed or invalid key: %w", err)
	}

	return plaintext, nil
}
