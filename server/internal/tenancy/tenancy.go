package tenancy

import (
	"errors"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"gorm.io/gorm"
)

var ErrNameTaken = errors.New("tenant subdomain already taken")

const hostSource = "core:tenancy"

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
	if base == "" {
		return "", false, nil
	}
	name = strings.ToLower(slug) + "." + base
	var conflicts int64
	if err := db.Model(&models.OrgHost{}).Where("host = ? AND owner_id <> ?", name, orgID).Count(&conflicts).Error; err != nil {
		return "", false, err
	}
	if conflicts > 0 {
		return "", false, ErrNameTaken
	}
	row := models.OrgHost{
		OrgID:   orgID,
		Host:    name,
		Traffic: true,
		Source:  hostSource,
	}
	if err := db.Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return "", false, ErrNameTaken
		}
		return "", false, err
	}
	origin.ClearHostCache(name)
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
	if base == "" {
		return nil
	}
	name := strings.ToLower(slug) + "." + base
	res := db.Where("host = ? AND owner_id = ? AND source = ?", name, orgID, hostSource).Delete(&models.OrgHost{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		origin.ClearHostCache(name)
	}
	return nil
}
