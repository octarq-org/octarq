# SPEC: Low-level RFC822 Parser & SMTP Client (`server/internal/mail`)

## 1. Overview & Architecture
`server/internal/mail` provides low-level RFC822 MIME parsing and outbound SMTP delivery primitives for Octarq Core.
- **Inbound MIME Parser**: Extracts headers, text/plain, text/html, attachments, and authentication headers (`Authentication-Results` SPF, DKIM, DMARC) with bounded memory limits (`maxPartBytes = 10 MiB`, `maxParts = 200`).
- **Outbound SMTP Sender**: Delivers outbound messages using explicit or environment credentials, with STARTTLS support, CRLF header injection defenses (`stripCRLF`), and fail-closed authentication checking.

## 2. Exported Contracts & Types
- `Parse(raw []byte) (*Parsed, error)`: Parses raw RFC822 bytes into normalized `Parsed` structure. If parsing fails, returns a fallback `Parsed` with raw payload rather than dropping the message.
- `Parsed`: Contains `MessageID`, `From`, `To`, `Subject`, `Text`, `HTML`, `Attachments`, `ReceivedAt`, `Raw`, `Auth` (SPF/DKIM/DMARC), `PartErrors`, and `MaxPartsReached`.
- `Sender`: Interface declaring `Send(m Message) error`.
- `SMTPSender`: Concrete implementation relaying through an SMTP server.
- `NewCustomSender(host, port, user, pass, from string) Sender`: Constructor for `SMTPSender`.

## 3. Security & Invariant Guarantees
- **Memory Denial of Service Protection**: Unbounded MIME parts are capped at `maxPartBytes` (10 MiB) via `io.LimitReader` and walked at most `maxParts` (200) times.
- **Header Injection Defenses**: `stripCRLF` neutralizes `\r`, `\n`, and `\x00` in `To`, `From`, and `Subject` fields to prevent arbitrary SMTP header injection.
- **Fail-Closed Authentication**: If credentials are provided but the remote SMTP server does not advertise `AUTH`, delivery fails closed immediately (`smtp: server doesn't support AUTH`) rather than sending unauthenticated mail.
- **Connection Timeout**: Network connections enforce strict deadlines (`SetDeadline`) to prevent hanging file descriptors.

## 4. File Layout
- `parse.go`: RFC822 parsing, charset decoding, memory limiting, and attachment extraction.
- `send.go`: SMTP sender, TLS negotiation, authentication, and message formatting.
- `mail_characterization_test.go`: Characterization test locking contracts and fail-closed behaviors.
- `parse_test.go`, `parse_more_test.go`: Inbound parser tests.
- `send_test.go`, `send_auth_test.go`, `send_mock_test.go`, `send_ssrf_test.go`: SMTP sender tests.
