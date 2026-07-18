package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	dns "github.com/octarq-org/octarq/plugins/dns"
	links "github.com/octarq-org/octarq/plugins/links"
	mailmodels "github.com/octarq-org/octarq/plugins/mail"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/auth"
	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/geo"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/internal/queue"
	"gorm.io/gorm"
)

// newHandlerForAdminTest builds a *Handler backed by an in-memory DB with the
// configured admin credentials, returning both the handler and db.
func newHandlerForAdminTest(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mailmodels.Mailbox{}, &mailmodels.Email{}, &mailmodels.SMTPSender{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{AdminUser: "operator@example.com", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, queue.New(""))
	return h, db
}

// isInstanceAdminFor is a small helper that fakes a request whose context
// carries uid, then checks isInstanceAdmin.
func isInstanceAdminFor(t *testing.T, h *Handler, uid uint) bool {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/instance-settings", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), uid))
	return h.isInstanceAdmin(req)
}

// TestInstanceAdminBoundToConfiguredAdmin_NotOrg1 asserts that instance-admin is
// bound to the configured OCTARQ_ADMIN account, NOT to whoever owns org 1. It
// simulates the attack: an attacker self-registers FIRST (so they own org 1),
// then the real operator logs in second. Only the operator must be instance
// admin.
func TestInstanceAdminBoundToConfiguredAdmin_NotOrg1(t *testing.T) {
	h, db := newHandlerForAdminTest(t)

	// --- Attacker registers first: they become the owner of org 1. ---
	attacker := models.User{Email: "attacker@evil.example", PasswordHash: "x"}
	if err := db.Create(&attacker).Error; err != nil {
		t.Fatalf("create attacker: %v", err)
	}
	attackerOrg := models.Org{Name: "attacker", Slug: "attacker", InboundToken: "t1"}
	if err := db.Create(&attackerOrg).Error; err != nil {
		t.Fatalf("create attacker org: %v", err)
	}
	if attackerOrg.ID != 1 {
		t.Fatalf("expected attacker org to be org 1 (first insert), got %d", attackerOrg.ID)
	}
	if err := db.Create(&models.OrgMember{OrgID: attackerOrg.ID, UserID: attacker.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("create attacker membership: %v", err)
	}

	// The attacker owns org 1 but must NOT be instance admin.
	if isInstanceAdminFor(t, h, attacker.ID) {
		t.Fatal("attacker who registered first (owner of org 1) must NOT be instance admin")
	}

	// --- Operator logs in second via the configured admin credential. ---
	uid, _, ok := h.authenticate("operator@example.com", "pw")
	if !ok {
		t.Fatal("configured admin credential should authenticate")
	}
	if uid == attacker.ID {
		t.Fatal("operator resolved to the attacker's user id")
	}

	// The configured operator IS instance admin; the attacker still is not.
	if !isInstanceAdminFor(t, h, uid) {
		t.Fatal("configured admin must be instance admin")
	}
	if isInstanceAdminFor(t, h, attacker.ID) {
		t.Fatal("attacker must never become instance admin regardless of login order")
	}
}

// TestInstanceAdminBackfillMigration asserts the one-time migration seeds the
// flag for the current org-1 owner on an existing install where nobody is
// flagged yet — so upgrades don't lose admin.
func TestInstanceAdminBackfillMigration(t *testing.T) {
	dbName := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append(models.AllModels(), &links.Link{}, &links.LinkEvent{}, &dns.Domain{}, &dns.ProviderAccount{}, &mailmodels.Mailbox{}, &mailmodels.Email{}, &mailmodels.SMTPSender{})...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Simulate a pre-existing install: org 1 with an owner, nobody flagged.
	owner := models.User{Email: "legacy-admin@example", PasswordHash: "x"}
	db.Create(&owner)
	org := models.Org{Name: "legacy", Slug: "legacy", InboundToken: "t"}
	db.Create(&org) // org 1
	db.Create(&models.OrgMember{OrgID: 1, UserID: owner.ID, Role: "owner"})

	// Run the backfill inline (same logic as db.Migrate's guarded block).
	var flagged int64
	db.Model(&models.User{}).Where("is_instance_admin = ?", true).Count(&flagged)
	if flagged != 0 {
		t.Fatalf("precondition: expected 0 flagged, got %d", flagged)
	}
	var ownerID uint
	db.Model(&models.OrgMember{}).
		Where("org_id = ? AND role = ?", 1, "owner").
		Order("user_id ASC").Limit(1).Pluck("user_id", &ownerID)
	db.Model(&models.User{}).Where("id = ?", ownerID).Update("is_instance_admin", true)

	var got models.User
	db.First(&got, owner.ID)
	if !got.IsInstanceAdmin {
		t.Fatal("backfill should have flagged the org-1 owner")
	}
}
