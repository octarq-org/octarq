package mail

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	internalmail "github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

type fakeQuotaChecker struct {
	err error
}

func (f fakeQuotaChecker) Check(_ context.Context, _ uint, _ string, _ int64) error {
	return f.err
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	se, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("expected huma.StatusError, got %T: %v", err, err)
	}
	return se.GetStatus()
}

// countingSMTP is a minimal SMTP server that counts every accepted connection
// and answers the handshake well enough for mail.NewCustomSender to complete a
// send. A blocked send must leave the counter at zero — that is the "邮件没发出"
// proof, stronger than an HTTP status alone.
func countingSMTP(t *testing.T) (host string, port int, accepted *int64) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var n int64
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&n, 1)
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				fmt.Fprintf(c, "220 localhost ESMTP\r\n")
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO"):
						fmt.Fprintf(c, "250-localhost Hello\r\n250-AUTH PLAIN\r\n250 HELP\r\n")
					case strings.HasPrefix(cmd, "AUTH PLAIN") || strings.HasPrefix(cmd, "AUTH"):
						fmt.Fprintf(c, "235 2.7.0 Authentication successful\r\n")
					case strings.HasPrefix(cmd, "MAIL FROM:"):
						fmt.Fprintf(c, "250 OK\r\n")
					case strings.HasPrefix(cmd, "RCPT TO:"):
						fmt.Fprintf(c, "250 OK\r\n")
					case cmd == "DATA":
						fmt.Fprintf(c, "354 Start mail input\r\n")
						for {
							bodyLine, err := r.ReadString('\n')
							if err != nil || bodyLine == ".\r\n" {
								break
							}
						}
						fmt.Fprintf(c, "250 OK\r\n")
					case cmd == "QUIT":
						fmt.Fprintf(c, "221 Bye\r\n")
						return
					}
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { l.Close() })
	return "127.0.0.1", l.Addr().(*net.TCPAddr).Port, &n
}

// mailQuotaPlugin builds a mounted mail plugin backed by a real DB and a
// counting SMTP relay. checker nil means "no quota checker registered at all".
func mailQuotaPlugin(t *testing.T, checker plugin.QuotaChecker) (*Plugin, *int64) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	p := New()
	if err := db.AutoMigrate(append(models.AllModels(), p.Models()...)...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&SMTPSender{})
	host, port, accepted := countingSMTP(t)
	p.Mount(nil, &plugin.Context{
		DB:          db,
		OrgID:       func(*http.Request) uint { return 100 },
		Decrypt:     func(encoded string) ([]byte, error) { return []byte(encoded), nil },
		RequireRole: func(*http.Request, string) bool { return true },
		Lookup: func(name string) (any, bool) {
			if name == plugin.ServiceQuotaChecker && checker != nil {
				return checker, true
			}
			return nil, false
		},
	})
	s := SMTPSender{
		ID:        1,
		OrgID:     100,
		Host:      host,
		Port:      port,
		User:      "test",
		Pass:      "",
		FromEmail: "noreply@example.com",
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("seed SMTP sender: %v", err)
	}
	return p, accepted
}

func sendEmailInput() *SendEmailInput {
	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	return &SendEmailInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: struct {
			internalmail.Message
			SMTPSenderID uint `json:"smtpSenderId"`
			TrackLinks   bool `json:"trackLinks"`
		}{
			SMTPSenderID: 1,
			Message: internalmail.Message{
				To:      []string{"a@example.com"},
				Subject: "test",
				Text:    "hi",
			},
		},
	}
}

// No checker registered (self-hosted) must behave exactly as before the seam
// existed: the mail actually goes out.
func TestSendEmailNoQuotaCheckerSends(t *testing.T) {
	t.Parallel()
	p, accepted := mailQuotaPlugin(t, nil)
	if _, err := p.sendEmail(context.Background(), sendEmailInput()); err != nil {
		t.Fatalf("sendEmail without checker: %v", err)
	}
	if atomic.LoadInt64(accepted) == 0 {
		t.Error("expected the SMTP relay to be reached when no checker is registered")
	}
}

// Exceeded quota → 429, and the relay is never touched (no half-sent mail).
func TestSendEmailQuotaExceededBlocks(t *testing.T) {
	t.Parallel()
	p, accepted := mailQuotaPlugin(t, fakeQuotaChecker{err: plugin.ErrQuotaExceeded})
	_, err := p.sendEmail(context.Background(), sendEmailInput())
	if got := statusOf(t, err); got != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d (%v)", got, err)
	}
	if atomic.LoadInt64(accepted) != 0 {
		t.Errorf("blocked send still reached SMTP (%d connections); want 0", atomic.LoadInt64(accepted))
	}
}

// Unavailable (plan lacks the capability) → 402, not 429. The two must stay
// distinct: one is "used up", the other is "upgrade to get this".
func TestSendEmailQuotaUnavailableIs402(t *testing.T) {
	t.Parallel()
	p, accepted := mailQuotaPlugin(t, fakeQuotaChecker{err: plugin.ErrQuotaUnavailable})
	_, err := p.sendEmail(context.Background(), sendEmailInput())
	if got := statusOf(t, err); got != http.StatusPaymentRequired {
		t.Fatalf("want 402, got %d (%v)", got, err)
	}
	if atomic.LoadInt64(accepted) != 0 {
		t.Errorf("402 refusal still reached SMTP (%d connections); want 0", atomic.LoadInt64(accepted))
	}
}

// A host whose Context.Lookup is nil (old host / MCP composition) must not
// panic — it must read as "no checker".
func TestSendEmailNilLookupNoPanic(t *testing.T) {
	t.Parallel()
	p, accepted := mailQuotaPlugin(t, nil)
	p.ctx = &plugin.Context{} // nil Lookup
	if _, err := p.sendEmail(context.Background(), sendEmailInput()); err != nil {
		t.Fatalf("nil Lookup must pass, got %v", err)
	}
	if atomic.LoadInt64(accepted) == 0 {
		t.Error("expected SMTP to be reached with nil Lookup")
	}
}
