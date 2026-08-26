package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin/safehttp"
)

// startTestSMPListener serves one minimal SMTP conversation on loopback so
// testSMTPSender's delivery can be observed end to end.
func startTestSMPListener(t *testing.T) (port string, received func() []string) {
	t.Helper()
	safehttp.SetAllowPrivateSMTP(true)
	t.Cleanup(func() { safehttp.SetAllowPrivateSMTP(false) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var got []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		w := bufio.NewWriter(conn)
		r := bufio.NewReader(conn)
		fmt.Fprint(w, "220 mock.local ESMTP\r\n")
		w.Flush()
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			verb := strings.ToUpper(strings.Fields(line)[0])
			switch verb {
			case "EHLO":
				fmt.Fprint(w, "250-mock.local\r\n250-AUTH PLAIN\r\n250 8BITMIME\r\n")
			case "AUTH":
				f := strings.Fields(line)
				ok := false
				if len(f) >= 3 && f[1] == "PLAIN" {
					raw, _ := base64.StdEncoding.DecodeString(f[2])
					parts := strings.Split(string(raw), "\x00")
					ok = len(parts) == 3 && parts[1] == "u" && parts[2] == "p"
				}
				if ok {
					fmt.Fprint(w, "235 ok\r\n")
				} else {
					fmt.Fprint(w, "535 auth failed\r\n")
				}
			case "DATA":
				fmt.Fprint(w, "354 go\r\n")
				w.Flush()
				var b strings.Builder
				for {
					dl, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if dl == ".\r\n" || dl == ".\n" {
						break
					}
					b.WriteString(dl)
				}
				got = append(got, b.String())
				fmt.Fprint(w, "250 queued\r\n")
			case "QUIT":
				fmt.Fprint(w, "221 bye\r\n")
				w.Flush()
				return
			default:
				fmt.Fprint(w, "250 ok\r\n")
			}
			w.Flush()
		}
	}()

	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, func() []string { <-done; return got }
}

func seedTestSender(t *testing.T, p *Plugin, orgID uint, host string, port int) uint {
	t.Helper()
	s := SMTPSender{Name: "probe", Host: host, Port: port, User: "u", Pass: "p", FromEmail: fmt.Sprintf("relay@%d.test", orgID)}
	s.OrgID = orgID
	if err := p.db.Create(&s).Error; err != nil {
		t.Fatalf("seed sender: %v", err)
	}
	return s.ID
}

func TestSMTPSenderTest_AuthzAndScoping(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	id := seedTestSender(t, p, 1, "203.0.113.10", 25)

	member := httptest.NewRequest(http.MethodPost, "/", nil)
	member.Header.Set("X-Role", "member")
	if _, err := p.testSMTPSender(ctx, &TestSMTPSenderInput{Ctx: mkCtx(member), ID: id}); err == nil {
		t.Error("member must be denied")
	}

	otherOrg := httptest.NewRequest(http.MethodPost, "/", nil)
	otherOrg.Header.Set("X-Org-ID", "2")
	if _, err := p.testSMTPSender(ctx, &TestSMTPSenderInput{Ctx: mkCtx(otherOrg), ID: id}); err == nil {
		t.Error("cross-org id must be 404")
	}

	missing := httptest.NewRequest(http.MethodPost, "/", nil)
	if _, err := p.testSMTPSender(ctx, &TestSMTPSenderInput{Ctx: mkCtx(missing), ID: 99999}); err == nil {
		t.Error("unknown id must be 404")
	}
}

func TestSMTPSenderTest_DeliversThroughSender(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	port, received := startTestSMPListener(t)
	hostNum := 1 // setupFullMailTestDB pins X-Org-ID=1
	id := seedTestSender(t, p, uint(hostNum), "127.0.0.1", mustAtoi(t, port))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	out, err := p.testSMTPSender(ctx, &TestSMTPSenderInput{Ctx: mkCtx(req), ID: id})
	if err != nil {
		t.Fatalf("testSMTPSender against live mock relay: %v", err)
	}
	if !out.Body["ok"] {
		t.Error("expected ok=true")
	}
	data := received()
	if len(data) != 1 || !strings.Contains(data[0], "Subject: SMTP test from octarq") {
		t.Errorf("expected exactly one delivered test message with the test subject, got %d messages", len(data))
	}
}

func TestSMTPSenderTest_FailureIsActionable(t *testing.T) {
	t.Parallel()
	p, mkCtx := setupFullMailTestDB(t)
	wipeMailTables(t, p)
	ctx := context.Background()

	// A port with no listener surfaces the dial error verbatim.
	id := seedTestSender(t, p, 1, "127.0.0.1", closedPort(t))
	_, err := p.testSMTPSender(ctx, &TestSMTPSenderInput{Ctx: mkCtx(httptest.NewRequest(http.MethodPost, "/", nil)), ID: id})
	if err == nil {
		t.Fatal("dialing a dead relay must fail")
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("non-numeric port %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// closedPort grabs a listening socket and closes it, leaving a port that
// refuses connections for the lifetime of the test.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
