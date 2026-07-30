package mail

import (
	"net"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/internal/safehttp"
)

func TestSendBlocksNonPublicRelayAtDial(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	msg := Message{
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "Test",
		Text:    "Test Body",
	}

	// 1. Test literal IP 127.0.0.1
	senderIP := NewCustomSender("127.0.0.1", portStr, "", "", "sender@example.com")
	errIP := senderIP.Send(msg)
	if errIP == nil || !strings.Contains(errIP.Error(), "ssrf guard") {
		t.Errorf("expected ssrf guard error for 127.0.0.1, got: %v", errIP)
	}

	// 2. Test hostname localhost
	senderHost := NewCustomSender("localhost", portStr, "", "", "sender@example.com")
	errHost := senderHost.Send(msg)
	if errHost == nil || !strings.Contains(errHost.Error(), "ssrf guard") {
		t.Errorf("expected ssrf guard error for localhost, got: %v", errHost)
	}

	// 3. Test SetAllowPrivateSMTP(true)
	safehttp.SetAllowPrivateSMTP(true)
	defer safehttp.SetAllowPrivateSMTP(false)

	errAllowed := senderIP.Send(msg)
	if errAllowed != nil && strings.Contains(errAllowed.Error(), "ssrf guard") {
		t.Errorf("expected send NOT to be blocked by ssrf guard when SetAllowPrivateSMTP is true, got: %v", errAllowed)
	}
}
