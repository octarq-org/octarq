package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateSMTPTargetExtraEdges(t *testing.T) {
	t.Parallel()

	if err := validateSMTPTarget("smtp.example.com", 587); err != nil {
		t.Errorf("hostname on an allowed port must pass, got %v", err)
	}
	if err := validateSMTPTarget("", 587); err == nil {
		t.Error("empty host must fail")
	}
	if err := validateSMTPTarget("db.localhost", 587); err == nil {
		t.Error("*.localhost host must fail")
	}
	if err := validateSMTPTarget("box.local", 587); err == nil {
		t.Error("*.local host must fail")
	}
	if err := validateSMTPTarget("localhost", 465); err == nil {
		t.Error("localhost must fail")
	}
}

func TestCreateSMTPSenderEdges(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	full := func() *CreateSMTPSenderInput {
		in := &CreateSMTPSenderInput{Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/smtp-senders", nil))}
		in.Body.Name = "relay"
		in.Body.Host = "smtp.example.com"
		in.Body.Port = 587
		in.Body.User = "u"
		in.Body.Pass = "p"
		in.Body.FromEmail = "from@example.com"
		return in
	}

	// Missing required fields -> 400.
	minimal := full()
	minimal.Body.Pass = ""
	if _, err := p.createSMTPSender(ctx, minimal); err == nil {
		t.Error("missing pass must fail with 400")
	}
	// Non-admin denied.
	member := full()
	member.Ctx = mkCtx(func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Header.Set("X-Role", "member")
		return r
	}())
	if _, err := p.createSMTPSender(ctx, member); err == nil {
		t.Error("member must be forbidden")
	}
	// Invalid target (bad port) -> 422.
	badPort := full()
	badPort.Body.Port = 2222
	if _, err := p.createSMTPSender(ctx, badPort); err == nil {
		t.Error("restricted port must fail")
	}
	// private IP host -> 422
	private := full()
	private.Body.Host = "127.0.0.1"
	if _, err := p.createSMTPSender(ctx, private); err == nil {
		t.Error("private IP host must fail")
	}
	// Encrypt unavailable -> 500.
	p.encrypt = nil
	if _, err := p.createSMTPSender(ctx, full()); err == nil {
		t.Error("missing encrypt seam must fail with 500")
	}
	// Encrypt error -> 500.
	p.encrypt = func([]byte) (string, error) { return "", errBoom }
	if _, err := p.createSMTPSender(ctx, full()); err == nil {
		t.Error("encrypt failure must fail with 500")
	}

	// Success records the seam and exposes PassSet.
	p.encrypt = func(b []byte) (string, error) { return "enc:" + string(b), nil }
	audited := 0
	p.audit = func(*http.Request, string, string, uint, map[string]any) { audited++ }
	out, err := p.createSMTPSender(ctx, full())
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	if !out.Body.PassSet {
		t.Error("PassSet must be true when a password is stored")
	}
	if audited != 1 {
		t.Errorf("audit calls = %d, want 1", audited)
	}
}

func TestUpdateSMTPSenderEdges(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	created, err := p.createSMTPSender(ctx, &CreateSMTPSenderInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/smtp-senders", nil)),
		Body: struct {
			Name      string `json:"name"`
			Host      string `json:"host"`
			Port      int    `json:"port"`
			User      string `json:"user"`
			Pass      string `json:"pass"`
			FromEmail string `json:"fromEmail"`
		}{Name: "relay", Host: "smtp.example.com", Port: 587, User: "u", Pass: "p", FromEmail: "from@example.com"},
	})
	if err != nil {
		t.Fatalf("seed sender: %v", err)
	}
	id := created.Body.ID

	// 404 for a missing sender.
	if _, err := p.updateSMTPSender(ctx, &UpdateSMTPSenderInput{Ctx: mkCtx(httptest.NewRequest(http.MethodPut, "/api/smtp-senders/999", nil)), ID: 999}); err == nil {
		t.Error("updating a missing sender must 404")
	}

	// Partial field update.
	name := "renamed"
	host := "smtp2.example.com"
	port := 465
	user := "u2"
	from := "from2@example.com"
	out, err := p.updateSMTPSender(ctx, &UpdateSMTPSenderInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPut, "/api/smtp-senders/1", nil)),
		ID:  id,
		Body: struct {
			Name      *string `json:"name,omitempty"`
			Host      *string `json:"host,omitempty"`
			Port      *int    `json:"port,omitempty"`
			User      *string `json:"user,omitempty"`
			Pass      *string `json:"pass,omitempty"`
			FromEmail *string `json:"fromEmail,omitempty"`
		}{Name: &name, Host: &host, Port: &port, User: &user, FromEmail: &from},
	})
	if err != nil {
		t.Fatalf("update sender: %v", err)
	}
	if out.Body.Name != "renamed" || out.Body.Port != 465 || out.Body.User != "u2" || out.Body.FromEmail != "from2@example.com" {
		t.Errorf("update not persisted: %+v", out.Body)
	}

	// Password rotation re-encrypts.
	newPass := "newpass"
	p.encrypt = func(b []byte) (string, error) { return "enc:" + string(b), nil }
	rot, err := p.updateSMTPSender(ctx, &UpdateSMTPSenderInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPut, "/api/smtp-senders/1", nil)),
		ID:  id,
		Body: struct {
			Name      *string `json:"name,omitempty"`
			Host      *string `json:"host,omitempty"`
			Port      *int    `json:"port,omitempty"`
			User      *string `json:"user,omitempty"`
			Pass      *string `json:"pass,omitempty"`
			FromEmail *string `json:"fromEmail,omitempty"`
		}{Pass: &newPass},
	})
	if err != nil {
		t.Fatalf("rotate password: %v", err)
	}
	if rot.Body.Pass != "enc:newpass" {
		t.Errorf("password not re-encrypted: %q", rot.Body.Pass)
	}

	// Invalid target after update -> 422.
	badHost := "127.0.0.1"
	if _, err := p.updateSMTPSender(ctx, &UpdateSMTPSenderInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPut, "/api/smtp-senders/1", nil)),
		ID:  id,
		Body: struct {
			Name      *string `json:"name,omitempty"`
			Host      *string `json:"host,omitempty"`
			Port      *int    `json:"port,omitempty"`
			User      *string `json:"user,omitempty"`
			Pass      *string `json:"pass,omitempty"`
			FromEmail *string `json:"fromEmail,omitempty"`
		}{Host: &badHost},
	}); err == nil {
		t.Error("updating to a private host must fail")
	}
}

func TestDeleteSMTPSenderEdges(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	created, err := p.createSMTPSender(ctx, &CreateSMTPSenderInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/smtp-senders", nil)),
		Body: struct {
			Name      string `json:"name"`
			Host      string `json:"host"`
			Port      int    `json:"port"`
			User      string `json:"user"`
			Pass      string `json:"pass"`
			FromEmail string `json:"fromEmail"`
		}{Name: "del", Host: "smtp.example.com", Port: 587, User: "u", Pass: "p"},
	})
	if err != nil {
		t.Fatalf("seed sender: %v", err)
	}

	// Missing sender -> 404.
	if _, err := p.deleteSMTPSender(ctx, &DeleteSMTPSenderInput{Ctx: mkCtx(httptest.NewRequest(http.MethodDelete, "/api/smtp-senders/999", nil)), ID: 999}); err == nil {
		t.Error("deleting a missing sender must 404")
	}
	// Success.
	out, err := p.deleteSMTPSender(ctx, &DeleteSMTPSenderInput{Ctx: mkCtx(httptest.NewRequest(http.MethodDelete, "/api/smtp-senders/1", nil)), ID: created.Body.ID})
	if err != nil || !out.Body["ok"] {
		t.Fatalf("delete sender: %v", err)
	}
}

func TestListSMTPSendersExposesPassSet(t *testing.T) {
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	p.encrypt = func(b []byte) (string, error) { return "enc:" + string(b), nil }
	if _, err := p.createSMTPSender(context.Background(), &CreateSMTPSenderInput{
		Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/api/smtp-senders", nil)),
		Body: struct {
			Name      string `json:"name"`
			Host      string `json:"host"`
			Port      int    `json:"port"`
			User      string `json:"user"`
			Pass      string `json:"pass"`
			FromEmail string `json:"fromEmail"`
		}{Name: "shown", Host: "smtp.example.com", Port: 587, User: "u", Pass: "secret"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := p.listSMTPSenders(context.Background(), &ListSMTPSendersInput{Ctx: mkCtx(httptest.NewRequest(http.MethodGet, "/api/smtp-senders", nil))})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Body) != 1 || !out.Body[0].PassSet {
		t.Errorf("expected a sender with PassSet=true, got %+v", out.Body)
	}
	if !out.Body[0].PassSet {
		t.Error("PassSet must reflect the stored (encrypted) password")
	}
}

var errBoom = errBoomer{}

type errBoomer struct{}

func (errBoomer) Error() string { return "boom" }
