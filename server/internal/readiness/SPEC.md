# Engineering Specification — internal/readiness

## 1. Overview
`internal/readiness` evaluates an Octarq instance's operational readiness and capability health into a structured report. It powers startup diagnostics and the `/api/instance/readiness` health API.

## 2. Capabilities Evaluated
- `public-origin`: Validates whether custom domains are registered to ensure secure password reset and invite links.
- `outbound-mail`: Validates whether the system SMTP sender or `mail.send` SPI service is ready.
- `registration`: Detects sign-up dead ends when email verification is mandated but no mail sender is configured (`StatusBlocked`).
- `database`: Reports database driver and sanitized DSN (redacting non-SQLite credentials).
- `hardening`: Reports development mode leniency vs production secret-key floor enforcement.

## 3. Security Invariants
- **Zero Secret Leakage**: Passwords, tokens, and secret keys are never included in check details or logs.
- **Fail-Closed Contradiction Detection**: If email verification is demanded without an active mail sender, registration status strictly transitions to `blocked`.

## 4. Verification
- Validated via `readiness_test.go` and `readiness_characterization_test.go`.
