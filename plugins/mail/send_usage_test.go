package mail

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/glebarez/sqlite"
	internalmail "github.com/octarq-org/octarq/internal/mail"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

func startDummySMTPServer(t *testing.T) (string, int, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host := "127.0.0.1"
	port := l.Addr().(*net.TCPAddr).Port

	var wg sync.WaitGroup
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				r := bufio.NewReader(c)
				fmt.Fprintf(c, "220 localhost ESMTP\r\n")
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						break
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					if strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO") {
						fmt.Fprintf(c, "250-localhost Hello\r\n250-AUTH PLAIN\r\n250 HELP\r\n")
					} else if strings.HasPrefix(cmd, "AUTH PLAIN") || strings.HasPrefix(cmd, "AUTH") {
						fmt.Fprintf(c, "235 2.7.0 Authentication successful\r\n")
					} else if strings.HasPrefix(cmd, "MAIL FROM:") {
						fmt.Fprintf(c, "250 OK\r\n")
					} else if strings.HasPrefix(cmd, "RCPT TO:") {
						fmt.Fprintf(c, "250 OK\r\n")
					} else if cmd == "DATA" {
						fmt.Fprintf(c, "354 Start mail input\r\n")
						for {
							bodyLine, err := r.ReadString('\n')
							if err != nil || bodyLine == ".\r\n" {
								break
							}
						}
						fmt.Fprintf(c, "250 OK\r\n")
					} else if cmd == "QUIT" {
						fmt.Fprintf(c, "221 Bye\r\n")
						break
					}
				}
			}(conn)
		}
	}()

	return host, port, func() {
		l.Close()
		wg.Wait()
	}
}
func TestMailRecordUsageSuccessAndFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	p := New()
	if err := db.AutoMigrate(append(models.AllModels(), p.Models()...)...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where("1 = 1").Delete(&SMTPSender{})

	host, port, stopSMTP := startDummySMTPServer(t)
	defer stopSMTP()

	var mu sync.Mutex
	var calls []struct {
		orgID  uint
		metric string
		n      int64
	}

	p.Mount(nil, &plugin.Context{
		DB:      db,
		OrgID:   func(r *http.Request) uint { return 100 },
		Decrypt: func(encoded string) ([]byte, error) { return []byte(encoded), nil },
		RecordUsage: func(orgID uint, metric string, n int64) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, struct {
				orgID  uint
				metric string
				n      int64
			}{orgID, metric, n})
		},
		RequireRole: func(*http.Request, string) bool { return true },
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
	db.Create(&s)

	// 1. sendMail path success
	err = p.sendMail(100, "user@example.com", "hello", "body", "body")
	if err != nil {
		t.Fatalf("sendMail failed: %v", err)
	}

	// 2. sendEmail HTTP path success (2 recipients)
	req := httptest.NewRequest(http.MethodPost, "/api/emails/send", nil)
	input := &SendEmailInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: struct {
			internalmail.Message
			SMTPSenderID uint `json:"smtpSenderId"`
			TrackLinks   bool `json:"trackLinks"`
		}{
			SMTPSenderID: 1,
			Message: internalmail.Message{
				To:      []string{"a@example.com", "b@example.com"},
				Subject: "test",
				Text:    "hi",
			},
		},
	}
	_, err = p.sendEmail(context.Background(), input)
	if err != nil {
		t.Fatalf("sendEmail failed: %v", err)
	}

	// 3. Send failure path: bad port
	sFail := SMTPSender{
		ID:        2,
		OrgID:     200,
		Host:      "127.0.0.1",
		Port:      1, // unreachable port
		User:      "test",
		Pass:      "",
		FromEmail: "noreply@example.com",
	}
	db.Create(&sFail)

	_ = p.sendMail(200, "user@example.com", "hello", "body", "body")

	inputFail := &SendEmailInput{
		Ctx: humago.NewContext(nil, req, httptest.NewRecorder()),
		Body: struct {
			internalmail.Message
			SMTPSenderID uint `json:"smtpSenderId"`
			TrackLinks   bool `json:"trackLinks"`
		}{
			SMTPSenderID: 2,
			Message: internalmail.Message{
				To:      []string{"c@example.com"},
				Subject: "test",
				Text:    "hi",
			},
		},
	}
	_, _ = p.sendEmail(context.Background(), inputFail)

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 2 {
		t.Fatalf("expected 2 successful RecordUsage calls, got %d", len(calls))
	}
	// Call 1 from sendMail: quantity = 1
	if calls[0].orgID != 100 || calls[0].metric != "mail" || calls[0].n != 1 {
		t.Errorf("call 0 unexpected: %+v", calls[0])
	}
	// Call 2 from sendEmail HTTP: quantity = 2 (len(msg.To))
	if calls[1].orgID != 100 || calls[1].metric != "mail" || calls[1].n != 2 {
		t.Errorf("call 1 unexpected: %+v", calls[1])
	}
}
