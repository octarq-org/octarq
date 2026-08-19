package mail

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/plugin/safehttp"
)

// fakeSMTPServer speaks just enough ESMTP to complete the greeting and EHLO,
// advertising only the extensions in caps. It records how far the client got.
func fakeSMTPServer(t *testing.T, caps []string) (port string, reached func() []string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	var seen []string
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
		w.WriteString("220 fake ESMTP\r\n")
		w.Flush()
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			verb := strings.ToUpper(strings.Fields(line)[0])
			seen = append(seen, verb)
			switch verb {
			case "EHLO":
				w.WriteString("250-fake greets you\r\n")
				for i, c := range caps {
					if i == len(caps)-1 {
						w.WriteString("250 " + c + "\r\n")
					} else {
						w.WriteString("250-" + c + "\r\n")
					}
				}
				if len(caps) == 0 {
					w.WriteString("250 SIZE 1000000\r\n")
				}
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

	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, func() []string { <-done; return seen }
}

// TestSendFailsClosedWhenServerDropsAUTH pins the fail-closed half of the send
// path. net/smtp.SendMail errors when credentials are configured but the server
// advertises no AUTH; delivering anyway would mean that a relay which stops
// offering AUTH — or anything on the path that strips the capability — silently
// receives the mail unauthenticated.
func TestSendFailsClosedWhenServerDropsAUTH(t *testing.T) {
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	port, reached := fakeSMTPServer(t, nil) // no AUTH advertised
	msg := Message{From: "a@example.com", To: []string{"b@example.com"}, Subject: "s", Text: "t"}

	err := NewCustomSender("127.0.0.1", port, "user", "pass", "a@example.com").Send(msg)
	if err == nil {
		t.Fatal("sending with credentials to a server offering no AUTH was allowed")
	}
	if !strings.Contains(err.Error(), "doesn't support AUTH") {
		t.Errorf("expected a fail-closed AUTH error, got %v", err)
	}
	for _, verb := range reached() {
		if verb == "MAIL" || verb == "DATA" {
			t.Errorf("the message was handed over anyway (reached %s)", verb)
		}
	}
}
