# Workflow: Instant OTP & Verification Code Retrieval

## Objective
Enable the AI agent to instantly retrieve transactional verification codes (One-Time Passwords / OTPs), 2FA tokens, and onboarding confirmation links from inbound emails so the founder never has to break flow or open an email client.

## Trigger Prompt Examples
- *"I just requested a verification code from Stripe / GitHub, what is the code?"*
- *"Extract the OTP code from the latest email sent to auth@mydomain.com"*
- *"What is my login code?"*

## Execution Protocol

### Step 1: Fetch Latest Transactional Emails
Call `list_emails` with:
- `limit`: 5
- `unreadOnly`: false (in case the email was just marked read by a background rule)
- `mailboxId`: (optional, if target mailbox is specified)

### Step 2: Parse Sender & Subject Pattern
Scan the returned email list starting with the most recent:
1. **Target Identification**: Check `from` and `subject` for terms like:
   - "verification code", "verify your email", "one-time passcode", "OTP", "login code", "confirm email"
   - Service names: "Stripe", "GitHub", "Cloudflare", "OpenAI", "AWS", "Google", etc.
2. **Token Extraction**:
   - Extract numeric codes (typically 4, 6, or 8 consecutive digits, e.g. `\b\d{4,8}\b`).
   - Extract alphanumeric codes if formatted specifically (e.g. `ABC-123456` or `[A-Z0-9]{6,8}`).

### Step 3: Format Direct Output

Respond concisely so the code can be copied in one click:

```markdown
🔑 **Verification Code**: `123456`

- **Service**: [Service Name / Sender]
- **Subject**: [Subject Line]
- **To**: [Recipient Address]
- **Received**: [Relative Time or Timestamp]
```
