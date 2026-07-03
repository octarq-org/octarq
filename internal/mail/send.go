package mail

import (
	"fmt"
	"mime"
	"net/smtp"
	"strings"
)

// stripCRLF removes CR/LF (and NUL) from a header value to prevent SMTP header
// injection: an attacker-controlled To/From/Subject containing "\r\n" could
// otherwise inject arbitrary headers (Bcc:, forged Content-Type) or a body.
func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "", "\x00", "").Replace(s)
}

// Message is an outbound email.
type Message struct {
	From    string
	To      []string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers outbound mail. Implemented today by an SMTP relay.
type Sender interface {
	Send(m Message) error
}

// SMTPSender relays through a configured SMTP server.
type SMTPSender struct {
	host, port, user, pass, from string
}

// NewCustomSender builds a Sender from explicit credentials.
func NewCustomSender(host, port, user, pass, from string) Sender {
	return &SMTPSender{
		host: host, port: port,
		user: user, pass: pass, from: from,
	}
}

func (s *SMTPSender) Send(m Message) error {
	from := m.From
	if from == "" {
		from = s.from
	}
	to := make([]string, len(m.To))
	for i, addr := range m.To {
		to[i] = stripCRLF(addr)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", stripCRLF(from))
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	// QEncoding both encodes non-ASCII subjects and neutralizes any CR/LF.
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	if m.HTML != "" {
		fmt.Fprintf(&b, "Content-Type: text/html; charset=UTF-8\r\n\r\n%s", m.HTML)
	} else {
		fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n\r\n%s", m.Text)
	}
	addr := s.host + ":" + s.port
	auth := smtp.PlainAuth("", s.user, s.pass, s.host)
	return smtp.SendMail(addr, auth, stripCRLF(from), to, []byte(b.String()))
}
