package notify

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/internal/notification"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.NotificationChannel{}); err != nil {
		t.Fatalf("migrate notification channels: %v", err)
	}
	return db
}

func TestNotify_RequiresWorkspace(t *testing.T) {
	p := &Plugin{}
	if _, err := p.notify(context.Background(), NotifyInput{Message: "hi"}); err == nil {
		t.Fatal("expected an error when the request carries no workspace")
	}
}

func TestNotify_EmptyMessageRejected(t *testing.T) {
	p := &Plugin{db: openTestDB(t)}
	if _, err := p.notify(plugin.WithOrgID(context.Background(), 1), NotifyInput{Message: "   "}); err == nil {
		t.Fatal("expected an error for a blank message")
	}
}

func TestNotify_NoChannelsFailsClosed(t *testing.T) {
	p := &Plugin{db: openTestDB(t)}
	if _, err := p.notify(plugin.WithOrgID(context.Background(), 1), NotifyInput{Message: "hi"}); err == nil {
		t.Fatal("expected a fail-closed error when the workspace has no channels")
	}
}

func TestNotify_DeliversToEnabledChannel(t *testing.T) {
	db := openTestDB(t)
	notification.SetConfigDecryptor(func(s string) (string, bool) { return s, true })

	var got string
	if err := notification.DefaultRouter().RegisterChannel(
		notification.NewLegacyNotifierAdapter("testnotify", "", func(_ context.Context, _ /*cfg*/, text string) error {
			got = text
			return nil
		}),
	); err != nil {
		t.Fatalf("register test channel: %v", err)
	}

	if err := db.Create(&models.NotificationChannel{OrgID: 1, Name: "t", Type: "testnotify", Config: "{}", Enabled: true}).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	p := &Plugin{db: db}
	out, err := p.notify(plugin.WithOrgID(context.Background(), 1), NotifyInput{Message: "hello", Title: "Agent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Delivered != 1 || len(out.Channels) != 1 || out.Channels[0] != "testnotify" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if got != "Agent — hello" {
		t.Fatalf("channel received %q, want %q", got, "Agent — hello")
	}
}

func TestNotify_DisabledChannelSkipped(t *testing.T) {
	db := openTestDB(t)
	ch := &models.NotificationChannel{OrgID: 1, Name: "t", Type: "testnotify", Config: "{}", Enabled: true}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	// The column defaults to true, so a create with false would be coerced back;
	// disable it explicitly.
	if err := db.Model(ch).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable channel: %v", err)
	}
	p := &Plugin{db: db}
	if _, err := p.notify(plugin.WithOrgID(context.Background(), 1), NotifyInput{Message: "hi"}); err == nil {
		t.Fatal("expected an error: the only channel is disabled")
	}
}
