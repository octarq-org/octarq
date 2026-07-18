package cleanup

import (
	"context"
	"testing"
	"time"

	dns "github.com/octarq-org/octarq/plugins/dns"
	links "github.com/octarq-org/octarq/plugins/links"
	mailmodels "github.com/octarq-org/octarq/plugins/mail"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mailmodels.Mailbox{}, &mailmodels.Email{}, &mailmodels.SMTPSender{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCleanup(t *testing.T) {
	db := testDB(t)

	now := time.Now()
	events := []links.LinkEvent{
		{
			CreatedAt: now.AddDate(0, 0, -10),
			LinkID:    1,
			IP:        "1.1.1.1",
		},
		{
			CreatedAt: now.AddDate(0, 0, -5),
			LinkID:    1,
			IP:        "2.2.2.2",
		},
		{
			CreatedAt: now.AddDate(0, 0, -1),
			LinkID:    1,
			IP:        "3.3.3.3",
		},
	}
	for i := range events {
		if err := db.Create(&events[i]).Error; err != nil {
			t.Fatalf("create event: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context immediately so Start's loop returns right away
	cancel()

	// 1. Check with negative/zero retention days - should not purge anything
	retentionDays := func() int { return 0 }
	Start(ctx, db, retentionDays)

	var count int64
	db.Model(&links.LinkEvent{}).Count(&count)
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}

	// 2. Check with retention days = 3 (events older than 3 days should be deleted, i.e., the one created -10 and -5 days ago)
	retentionDays = func() int { return 3 }
	Start(ctx, db, retentionDays)

	db.Model(&links.LinkEvent{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}

	var remaining []links.LinkEvent
	db.Find(&remaining)
	if len(remaining) != 1 || remaining[0].IP != "3.3.3.3" {
		t.Errorf("unexpected remaining event(s): %+v", remaining)
	}
}
