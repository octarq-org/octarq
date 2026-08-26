package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/config"
	"github.com/octarq-org/octarq/internal/models"
	mailmodels "github.com/octarq-org/octarq/plugins/mail"
)

// instanceReadinessBody is the decoded wire shape of GET /api/instance/readiness.
type readinessCheckWire struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	FixPath string `json:"fixPath"`
}

func getReadiness(t *testing.T, srv http.Handler, cookies []*http.Cookie) (int, []readinessCheckWire) {
	t.Helper()
	rec := do(srv, "GET", "/api/instance/readiness", cookies, "")
	var checks []readinessCheckWire
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &checks); err != nil {
			t.Fatalf("decode readiness response: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, checks
}

func findCheck(checks []readinessCheckWire, id string) *readinessCheckWire {
	for i := range checks {
		if checks[i].ID == id {
			return &checks[i]
		}
	}
	return nil
}

// TestInstanceReadinessRequiresInstanceAdmin pins the gate: the checks describe
// the whole deployment and whether sign-up even works, so a tenant member must
// not read them.
func TestInstanceReadinessRequiresInstanceAdmin(t *testing.T) {
	srv, db := newTestHandler(t)

	// Anonymous → 401.
	if code, _ := getReadiness(t, srv, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous readiness: got %d, want 401", code)
	}

	// Plain member → 403.
	const org = uint(101)
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	memberSession := sessionCookies(t, memberUID, org)
	if code, _ := getReadiness(t, srv, memberSession); code != http.StatusForbidden {
		t.Fatalf("member readiness: got %d, want 403", code)
	}

	// Instance admin → 200.
	adminSession := loginCookies(t, srv)
	if code, _ := getReadiness(t, srv, adminSession); code != http.StatusOK {
		t.Fatalf("admin readiness: got %d, want 200", code)
	}
}

// TestInstanceReadinessBlockedWithoutSystemSender is the guard for the
// self-contradiction check: an instance that requires a verified email but has
// no system sender has a broken registration flow, and the readiness API must
// say blocked — not degraded, not ok.
func TestInstanceReadinessBlockedWithoutSystemSender(t *testing.T) {
	srv, _ := newTestHandler(t)
	// require_email_verification defaults to on; no SMTPSender rows exist.

	code, checks := getReadiness(t, srv, loginCookies(t, srv))
	if code != http.StatusOK {
		t.Fatalf("admin readiness: got %d, want 200", code)
	}
	reg := findCheck(checks, "registration")
	if reg == nil || reg.Status != "blocked" {
		t.Fatalf("readiness omits the registration check or it is not blocked: %+v", checks)
	}
	if !strings.Contains(reg.Detail, "verification") {
		t.Fatalf("blocked detail does not explain the verification dead end: %s", reg.Detail)
	}
}

// TestInstanceReadinessOKWithSystemSender is the other direction: with a sender
// configured, registration must read ok.
func TestInstanceReadinessOKWithSystemSender(t *testing.T) {
	srv, db := newTestHandler(t)
	if err := db.Create(&mailmodels.SMTPSender{OrgID: 1, Name: "test", Host: "smtp.example.com", Port: 587, User: "u", Pass: "enc", FromEmail: "noreply@example.com"}).Error; err != nil {
		t.Fatalf("seed sender: %v", err)
	}

	code, checks := getReadiness(t, srv, loginCookies(t, srv))
	if code != http.StatusOK {
		t.Fatalf("admin readiness: got %d, want 200", code)
	}
	if reg := findCheck(checks, "registration"); reg == nil || reg.Status != "ok" {
		t.Fatalf("registration check = %+v, want ok", reg)
	}
}

// TestInstanceReadinessOmitsSecrets is the API twin of the log-side
// TestReadinessReportOmitsSecrets: the response travels to a browser session,
// but the same rule holds — a KEK or DSN password landing in a JSON body is a
// compromise nobody notices. Sentinel values make any hit unambiguous.
func TestInstanceReadinessOmitsSecrets(t *testing.T) {
	const (
		secretKey   = "SENTINEL-SECRET-KEY-must-never-be-served-0123456789abcdef0123456789abcdef"
		dsnPassword = "SENTINEL-DSN-PASSWORD-must-never-be-served"
	)
	cfg := &config.Config{
		AdminUser:     "admin",
		AdminPassword: "pw", // loginCookies logs in with admin/pw; readiness never reads this value
		SecretKey:     secretKey,
		DBDriver:      "postgres",
		DBDSN:         "postgres://octarq:" + dsnPassword + "@db.internal:5432/octarq",
	}
	_, srv, _ := newTestHandlerRawCfg(t, cfg)

	code, checks := getReadiness(t, srv, loginCookies(t, srv))
	if code != http.StatusOK {
		t.Fatalf("admin readiness: got %d, want 200", code)
	}
	if len(checks) == 0 {
		t.Fatal("readiness returned no checks; the leak assertion below would be vacuous")
	}
	raw, err := json.Marshal(checks)
	if err != nil {
		t.Fatalf("marshal readiness: %v", err)
	}
	body := string(raw)
	for name, secret := range map[string]string{
		"OCTARQ_SECRET_KEY":     secretKey,
		"database DSN password": dsnPassword,
	} {
		if strings.Contains(body, secret) {
			t.Errorf("readiness API leaks the %s value:\n%s", name, body)
		}
	}
	// The DSN is withheld, but the driver still names itself.
	if !strings.Contains(body, "postgres") {
		t.Errorf("readiness API should still name the database driver:\n%s", body)
	}
	// The response must carry the fixPath contract fields.
	for _, id := range []string{"public-origin", "outbound-mail", "registration", "database"} {
		if c := findCheck(checks, id); c == nil {
			t.Errorf("readiness omits check %q", id)
		}
	}
}

// TestMeReportsIsInstanceAdmin pins the new source of the instance identity:
// /api/auth/me now answers it, so instance admin does not have to ride along
// in the tenant settings response forever (the frontend still reads it there —
// the old field is removed in a later batch once the frontend switches).
func TestMeReportsIsInstanceAdmin(t *testing.T) {
	srv, db := newTestHandler(t)

	// Bootstrap admin → true.
	rec := do(srv, "GET", "/api/auth/me", loginCookies(t, srv), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me as admin: got %d", rec.Code)
	}
	var adminMe struct {
		IsInstanceAdmin bool `json:"isInstanceAdmin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &adminMe); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if !adminMe.IsInstanceAdmin {
		t.Fatal("me as admin: isInstanceAdmin = false, want true")
	}

	// Plain member → false.
	const org = uint(101)
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	rec = do(srv, "GET", "/api/auth/me", sessionCookies(t, memberUID, org), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me as member: got %d", rec.Code)
	}
	var memberMe struct {
		IsInstanceAdmin bool `json:"isInstanceAdmin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &memberMe); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if memberMe.IsInstanceAdmin {
		t.Fatal("me as member: isInstanceAdmin = true, want false")
	}
}

// TestInstanceSettingsSystemSenderIDRoundTrip pins the instance setting that
// names the system sender: write it, read it back, clear it with 0.
func TestInstanceSettingsSystemSenderIDRoundTrip(t *testing.T) {
	srv, db := newTestHandler(t)
	admin := loginCookies(t, srv)

	// Unset initially.
	rec := do(srv, "GET", "/api/instance-settings", admin, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get instance settings: got %d (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := body["systemSenderId"].(float64); v != 0 {
		t.Fatalf("fresh instance systemSenderId = %v, want 0", v)
	}

	// Set it.
	rec = do(srv, "PUT", "/api/instance-settings", admin, `{"systemSenderId":7}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put instance settings: got %d (%s)", rec.Code, rec.Body.String())
	}
	var setting models.Setting
	if err := db.First(&setting, "key = ?", keySystemSenderID).Error; err != nil {
		t.Fatalf("setting row missing: %v", err)
	}
	if setting.Value != "7" {
		t.Fatalf("stored value = %q, want 7", setting.Value)
	}

	// Read it back.
	rec = do(srv, "GET", "/api/instance-settings", admin, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get instance settings: got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := body["systemSenderId"].(float64); v != 7 {
		t.Fatalf("systemSenderId after set = %v, want 7", v)
	}

	// A plain member is refused.
	const org = uint(101)
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	rec = do(srv, "PUT", "/api/instance-settings", sessionCookies(t, memberUID, org), `{"systemSenderId":7}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member put instance settings: got %d, want 403", rec.Code)
	}

	// Clear with 0.
	rec = do(srv, "PUT", "/api/instance-settings", admin, `{"systemSenderId":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear instance settings: got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(srv, "GET", "/api/instance-settings", admin, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := body["systemSenderId"].(float64); v != 0 {
		t.Fatalf("systemSenderId after clear = %v, want 0", v)
	}
}
