package mail

import (
	"encoding/json"
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

func TestParseInlineImage(t *testing.T) {
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@example.com
Subject: inline test
Message-ID: <inline@example.com>
Content-Type: multipart/related; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: text/html; charset=utf-8

<img src="cid:logo">
--b1
Content-Type: image/png
Content-Disposition: inline
Content-ID: <logo>

fake-png-data
--b1--
`, "\n", "\r\n")

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(p.Attachments))
	}
	att := p.Attachments[0]
	if !att.Inline {
		t.Errorf("expected Inline == true, got false")
	}
	if att.ContentID != "logo" {
		t.Errorf("expected ContentID == %q, got %q", "logo", att.ContentID)
	}
}

func TestParseBadPartDoesNotTruncate(t *testing.T) {
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@example.com
Subject: bad part test
Message-ID: <badpart@example.com>
Content-Type: multipart/mixed; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: text/plain; charset=utf-8

part1 text
--b1
MalformedHeaderLineWithoutColon
Content-Type: text/plain

part2 bad header
--b1
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="part3.txt"

part3 attachment
--b1--
`, "\n", "\r\n")

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Filename != "part3.txt" {
		t.Errorf("expected part3 attachment to be parsed despite part2 being malformed, got Attachments = %+v", p.Attachments)
	}
	if p.PartErrors == 0 {
		t.Errorf("expected PartErrors > 0, got %d", p.PartErrors)
	}
}

func TestParseTruncatedAttachment(t *testing.T) {
	extraBytes := 1024
	largeBody := strings.Repeat("A", maxPartBytes+extraBytes)
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@example.com
Subject: large attachment test
Message-ID: <large@example.com>
Content-Type: multipart/mixed; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="large.bin"

`, "\n", "\r\n") + largeBody + "\r\n--b1--\r\n"

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(p.Attachments))
	}
	att := p.Attachments[0]
	if !att.Truncated {
		t.Errorf("expected Truncated == true")
	}
	expectedSize := maxPartBytes + extraBytes
	if att.Size != expectedSize {
		t.Errorf("Size = %d, want exact total %d (not truncated fake value %d)", att.Size, expectedSize, maxPartBytes)
	}
}

func TestLegacyAttachmentJSONUnmarshal(t *testing.T) {
	jsonStr := `[{"filename":"a.pdf","contentType":"application/pdf","size":12}]`
	var atts []Attachment
	if err := json.Unmarshal([]byte(jsonStr), &atts); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	a := atts[0]
	if a.Filename != "a.pdf" || a.ContentType != "application/pdf" || a.Size != 12 {
		t.Errorf("fields mismatch: %+v", a)
	}
	if a.Inline || a.ContentID != "" || a.Truncated {
		t.Errorf("expected zero values for new fields, got Inline=%v ContentID=%q Truncated=%v", a.Inline, a.ContentID, a.Truncated)
	}
}

func TestParseDuplicateHTML(t *testing.T) {
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@example.com
Subject: dup html test
Message-ID: <dup@example.com>
Content-Type: multipart/alternative; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: text/html; charset=utf-8

<p>first html</p>
--b1
Content-Type: text/html; charset=utf-8

<p>second html</p>
--b1--
`, "\n", "\r\n")

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(p.HTML, "first html") {
		t.Errorf("expected p.HTML to contain 'first html', got %q", p.HTML)
	}
	if strings.Contains(p.HTML, "second html") {
		t.Errorf("expected second html to be ignored, got %q", p.HTML)
	}
	if p.PartErrors == 0 {
		t.Errorf("expected PartErrors > 0 for duplicate HTML, got %d", p.PartErrors)
	}
}
