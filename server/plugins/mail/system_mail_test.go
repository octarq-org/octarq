package mail

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// captureSMTP speaks just enough ESMTP to complete a send and returns the DATA
// payload so tests can assert what actually left the instance.
func captureSMTP(t *testing.T) (host string, port int, got func() string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	var mu sync.Mutex
	var captured string
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		w.WriteString("220 fake ESMTP\r\n")
		w.Flush()
		inData := false
		var body strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					inData = false
					mu.Lock()
					captured = body.String()
					mu.Unlock()
					w.WriteString("250 ok\r\n")
					w.Flush()
					continue
				}
				body.WriteString(line + "\n")
				continue
			}
			verb := strings.ToUpper(strings.Fields(line)[0])
			switch verb {
			case "EHLO":
				w.WriteString("250-fake greets you\r\n250-AUTH PLAIN\r\n250 SIZE 1000000\r\n")
			case "AUTH":
				w.WriteString("235 ok\r\n")
			case "MAIL", "RCPT":
				w.WriteString("250 ok\r\n")
			case "DATA":
				w.WriteString("354 go ahead\r\n")
				inData = true
			case "QUIT":
				w.WriteString("221 bye\r\n")
				w.Flush()
				return
			default:
				w.WriteString("250 ok\r\n")
			}
			w.Flush()
		}
	}()
	host = "127.0.0.1"
	port = ln.Addr().(*net.TCPAddr).Port
	return host, port, func() string {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
}

// newSystemMailTestPlugin builds a mail plugin over a fresh DB with the
// pass-through decrypt seam used across this package's send tests and a
// configurable global-setting reader for the system-sender setting.
func newSystemMailTestPlugin(t *testing.T, globalSetting func(string) string) (*Plugin, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&SMTPSender{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// The shared in-memory cache is reused across tests in this package, so
	// wipe rows a previous test may have left.
	db.Where("1 = 1").Delete(&SMTPSender{})
	p := New()
	p.db = db
	p.decrypt = func(encoded string) ([]byte, error) { return []byte(encoded), nil }
	if globalSetting != nil {
		p.getGlobalSetting = globalSetting
	}
	return p, db
}

// TestSendSystemMailNoSenderReturnsClearError pins the fail-closed half of the
// system sender: with zero SMTP senders the send must fail with a message that
// says what is missing, and mailReady must answer false — the state a fresh
// `docker run` instance is in, where registration 503s instead of promising an
// email that can never arrive.
func TestSendSystemMailNoSenderReturnsClearError(t *testing.T) {
	p, _ := newSystemMailTestPlugin(t, nil)

	if p.mailReady() {
		t.Fatal("mailReady = true with no sender configured")
	}
	err := p.sendSystemMail("user@example.com", "Verify your octarq email", "", "verify me")
	if err == nil {
		t.Fatal("sendSystemMail succeeded with no SMTP sender configured")
		return
	}
	if !strings.Contains(err.Error(), "no SMTP sender") {
		t.Fatalf("sendSystemMail error does not explain the missing sender: %v", err)
	}
}

// TestSendSystemMailDeliversWithoutOrg pins the send path itself: a sender on
// some workspace delivers system mail to the fake relay. The send carries no
// orgID — the recipient's membership (or lack of one) must not matter.
// TestSendSystemMailDeliversWithoutOrg pins the send path itself: an explicitly
// configured system sender delivers system mail to the fake relay. The send carries no
// orgID — the recipient's membership (or lack of one) must not matter.
func TestSendSystemMailDeliversWithoutOrg(t *testing.T) {
	host, port, got := captureSMTP(t)
	p, db := newSystemMailTestPlugin(t, func(key string) string {
		if key == "mail_system_sender_id" {
			return "1"
		}
		return ""
	})
	if err := db.Create(&SMTPSender{
		ID: 1, OrgID: 1, Name: "relay", Host: host, Port: port,
		User: "u", Pass: "pw", FromEmail: "noreply@example.com",
	}).Error; err != nil {
		t.Fatalf("seed sender: %v", err)
	}

	err := p.sendSystemMail("user@example.com", "Verify your octarq email", "", "verify me")
	if err != nil {
		t.Fatalf("sendSystemMail failed: %v", err)
	}
	body := got()
	if !strings.Contains(body, "verify me") {
		t.Fatalf("relay received no expected payload:\n%s", body)
	}
}

// TestSystemSenderResolution pins the fail-closed resolution order: the
// explicitly configured sender (mail_system_sender_id) wins when it exists;
// unset or stale ids fail closed without falling back to arbitrary tenant senders.
func TestSystemSenderResolution(t *testing.T) {
	settings := map[string]string{}
	p, db := newSystemMailTestPlugin(t, func(key string) string { return settings[key] })
	first := SMTPSender{ID: 1, OrgID: 1, Name: "first"}
	second := SMTPSender{ID: 2, OrgID: 2, Name: "second"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("seed second: %v", err)
	}

	// Unset → fail closed.
	if _, err := p.systemSender(); err == nil {
		t.Fatal("unset setting must fail closed without fallback to lowest id")
	}

	// Configured → that sender.
	settings["mail_system_sender_id"] = "2"
	s, err := p.systemSender()
	if err != nil {
		t.Fatalf("systemSender: %v", err)
	}
	if s.ID != 2 {
		t.Fatalf("configured setting resolved sender %d, want 2", s.ID)
	}

	// Stale id (sender deleted) → fail closed.
	settings["mail_system_sender_id"] = "999"
	if _, err := p.systemSender(); err == nil {
		t.Fatal("stale setting must fail closed without fallback to lowest id")
	}
}
