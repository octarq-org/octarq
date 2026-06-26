package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

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
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", m.Subject)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	if m.HTML != "" {
		fmt.Fprintf(&b, "Content-Type: text/html; charset=UTF-8\r\n\r\n%s", m.HTML)
	} else {
		fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n\r\n%s", m.Text)
	}
	addr := s.host + ":" + s.port
	auth := smtp.PlainAuth("", s.user, s.pass, s.host)
	return smtp.SendMail(addr, auth, from, m.To, []byte(b.String()))
}
