package api

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/internal/crypto"
	"github.com/octarq-org/octarq/internal/safehttp"
	mailmodels "github.com/octarq-org/octarq/plugins/mail"
)

// captureSMTP speaks just enough ESMTP to complete a send and returns the DATA
// payload, so tests can assert what actually left the instance over the wire.
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

// TestVerificationEmailDeliveredWithoutOrgMembership is THE guard for the bug
// this batch fixes: registration used to resolve the org for the verification
// email BEFORE the org transaction ran, so primaryOrgForUser returned 0 and
// sendMail(0, …) found no sender — the mail never left the instance, on any
// deployment, SMTP configured or not. The verification email now goes through
// the instance's system sender, which must deliver while the new user still
// belongs to no workspace at all.
func TestVerificationEmailDeliveredWithoutOrgMembership(t *testing.T) {
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	srv, db := newTestHandler(t)
	host, port, got := captureSMTP(t)

	// Rebuild the handler's cipher (same KEK, same envelope store) so the
	// sender password is encryptable with the key the mail plugin will decrypt
	// with.
	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("enable envelope: %v", err)
	}
	enc, err := cipher.Encrypt([]byte("pw"))
	if err != nil {
		t.Fatalf("encrypt sender pass: %v", err)
	}
	if err := db.Create(&mailmodels.SMTPSender{
		OrgID: 1, Name: "relay", Host: host, Port: port,
		User: "u", Pass: enc, FromEmail: "noreply@example.com",
	}).Error; err != nil {
		t.Fatalf("seed sender: %v", err)
	}

	// require_email_verification defaults to on; the seeded sender makes
	// mail.ready answer true, so registration proceeds past the 503 gate.
	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fresh@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	var body registerBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode register response: %v (%s)", err, rec.Body.String())
	}
	if !body.VerificationRequired {
		t.Fatalf("register with gate on: verificationRequired = false (%s)", rec.Body.String())
	}

	// The send happens synchronously inside register, before the org/membership
	// transaction — the relay must have received the verification mail anyway.
	mailed := got()
	if !strings.Contains(mailed, "Verify your email address for octarq") {
		t.Fatalf("verification email never reached the relay — the org-less send is broken:\n%s", mailed)
	}
	if !strings.Contains(mailed, "/api/auth/verify-email?token=") {
		t.Fatalf("verification email carries no verify link:\n%s", mailed)
	}
}
