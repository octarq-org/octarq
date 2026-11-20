package tenancy

import (
	"errors"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"gorm.io/gorm"
)

var ErrNameTaken = errors.New("tenant subdomain already taken")

type domainRow struct {
	ID        uint   `gorm:"primaryKey"`
	OrgID     uint   `gorm:"column:owner_id"`
	Name      string `gorm:"uniqueIndex;size:255"`
	ForLink   bool
	ForMail   bool
	LinkHosts models.HostList `gorm:"type:text"`
	MailHosts models.HostList `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (domainRow) TableName() string { return "domains" }

func Subdomain(db *gorm.DB, slug string) (string, bool) {
	base := models.BaseDomain(db)
	if base == "" || slug == "" {
		return "", false
	}
	return strings.ToLower(slug) + "." + base, true
}

func Provision(db *gorm.DB, orgID uint, slug string) (name string, provisioned bool, err error) {
	if db == nil || orgID == 0 || slug == "" {
		return "", false, nil
	}
	base := models.BaseDomain(db)
	if base == "" || !db.Migrator().HasTable("domains") {
		return "", false, nil
	}
	name = strings.ToLower(slug) + "." + base
	row := domainRow{
		OrgID:     orgID,
		Name:      name,
		ForLink:   true,
		LinkHosts: models.HostList{{Host: name, Enabled: true}},
	}
	if err := db.Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return "", false, ErrNameTaken
		}
		return "", false, err
	}
	origin.ClearDomainCache(name)
	return name, true, nil
}

func IsDuplicateKey(err error) bool { return isDuplicateKey(err) }

func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique constraint failed") ||
		strings.Contains(lower, "duplicate key value violates unique constraint")
}

func Retire(db *gorm.DB, orgID uint, slug string) error {
	if db == nil || orgID == 0 || slug == "" {
		return nil
	}
	base := models.BaseDomain(db)
	if base == "" || !db.Migrator().HasTable("domains") {
		return nil
	}
	name := strings.ToLower(slug) + "." + base
	res := db.Where("name = ? AND owner_id = ?", name, orgID).Delete(&domainRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		origin.ClearDomainCache(name)
	}
	return nil
}
