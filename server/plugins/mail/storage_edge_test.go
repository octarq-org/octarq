package mail

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

func TestDBStorageProviderNilDBErrors(t *testing.T) {
	p := &DBStorageProvider{}
	ctx := context.Background()
	if err := p.Put(ctx, "k", []byte("d")); err == nil {
		t.Error("Put with nil db must error")
	}
	if _, err := p.Get(ctx, "k"); err == nil {
		t.Error("Get with nil db must error")
	}
	if err := p.Delete(ctx, "k"); err == nil {
		t.Error("Delete with nil db must error")
	}
	if _, err := p.Stat(ctx, "k"); err == nil {
		t.Error("Stat with nil db must error")
	}
}

func TestDBStorageProviderRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&MailRawBlob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&MailRawBlob{})
	p := NewDBStorageProvider(db)
	ctx := context.Background()

	if err := p.Put(ctx, "a/b.eml", []byte("raw bytes")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, err := p.Get(ctx, "a/b.eml")
	if err != nil || string(data) != "raw bytes" {
		t.Fatalf("Get: %v %q", err, data)
	}
	size, err := p.Stat(ctx, "a/b.eml")
	if err != nil || size != 9 {
		t.Fatalf("Stat: %v size=%d (want 9)", err, size)
	}

	// Missing keys report not-found.
	if _, err := p.Get(ctx, "missing.eml"); !errors.Is(err, plugin.ErrStorageNotFound) {
		t.Errorf("Get missing: %v, want ErrStorageNotFound", err)
	}
	if _, err := p.Stat(ctx, "missing.eml"); !errors.Is(err, plugin.ErrStorageNotFound) {
		t.Errorf("Stat missing: %v, want ErrStorageNotFound", err)
	}

	// Delete removes the blob.
	if err := p.Delete(ctx, "a/b.eml"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := p.Get(ctx, "a/b.eml"); !errors.Is(err, plugin.ErrStorageNotFound) {
		t.Errorf("Get after delete: %v, want ErrStorageNotFound", err)
	}
}
