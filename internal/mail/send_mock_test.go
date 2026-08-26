package mail

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin/safehttp"
)

type mockSMTPOptions struct {
	authRequired bool
	validUser    string
	validPass    string
	rejectRcpt   string
	rejectMail   bool
	rejectData   bool
	failAuth     bool
}

func startMockSMTPServer(t *testing.T, opts mockSMTPOptions) (port string, getReceived func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var receivedData []string
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
		_, _ = fmt.Fprint(w, "220 mock.octarq.local ESMTP\r\n")
		_ = w.Flush()

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			fields := strings.Fields(trimmed)
			if len(fields) == 0 {
				continue
			}
			verb := strings.ToUpper(fields[0])
			switch verb {
			case "EHLO", "HELO":
				_, _ = fmt.Fprint(w, "250-mock.octarq.local\r\n")
				if opts.authRequired || opts.failAuth {
					_, _ = fmt.Fprint(w, "250-AUTH PLAIN\r\n")
				}
				_, _ = fmt.Fprint(w, "250 8BITMIME\r\n")
			case "AUTH":
				if opts.failAuth {
					_, _ = fmt.Fprint(w, "535 5.7.8 Authentication failed\r\n")
				} else if len(fields) >= 2 && fields[1] == "PLAIN" {
					var rawAuth []byte
					if len(fields) >= 3 {
						rawAuth, _ = base64.StdEncoding.DecodeString(fields[2])
					}
					parts := strings.Split(string(rawAuth), "\x00")
					if len(parts) == 3 && (opts.validUser == "" || (parts[1] == opts.validUser && parts[2] == opts.validPass)) {
						_, _ = fmt.Fprint(w, "235 2.7.0 Authentication successful\r\n")
					} else {
						_, _ = fmt.Fprint(w, "535 5.7.8 Authentication failed\r\n")
					}
				} else {
					_, _ = fmt.Fprint(w, "504 Unrecognized authentication type\r\n")
				}
			case "MAIL":
				if opts.rejectMail {
					_, _ = fmt.Fprint(w, "550 Sender rejected\r\n")
				} else {
					_, _ = fmt.Fprint(w, "250 2.1.0 Ok\r\n")
				}
			case "RCPT":
				if opts.rejectRcpt != "" && strings.Contains(trimmed, opts.rejectRcpt) {
					_, _ = fmt.Fprint(w, "550 User not found\r\n")
				} else {
					_, _ = fmt.Fprint(w, "250 2.1.5 Ok\r\n")
				}
			case "DATA":
				if opts.rejectData {
					_, _ = fmt.Fprint(w, "554 Transaction failed\r\n")
				} else {
					_, _ = fmt.Fprint(w, "354 End data with <CR><LF>.<CR><LF>\r\n")
					_ = w.Flush()
					var dataBuilder strings.Builder
					for {
						dataLine, err := r.ReadString('\n')
						if err != nil {
							return
						}
						if dataLine == ".\r\n" || dataLine == ".\n" {
							break
						}
						dataBuilder.WriteString(dataLine)
					}
					receivedData = append(receivedData, dataBuilder.String())
					_, _ = fmt.Fprint(w, "250 2.0.0 Ok: queued\r\n")
				}
			case "QUIT":
				_, _ = fmt.Fprint(w, "221 2.0.0 Bye\r\n")
				_ = w.Flush()
				return
			default:
				_, _ = fmt.Fprint(w, "250 Ok\r\n")
			}
			_ = w.Flush()
		}
	}()

	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, func() []string { <-done; return receivedData }
}

func TestSendSuccess(t *testing.T) {
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	port, getReceived := startMockSMTPServer(t, mockSMTPOptions{
		authRequired: true,
		validUser:    "alice",
		validPass:    "secret",
	})

	sender := NewCustomSender("127.0.0.1", port, "alice", "secret", "default@example.com")
	msg := Message{
		To:      []string{"bob@example.com", "carol@example.com"},
		Subject: "Test Email with UTF-8: 🚀",
		Text:    "Hello plaintext content",
	}

	err := sender.Send(msg)
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	received := getReceived()
	if len(received) != 1 {
		t.Fatalf("expected 1 received message, got %d", len(received))
	}
	body := received[0]
	if !strings.Contains(body, "From: default@example.com") {
		t.Errorf("expected default sender in From header, got: %s", body)
	}
	if !strings.Contains(body, "To: bob@example.com, carol@example.com") {
		t.Errorf("expected To header, got: %s", body)
	}
	if !strings.Contains(body, "Content-Type: text/plain; charset=UTF-8") {
		t.Errorf("expected text/plain content type, got: %s", body)
	}
	if !strings.Contains(body, "Hello plaintext content") {
		t.Errorf("expected text body, got: %s", body)
	}
}

func TestSendHTMLSuccessNoAuth(t *testing.T) {
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	port, getReceived := startMockSMTPServer(t, mockSMTPOptions{})

	sender := NewCustomSender("127.0.0.1", port, "", "", "default@example.com")
	msg := Message{
		From:    "custom@example.com",
		To:      []string{"bob@example.com"},
		Subject: "HTML Test",
		HTML:    "<h1>Hello HTML</h1>",
	}

	err := sender.Send(msg)
	if err != nil {
		t.Fatalf("unexpected Send error: %v", err)
	}

	received := getReceived()
	if len(received) != 1 {
		t.Fatalf("expected 1 received message, got %d", len(received))
	}
	body := received[0]
	if !strings.Contains(body, "From: custom@example.com") {
		t.Errorf("expected custom sender in From header, got: %s", body)
	}
	if !strings.Contains(body, "Content-Type: text/html; charset=UTF-8") {
		t.Errorf("expected text/html content type, got: %s", body)
	}
	if !strings.Contains(body, "<h1>Hello HTML</h1>") {
		t.Errorf("expected HTML body, got: %s", body)
	}
}

func TestSendErrors(t *testing.T) {
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	t.Run("auth failure", func(t *testing.T) {
		port, _ := startMockSMTPServer(t, mockSMTPOptions{failAuth: true})
		sender := NewCustomSender("127.0.0.1", port, "alice", "wrong", "default@example.com")
		err := sender.Send(Message{To: []string{"bob@example.com"}, Subject: "s", Text: "t"})
		if err == nil {
			t.Fatal("expected auth error, got nil")
			return
		}
	})

	t.Run("mail from rejected", func(t *testing.T) {
		port, _ := startMockSMTPServer(t, mockSMTPOptions{rejectMail: true})
		sender := NewCustomSender("127.0.0.1", port, "", "", "default@example.com")
		err := sender.Send(Message{To: []string{"bob@example.com"}, Subject: "s", Text: "t"})
		if err == nil {
			t.Fatal("expected mail from rejection error, got nil")
			return
		}
	})

	t.Run("rcpt to rejected", func(t *testing.T) {
		port, _ := startMockSMTPServer(t, mockSMTPOptions{rejectRcpt: "bad@example.com"})
		sender := NewCustomSender("127.0.0.1", port, "", "", "default@example.com")
		err := sender.Send(Message{To: []string{"bad@example.com"}, Subject: "s", Text: "t"})
		if err == nil {
			t.Fatal("expected rcpt rejection error, got nil")
			return
		}
	})

	t.Run("data rejected", func(t *testing.T) {
		port, _ := startMockSMTPServer(t, mockSMTPOptions{rejectData: true})
		sender := NewCustomSender("127.0.0.1", port, "", "", "default@example.com")
		err := sender.Send(Message{To: []string{"bob@example.com"}, Subject: "s", Text: "t"})
		if err == nil {
			t.Fatal("expected data rejection error, got nil")
			return
		}
	})
}
