package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/config"
)

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

func TestInstanceReadinessRequiresInstanceAdmin(t *testing.T) {
	srv, db := newTestHandler(t)

	if code, _ := getReadiness(t, srv, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous readiness: got %d, want 401", code)
	}

	const org = uint(101)
	memberUID := seedOrgMember(t, db, org, "member@x.com", "member")
	memberSession := sessionCookies(t, memberUID, org)
	if code, _ := getReadiness(t, srv, memberSession); code != http.StatusForbidden {
		t.Fatalf("member readiness: got %d, want 403", code)
	}

	adminSession := loginCookies(t, srv)
	if code, _ := getReadiness(t, srv, adminSession); code != http.StatusOK {
		t.Fatalf("admin readiness: got %d, want 200", code)
	}
}

func TestInstanceReadinessBlockedWithoutSystemSender(t *testing.T) {
	srv, _ := newTestHandler(t)

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

func TestInstanceReadinessOKWithSystemSender(t *testing.T) {
	srv, db := newTestHandler(t)
	enableMail(t, db)

	code, checks := getReadiness(t, srv, loginCookies(t, srv))
	if code != http.StatusOK {
		t.Fatalf("admin readiness: got %d, want 200", code)
	}
	if reg := findCheck(checks, "registration"); reg == nil || reg.Status != "ok" {
		t.Fatalf("registration check = %+v, want ok", reg)
	}
}

func TestInstanceReadinessOmitsSecrets(t *testing.T) {
	const (
		secretKey   = "SENTINEL-SECRET-KEY-must-never-be-served-0123456789abcdef0123456789abcdef"
		dsnPassword = "SENTINEL-DSN-PASSWORD-must-never-be-served"
	)
	cfg := &config.Config{
		AdminUser:     "admin",
		AdminPassword: "pw",
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
	if !strings.Contains(body, "postgres") {
		t.Errorf("readiness API should still name the database driver:\n%s", body)
	}
	for _, id := range []string{"public-origin", "outbound-mail", "registration", "database"} {
		if c := findCheck(checks, id); c == nil {
			t.Errorf("readiness omits check %q", id)
		}
	}
}

func TestMeReportsIsInstanceAdmin(t *testing.T) {
	srv, db := newTestHandler(t)

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
