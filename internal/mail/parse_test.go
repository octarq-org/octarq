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

func TestParseAuthResults(t *testing.T) {
	cases := []struct {
		hdr              string
		spf, dkim, dmarc string
	}{
		{
			"mx.example.com; spf=pass smtp.mailfrom=x@example.com; dkim=pass header.i=@example.com; dmarc=pass",
			"pass", "pass", "pass",
		},
		{
			"mx.example.com; spf=fail; dkim=none; dmarc=fail",
			"fail", "none", "fail",
		},
		{
			"mx.example.com; spf=softfail (reason); dkim=temperror",
			"softfail", "temperror", "",
		},
		{
			"mx.example.com; x-not-spf=pass; x-not-dkim=fail; x-not-dmarc=pass",
			"", "", "",
		},
		{
			"no-auth-results-here",
			"", "", "",
		},
	}
	for _, c := range cases {
		var got AuthResults
		parseAuthResults(c.hdr, &got)
		if got.SPF != c.spf {
			t.Errorf("SPF: got %q, want %q (hdr: %q)", got.SPF, c.spf, c.hdr)
		}
		if got.DKIM != c.dkim {
			t.Errorf("DKIM: got %q, want %q (hdr: %q)", got.DKIM, c.dkim, c.hdr)
		}
		if got.DMARC != c.dmarc {
			t.Errorf("DMARC: got %q, want %q (hdr: %q)", got.DMARC, c.dmarc, c.hdr)
		}
	}
}

func TestParseEmailWithAuthResults(t *testing.T) {
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@test.net
Subject: auth test
Authentication-Results: mx.example.com;
 spf=pass smtp.mailfrom=example.com;
 dkim=pass header.i=@example.com;
 dmarc=pass
Message-Id: <auth123@example.com>

body
`, "\n", "\r\n")
	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Auth.SPF != "pass" {
		t.Errorf("SPF = %q, want pass", p.Auth.SPF)
	}
	if p.Auth.DKIM != "pass" {
		t.Errorf("DKIM = %q, want pass", p.Auth.DKIM)
	}
	if p.Auth.DMARC != "pass" {
		t.Errorf("DMARC = %q, want pass", p.Auth.DMARC)
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
