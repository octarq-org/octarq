package mail

import (
	"strings"
	"testing"
)

func TestParseMultipart(t *testing.T) {
	raw := strings.ReplaceAll(`From: Alice <alice@example.com>
To: Bob <bob@example.net>
Subject: Hello there
Message-Id: <abc123@example.com>
Content-Type: multipart/alternative; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: text/plain; charset=utf-8

plain body
--b1
Content-Type: text/html; charset=utf-8

<p>html body</p>
--b1--
`, "\n", "\r\n")

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.From != "alice@example.com" {
		t.Errorf("From = %q", p.From)
	}
	if p.To != "bob@example.net" {
		t.Errorf("To = %q", p.To)
	}
	if p.Subject != "Hello there" {
		t.Errorf("Subject = %q", p.Subject)
	}
	if strings.TrimSpace(p.Text) != "plain body" {
		t.Errorf("Text = %q", p.Text)
	}
	if !strings.Contains(p.HTML, "html body") {
		t.Errorf("HTML = %q", p.HTML)
	}
}

func TestParseUnparseableDoesNotPanic(t *testing.T) {
	p, err := Parse([]byte("this is not a valid email at all \x00\x01"))
	if err != nil {
		t.Fatalf("Parse returned error for garbage input: %v", err)
	}
	if p == nil {
		t.Fatal("Parse returned nil for garbage input")
	}
	if len(p.Raw) == 0 {
		t.Error("expected raw bytes preserved on unparseable input")
	}
}
