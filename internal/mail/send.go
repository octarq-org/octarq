package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/octarq-org/octarq/plugin/safehttp"
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
	addr := net.JoinHostPort(s.host, s.port)
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: safehttp.SMTPControl,
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: s.host}
		if err := c.StartTLS(config); err != nil {
			return err
		}
	}

	// Fail closed when credentials are configured but the server does not offer
	// AUTH, matching net/smtp.SendMail. Skipping auth instead would silently
	// deliver unauthenticated whenever a relay stops advertising the capability
	// — including when something on the path strips it.
	if s.user != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		auth := smtp.PlainAuth("", s.user, s.pass, s.host)
		if err := c.Auth(auth); err != nil {
			return err
		}
	}

	if err := c.Mail(stripCRLF(from)); err != nil {
		return err
	}
	for _, rec := range to {
		if err := c.Rcpt(rec); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(b.String()))
	if err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
