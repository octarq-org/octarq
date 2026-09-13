package mail

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseDuplicatePlainText(t *testing.T) {
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@example.com
Subject: dup plain test
Date: Tue, 18 Aug 2026 12:00:00 +0000
Message-ID: <dup-plain@example.com>
Content-Type: multipart/alternative; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: text/plain; charset=utf-8

first plain text
--b1
Content-Type: text/plain; charset=utf-8

second plain text
--b1--
`, "\n", "\r\n")

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(p.Text, "first plain text") {
		t.Errorf("expected p.Text to contain 'first plain text', got %q", p.Text)
	}
	if strings.Contains(p.Text, "second plain text") {
		t.Errorf("expected second plain text to be ignored, got %q", p.Text)
	}
	if p.PartErrors == 0 {
		t.Errorf("expected PartErrors > 0 for duplicate plain text, got %d", p.PartErrors)
	}
	if p.ReceivedAt.IsZero() || p.ReceivedAt.Year() != 2026 {
		t.Errorf("expected parsed ReceivedAt date 2026, got %v", p.ReceivedAt)
	}
}

func TestParseMaxPartsLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("From: sender@example.com\r\n")
	b.WriteString("To: recv@example.com\r\n")
	b.WriteString("Subject: many parts test\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"b1\"\r\n")
	b.WriteString("MIME-Version: 1.0\r\n\r\n")

	for i := 0; i < 205; i++ {
		b.WriteString("--b1\r\n")
		b.WriteString("Content-Type: text/plain\r\n\r\n")
		fmt.Fprintf(&b, "part %d\r\n", i)
	}
	b.WriteString("--b1--\r\n")

	p, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.MaxPartsReached {
		t.Errorf("expected MaxPartsReached == true")
	}
	if p.PartErrors == 0 {
		t.Errorf("expected PartErrors > 0")
	}
}

func TestParseInlineAttachmentWithContentTypeName(t *testing.T) {
	raw := strings.ReplaceAll(`From: sender@example.com
To: recv@example.com
Subject: inline name test
Content-Type: multipart/related; boundary="b1"
MIME-Version: 1.0

--b1
Content-Type: text/plain; charset=utf-8

body text
--b1
Content-Type: image/jpeg; name="photo.jpg"
Content-ID: <photo123>

fake-jpg-data
--b1
Content-Type: application/pdf
Content-Disposition: inline; filename="document.pdf"
Content-ID: <doc456>

fake-pdf-data
--b1--
`, "\n", "\r\n")

	p, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(p.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(p.Attachments))
	}
	if p.Attachments[0].Filename != "photo.jpg" || p.Attachments[0].ContentID != "photo123" {
		t.Errorf("attachment 0 mismatch: %+v", p.Attachments[0])
	}
	if p.Attachments[1].Filename != "document.pdf" || p.Attachments[1].ContentID != "doc456" {
		t.Errorf("attachment 1 mismatch: %+v", p.Attachments[1])
	}
}
