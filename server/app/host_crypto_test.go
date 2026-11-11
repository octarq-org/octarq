package app

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

// hostCryptoTestStore backs the adapter test with the production settings store.
func hostCryptoTestStore(t *testing.T) settingsStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Setting{}, &models.NotificationChannel{}, &models.Webhook{}, &models.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return settingsStore{db: db}
}

func testHostCryptoVault(t *testing.T) (*hostCryptoVault, settingsStore) {
	t.Helper()
	store := hostCryptoTestStore(t)
	cipher := crypto.New("test-secret")
	if err := cipher.EnableEnvelope(store); err != nil {
		t.Fatalf("EnableEnvelope: %v", err)
	}
	return &hostCryptoVault{cipher: cipher}, store
}

func TestHostCryptoVaultAAD(t *testing.T) {
	vault, _ := testHostCryptoVault(t)

	ct, err := vault.EncryptWithAAD([]byte("token"), []byte("org:1"))
	if err != nil {
		t.Fatalf("EncryptWithAAD: %v", err)
	}
	got, err := vault.DecryptWithAAD(ct, []byte("org:1"))
	if err != nil || string(got) != "token" {
		t.Fatalf("DecryptWithAAD = %q, %v; want %q", got, err, "token")
	}
	if _, err := vault.DecryptWithAAD(ct, []byte("org:2")); err == nil {
		t.Fatal("DecryptWithAAD opened a ciphertext sealed with another AAD")
	}
}

func TestHostCryptoVaultSubjectNamespace(t *testing.T) {
	vault, store := testHostCryptoVault(t)

	subject, err := vault.SubjectVault(42, "inbox")
	if err != nil {
		t.Fatalf("SubjectVault: %v", err)
	}
	ct, err := subject.Encrypt([]byte("message"))
	if err != nil {
		t.Fatalf("subject Encrypt: %v", err)
	}
	if _, ok := store.Get(crypto.SubjectDEKSettingKey(42, "inbox")); !ok {
		t.Fatal("the subject DEK was not persisted")
	}

	other, err := subject.SubjectVault(42, "outbox")
	if err != nil {
		t.Fatalf("subject SubjectVault: %v", err)
	}
	otherCT, err := other.Encrypt([]byte("reply"))
	if err != nil {
		t.Fatalf("other Encrypt: %v", err)
	}
	if _, err := subject.Decrypt(otherCT); err == nil {
		t.Fatal("one subject opened another subject's ciphertext")
	}

	aadCT, err := subject.EncryptWithAAD([]byte("message"), []byte("field:body"))
	if err != nil {
		t.Fatalf("subject EncryptWithAAD: %v", err)
	}
	if got, err := subject.DecryptWithAAD(aadCT, []byte("field:body")); err != nil || string(got) != "message" {
		t.Fatalf("subject DecryptWithAAD = %q, %v; want %q", got, err, "message")
	}

	if err := vault.ShredSubjectKey(42, "inbox"); err != nil {
		t.Fatalf("ShredSubjectKey: %v", err)
	}
	if _, ok := store.Get(crypto.SubjectDEKSettingKey(42, "inbox")); ok {
		t.Fatal("the subject DEK row survived the shred")
	}
	revived, err := vault.SubjectVault(42, "inbox")
	if err != nil {
		t.Fatalf("SubjectVault after shred: %v", err)
	}
	if _, err := revived.Decrypt(ct); err == nil {
		t.Fatal("a shredded subject decrypted its old data")
	}

	instanceCT, err := vault.Encrypt([]byte("instance"))
	if err != nil {
		t.Fatalf("instance Encrypt: %v", err)
	}
	if got, err := vault.Decrypt(instanceCT); err != nil || string(got) != "instance" {
		t.Fatalf("instance DEK = %q, %v; want %q", got, err, "instance")
	}
	if got, err := other.Decrypt(otherCT); err != nil || string(got) != "reply" {
		t.Fatalf("other subject = %q, %v; want %q", got, err, "reply")
	}

	// A subject adapter can shred a namespace too.
	if err := subject.ShredSubjectKey(42, "outbox"); err != nil {
		t.Fatalf("subject ShredSubjectKey: %v", err)
	}
	revivedOther, err := vault.SubjectVault(42, "outbox")
	if err != nil {
		t.Fatalf("SubjectVault outbox after shred: %v", err)
	}
	if _, err := revivedOther.Decrypt(otherCT); err == nil {
		t.Fatal("a shredded subject decrypted its old data")
	}
}

func TestHostCryptoVaultWithoutCipher(t *testing.T) {
	vault := &hostCryptoVault{}
	if _, err := vault.Encrypt([]byte("x")); err == nil {
		t.Fatal("Encrypt worked without a cipher")
	}
	if _, err := vault.Decrypt("x"); err == nil {
		t.Fatal("Decrypt worked without a cipher")
	}
	if _, err := vault.EncryptWithAAD([]byte("x"), nil); err == nil {
		t.Fatal("EncryptWithAAD worked without a cipher")
	}
	if _, err := vault.DecryptWithAAD("x", nil); err == nil {
		t.Fatal("DecryptWithAAD worked without a cipher")
	}
	if _, err := vault.SubjectVault(1, "s"); err == nil {
		t.Fatal("SubjectVault worked without a cipher")
	}
	if err := vault.ShredSubjectKey(1, "s"); err == nil {
		t.Fatal("ShredSubjectKey worked without a cipher")
	}
}
