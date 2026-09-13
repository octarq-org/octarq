package links

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	dns "github.com/octarq-org/octarq/plugins/dns"
	"gorm.io/gorm"
)

func BenchmarkEngineFlushBatch_Current(b *testing.B) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		b.Fatalf("migrate: %v", err)
	}

	db.Where("1 = 1").Delete(&Link{})
	db.Where("1 = 1").Delete(&LinkEvent{})

	// Create some links to click on
	linkIDs := []uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	prefix := rand.Intn(1000)
	for _, id := range linkIDs {
		link := &Link{
			Slug:    fmt.Sprintf("test%d_%d", prefix, id),
			Target:  "https://example.com",
			Enabled: true,
			OrgID:   1,
			Clicks:  0,
		}
		if err := db.Create(link).Error; err != nil {
			b.Fatalf("create link: %v", err)
		}
	}

	// Refresh the slice to have actual IDs
	var links []Link
	db.Find(&links)
	for i, link := range links {
		linkIDs[i] = link.ID
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		clicksByLink := map[uint]int64{
			linkIDs[0]: 1,
			linkIDs[1]: 2,
			linkIDs[2]: 3,
			linkIDs[3]: 4,
			linkIDs[4]: 5,
			linkIDs[5]: 6,
			linkIDs[6]: 7,
			linkIDs[7]: 8,
			linkIDs[8]: 9,
			linkIDs[9]: 10,
		}
		b.StartTimer()

		err := db.Transaction(func(tx *gorm.DB) error {
			for linkID, count := range clicksByLink {
				if err := tx.Model(&Link{}).Where("id = ?", linkID).
					UpdateColumn("clicks", gorm.Expr("clicks + ?", count)).Error; err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			b.Fatalf("transaction failed: %v", err)
		}
	}
}

func BenchmarkEngineFlushBatch_Optimized(b *testing.B) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &Link{}, &LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{})...); err != nil {
		b.Fatalf("migrate: %v", err)
	}

	db.Where("1 = 1").Delete(&Link{})
	db.Where("1 = 1").Delete(&LinkEvent{})

	// Create some links to click on
	linkIDs := []uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	prefix := rand.Intn(1000)
	for _, id := range linkIDs {
		link := &Link{
			Slug:    fmt.Sprintf("test%d_%d", prefix, id),
			Target:  "https://example.com",
			Enabled: true,
			OrgID:   1,
			Clicks:  0,
		}
		if err := db.Create(link).Error; err != nil {
			b.Fatalf("create link: %v", err)
		}
	}

	// Refresh the slice to have actual IDs
	var links []Link
	db.Find(&links)
	for i, link := range links {
		linkIDs[i] = link.ID
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		clicksByLink := map[uint]int64{
			linkIDs[0]: 1,
			linkIDs[1]: 2,
			linkIDs[2]: 3,
			linkIDs[3]: 4,
			linkIDs[4]: 5,
			linkIDs[5]: 6,
			linkIDs[6]: 7,
			linkIDs[7]: 8,
			linkIDs[8]: 9,
			linkIDs[9]: 10,
		}
		b.StartTimer()

		err := db.Transaction(func(tx *gorm.DB) error {
			if len(clicksByLink) > 0 {
				var queryBuilder strings.Builder
				queryBuilder.WriteString("UPDATE links SET clicks = clicks + CASE id")

				args := make([]interface{}, 0, len(clicksByLink)*2+1)
				ids := make([]uint, 0, len(clicksByLink))

				for linkID, count := range clicksByLink {
					queryBuilder.WriteString(" WHEN ? THEN ?")
					args = append(args, linkID, count)
					ids = append(ids, linkID)
				}

				queryBuilder.WriteString(" ELSE 0 END WHERE id IN ?")
				args = append(args, ids)

				if err := tx.Exec(queryBuilder.String(), args...).Error; err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			b.Fatalf("transaction failed: %v", err)
		}
	}
}
