// Package tenancy provisions and retires the automatic tenant subdomain that a
// configured shared base domain gives every newly created org.
//
// "Provisioning" is exactly one thing: writing the org's <slug>.<base> hostname
// as a row in the dns plugin's domains table — the same table origin resolves
// host→org against. There is deliberately no second resolution path: a tenant
// subdomain resolves only because the dns plugin's Domain row exists, in
// exactly the way a custom domain does.
//
// The package exists so core's org-creation code (internal/api and
// internal/auth) can provision without importing the dns plugin — the
// dependency between them runs the other way. The mirror of the domains table
// is the same bargain the origin package already makes for reading; here it is
// the minimal insert shape.
package tenancy

import (
	"errors"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/origin"
	"gorm.io/gorm"
)

// ErrNameTaken is returned by Provision when the target hostname already exists
// in the domains table. The unique index on Domain.Name is the arbiter; a
// collision is reported as an error — never skipped silently — so an org
// creation that cannot claim its address fails loudly instead of silently
// producing "some orgs have an address, some don't".
var ErrNameTaken = errors.New("tenant subdomain already taken")

// domainRow mirrors the dns plugin's Domain for the single insert provisioning
// makes. Core cannot import the plugin; this is the same trade the origin
// package's domainRow makes for reads. The dns plugin owns the table's other
// columns (zone id, provider account, …), which are all NULL/zero for a
// provisioned row and are written by the plugin's own paths. The host lists are
// carried so a table migrated from this struct satisfies the columns origin
// reads.
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

// Subdomain returns the tenant hostname for slug under the configured base
// domain, and whether the feature is on at all. When the base is unconfigured
// the second value is false and the first is "". It is the read-side twin of
// Provision, used to display the org's address without ever having to guess
// the base.
func Subdomain(db *gorm.DB, slug string) (string, bool) {
	base := models.BaseDomain(db)
	if base == "" || slug == "" {
		return "", false
	}
	return strings.ToLower(slug) + "." + base, true
}

// Provision writes the tenant subdomain row for a newly created org and returns
// its hostname and whether one was provisioned.
//
// provisioned is false (and err nil) when the feature is off — no base domain
// configured — or when this build has no domains table at all. A build without
// the dns plugin cannot resolve hostnames anyway, so there is nothing to write
// and the org behaves exactly as it did before this feature existed.
//
// A hostname collision (another org already holds this slug's subdomain, or a
// row exists from before the base was configured) returns ErrNameTaken.
func Provision(db *gorm.DB, orgID uint, slug string) (name string, provisioned bool, err error) {
	if db == nil || orgID == 0 || slug == "" {
		return "", false, nil
	}
	base := models.BaseDomain(db)
	if base == "" || !db.Migrator().HasTable("domains") {
		return "", false, nil
	}
	name = strings.ToLower(slug) + "." + base
	row := domainRow{OrgID: orgID, Name: name}
	if err := db.Create(&row).Error; err != nil {
		if isDuplicateKey(err) {
			return "", false, ErrNameTaken
		}
		return "", false, err
	}
	origin.ClearDomainCache(name)
	return name, true, nil
}

// isDuplicateKey reports whether err is a unique-index violation. GORM only
// translates these to gorm.ErrDuplicatedKey when the dialector is opened with
// TranslateError enabled, which this codebase does not do — production Postgres
// surfaces *pgconn.PgError and tests surface the sqlite text, so both raw
// shapes are matched directly.
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique constraint failed") ||
		strings.Contains(lower, "duplicate key value violates unique constraint")
}

// Retire deletes the tenant subdomain row belonging to a slug that is no longer
// the org's address (a rename). It leaves the row resolvable to nothing, which
// is the subdomain counterpart of OrgSlugHistory: a retired address must not be
// claimable by another org, and must not keep resolving to this one.
//
// Like Provision it is a no-op when the feature is off.
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
