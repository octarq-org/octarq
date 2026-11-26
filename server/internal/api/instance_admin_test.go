package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/auth"
	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/geo"
	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

func newHandlerForAdminTest(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	cfg := &config.Config{AdminUser: "operator@example.com", AdminPassword: "pw", SecretKey: "secret"}
	cipher := crypto.New(cfg.SecretKey)
	authMgr := auth.New(cfg, cipher).WithDB(db)
	g, _ := geo.Open("")
	h := New(cfg, db, cipher, authMgr, g, testQueue())
	return h, db
}

func isInstanceAdminFor(t *testing.T, h *Handler, uid uint) bool {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/instance-settings", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), uid))
	return h.isInstanceAdmin(req)
}

func TestInstanceAdminBoundToConfiguredAdmin_NotOrg1(t *testing.T) {
	h, db := newHandlerForAdminTest(t)

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

	if isInstanceAdminFor(t, h, attacker.ID) {
		t.Fatal("attacker who registered first (owner of org 1) must NOT be instance admin")
	}

	uid, _, err := h.authenticate("operator@example.com", "pw")
	if err != nil {
		t.Fatalf("configured admin credential should authenticate: %v", err)
	}
	if uid == attacker.ID {
		t.Fatal("operator resolved to the attacker's user id")
	}

	if !isInstanceAdminFor(t, h, uid) {
		t.Fatal("configured admin must be instance admin")
	}
	if isInstanceAdminFor(t, h, attacker.ID) {
		t.Fatal("attacker must never become instance admin regardless of login order")
	}
}

func TestInstanceAdminBackfillMigration(t *testing.T) {
	db := newTestDB(t)
	owner := models.User{Email: "legacy-admin@example", PasswordHash: "x"}
	db.Create(&owner)
	org := models.Org{Name: "legacy", Slug: "legacy", InboundToken: "t"}
	db.Create(&org)
	db.Create(&models.OrgMember{OrgID: 1, UserID: owner.ID, Role: "owner"})

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

func TestInstanceAdminEnvHasVerifiedEmail(t *testing.T) {
	h, db := newHandlerForAdminTest(t)
	uid, _, err := h.authenticate("operator@example.com", "pw")
	if err != nil {
		t.Fatalf("configured admin credential should authenticate: %v", err)
	}
	var user models.User
	if err := db.First(&user, uid).Error; err != nil {
		t.Fatalf("find admin: %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("env created instance admin must have EmailVerified = true")
	}
}

func TestInstanceAdminEmailVerifiedCorrectionOnBootstrapAndLogin(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	adminUser := models.User{
		Email:           "admin",
		PasswordHash:    "",
		IsInstanceAdmin: true,
		EmailVerified:   false,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create unverified admin: %v", err)
	}

	uid := h.bootstrapUserID("admin", 1)
	if uid != adminUser.ID {
		t.Fatalf("expected admin UID %d, got %d", adminUser.ID, uid)
	}

	var user models.User
	if err := db.First(&user, adminUser.ID).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("bootstrapUserID should have auto-corrected EmailVerified to true")
	}

	db.Model(&user).Update("email_verified", false)

	rec := do(srv, "POST", "/api/auth/login", nil, `{"email":"admin","password":"pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login failed: got %d (%s)", rec.Code, rec.Body.String())
	}

	if err := db.First(&user, adminUser.ID).Error; err != nil {
		t.Fatalf("find user after login: %v", err)
	}
	if !user.EmailVerified {
		t.Fatal("login should have auto-corrected EmailVerified to true for instance admin")
	}
}
