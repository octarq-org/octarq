package db

import (
	"fmt"

	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func BenchmarkBackfillTenantSubdomainLinkHosts(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}

	db.AutoMigrate(&models.Org{}, &tenantDomainRow{}, &models.Setting{})
	db.Create(&models.Setting{Key: "base_domain", Value: "example.com"})

	for i := 0; i < 1000; i++ {
		org := models.Org{
			Slug: fmt.Sprintf("org%d", i),
		}
		db.Create(&org)

		dom := tenantDomainRow{
			OrgID:   org.ID,
			Name:    fmt.Sprintf("org%d.example.com", i),
			ForLink: false,
		}
		db.Create(&dom)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db.Model(&tenantDomainRow{}).Where("for_link = ?", true).Update("for_link", false)
		db.Model(&tenantDomainRow{}).Where("1=1").Update("link_hosts", "[]")
		b.StartTimer()
		backfillTenantSubdomainLinkHosts(db)
	}
}
