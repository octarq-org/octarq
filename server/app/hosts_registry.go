package app

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/origin"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type hostRegistry struct {
	db *gorm.DB
}

var _ plugin.HostRegistry = (*hostRegistry)(nil)

func newHostRegistry(db *gorm.DB) plugin.HostRegistry {
	if db == nil {
		return plugin.NoopHostRegistry()
	}
	return &hostRegistry{db: db}
}

func (r *hostRegistry) OwnerOf(host string) (uint, bool) {
	if r == nil || r.db == nil {
		return 0, false
	}
	return origin.OwnerOf(r.db, host)
}

func (r *hostRegistry) ServesTraffic(host string) bool {
	if r == nil || r.db == nil {
		return false
	}
	return origin.ServesTraffic(r.db, host)
}

func (r *hostRegistry) Any() bool {
	if r == nil || r.db == nil {
		return false
	}
	return origin.AnyRegistered(r.db)
}

func (r *hostRegistry) Bind(ctx context.Context, orgID uint, host string, traffic bool, source string) error {
	if r == nil || r.db == nil {
		return plugin.ErrHostsUnavailable
	}
	host, err := normalizeBindHost(r.db, host)
	if err != nil {
		return err
	}
	if orgID == 0 {
		return plugin.ErrHostOrgRequired
	}
	if strings.TrimSpace(source) == "" {
		return plugin.ErrHostSourceRequired
	}
	db := r.db.WithContext(ctx)
	err = db.Transaction(func(tx *gorm.DB) error {
		var conflicts int64
		if err := tx.Model(&models.OrgHost{}).Where("host = ? AND owner_id <> ?", host, orgID).Count(&conflicts).Error; err != nil {
			return err
		}
		if conflicts > 0 {
			return plugin.ErrHostTaken
		}
		var existing models.OrgHost
		err := tx.Where("host = ? AND owner_id = ? AND source = ?", host, orgID, source).First(&existing).Error
		switch {
		case err == nil:
			return tx.Model(&existing).Update("traffic", traffic).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			row := models.OrgHost{OrgID: orgID, Host: host, Traffic: traffic, Source: source}
			if err := tx.Create(&row).Error; err != nil {
				if isHostUniqueViolation(err) {
					return plugin.ErrHostTaken
				}
				return err
			}
			return nil
		default:
			return err
		}
	})
	if err != nil {
		return err
	}
	origin.ClearHostCache(host)
	return nil
}

func (r *hostRegistry) Unbind(ctx context.Context, orgID uint, host, source string) error {
	if r == nil || r.db == nil {
		return plugin.ErrHostsUnavailable
	}
	host = origin.NormalizeHost(host)
	if host == "" {
		return plugin.ErrHostInvalid
	}
	if orgID == 0 {
		return plugin.ErrHostOrgRequired
	}
	if strings.TrimSpace(source) == "" {
		return plugin.ErrHostSourceRequired
	}
	res := r.db.WithContext(ctx).
		Where("host = ? AND owner_id = ? AND source = ?", host, orgID, source).
		Delete(&models.OrgHost{})
	if res.Error != nil {
		return res.Error
	}
	origin.ClearHostCache(host)
	return nil
}

func (r *hostRegistry) ReplaceOwned(ctx context.Context, orgID uint, source string, hosts []string, traffic bool) error {
	if r == nil || r.db == nil {
		return plugin.ErrHostsUnavailable
	}
	if orgID == 0 {
		return plugin.ErrHostOrgRequired
	}
	if strings.TrimSpace(source) == "" {
		return plugin.ErrHostSourceRequired
	}
	normalized := make([]string, 0, len(hosts))
	seen := make(map[string]bool, len(hosts))
	for _, raw := range hosts {
		host, err := normalizeBindHost(r.db, raw)
		if err != nil {
			return err
		}
		if seen[host] {
			continue
		}
		seen[host] = true
		normalized = append(normalized, host)
	}

	var removed []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []models.OrgHost
		if err := tx.Where("owner_id = ? AND source = ?", orgID, source).Find(&existing).Error; err != nil {
			return err
		}
		for _, row := range existing {
			removed = append(removed, row.Host)
		}
		if err := tx.Where("owner_id = ? AND source = ?", orgID, source).Delete(&models.OrgHost{}).Error; err != nil {
			return err
		}
		for _, host := range normalized {
			var conflicts int64
			if err := tx.Model(&models.OrgHost{}).Where("host = ? AND owner_id <> ?", host, orgID).Count(&conflicts).Error; err != nil {
				return err
			}
			if conflicts > 0 {
				return plugin.ErrHostTaken
			}
			var row models.OrgHost
			err := tx.Where("host = ? AND owner_id = ? AND source = ?", host, orgID, source).First(&row).Error
			switch {
			case err == nil:
				if err := tx.Model(&row).Update("traffic", traffic).Error; err != nil {
					return err
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				newRow := models.OrgHost{OrgID: orgID, Host: host, Traffic: traffic, Source: source}
				if err := tx.Create(&newRow).Error; err != nil {
					if isHostUniqueViolation(err) {
						return plugin.ErrHostTaken
					}
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, host := range removed {
		origin.ClearHostCache(host)
	}
	for _, host := range normalized {
		origin.ClearHostCache(host)
	}
	return nil
}

func normalizeBindHost(db *gorm.DB, raw string) (string, error) {
	host := origin.NormalizeHost(raw)
	if host == "" || host == "localhost" || host == "[::1]" {
		return "", plugin.ErrHostInvalid
	}
	if strings.ContainsAny(host, " \t\r\n/\\?#@*{}%\"'<>") {
		return "", plugin.ErrHostInvalid
	}
	if strings.Contains(host, "://") {
		return "", plugin.ErrHostInvalid
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return "", plugin.ErrHostInvalid
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", plugin.ErrHostInvalid
	}
	for _, label := range labels {
		if label == "" {
			return "", plugin.ErrHostInvalid
		}
	}
	if base := models.BaseDomain(db); base != "" && host == base {
		return "", plugin.ErrHostInvalid
	}
	return host, nil
}

func isHostUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique constraint failed") ||
		strings.Contains(lower, "duplicate key value violates unique constraint") ||
		strings.Contains(lower, "unique violation")
}
