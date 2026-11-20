package api

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/plugin/safehttp"
)

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

func TestVerificationEmailDeliveredWithoutOrgMembership(t *testing.T) {
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	h, srv, db := newTestHandlerRaw(t)
	host, port, got := captureSMTP(t)

	cipher := crypto.New("secret")
	if err := cipher.EnableEnvelope(apiEnvStore{db}); err != nil {
		t.Fatalf("enable envelope: %v", err)
	}
	enc, err := cipher.Encrypt([]byte("pw"))
	if err != nil {
		t.Fatalf("encrypt sender pass: %v", err)
	}
	sender := testSMTPSender{
		OrgID: 1, Name: "relay", Host: host, Port: port,
		User: "u", Pass: string(enc), FromEmail: "noreply@example.com",
	}
	if err := db.Create(&sender).Error; err != nil {
		t.Fatalf("seed sender: %v", err)
	}
	if err := h.setSetting(keySystemSenderID, strconv.FormatUint(uint64(sender.ID), 10)); err != nil {
		t.Fatalf("set system sender: %v", err)
	}

	rec := do(srv, "POST", "/api/auth/register", nil, `{"email":"fresh@user.com","password":"hunter2pw"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("register: got %d (%s)", rec.Code, rec.Body.String())
	}
	var body registerBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode register response: %v (%s)", err, rec.Body.String())
	}
	if !body.OK {
		t.Fatalf("register did not report success (%s)", rec.Body.String())
	}

	mailed := got()
	if !strings.Contains(mailed, "Confirm your email address for Octarq") {
		t.Fatalf("verification email never reached the relay — the org-less send is broken:\n%s", mailed)
	}
	if !strings.Contains(mailed, "/api/auth/verify-email?token=") {
		t.Fatalf("verification email carries no verify link:\n%s", mailed)
	}
}
