# Autonomous Solopreneur Blueprint — Claude Code Guide

This folder contains the **Autonomous Solopreneur** reference blueprint for driving Octarq over the Model Context Protocol (MCP) using Claude Code.

---

## 1. Connect Claude Code to Octarq Remote MCP

Octarq provides a native Server-Sent Events (SSE) MCP endpoint at `/api/mcp/sse`.

### Add the MCP server to Claude Code
Run the following command in your terminal:

```bash
# Using your workspace API token (get one from Octarq Dashboard -> Personal Settings -> API Tokens)
claude mcp add --transport sse octarq http://localhost:8080/api/mcp/sse --header "Authorization: Bearer oct_your_token_here"
```

Or when running against a remote production domain:
```bash
claude mcp add --transport sse octarq https://your-octarq-host.com/api/mcp/sse --header "Authorization: Bearer oct_your_token_here"
```

---

## 2. Available MCP Tools in Workspace

Once connected, Claude Code gains native access to Octarq tools:

| MCP Tool Name | Description | Common Use Case |
|---|---|---|
| `list_domains` | Lists registered domains and mail/link flags | Verify SPF, DKIM, DMARC, and routing health |
| `create_shortlink` | Creates declarative trackable short link with custom slug & host | Launch marketing campaigns with UTM tags |
| `list_links` | Queries workspace links, clicks, and status | Real-time traffic monitoring & attribution |
| `list_mailboxes` | Lists dedicated business mailboxes & unread counts | Monitor incoming SaaS confirmations & alerts |
| `list_emails` | Lists recently received emails & snippets | Extract OTP 2FA codes for automated signups |
| `export_data` | Exports workspace links, domains, and mail | Offline backups & audit trails |

---

## 3. Preset Slash Commands & Prompt Presets

You can issue the following preset slash commands to Claude Code inside this directory:

### `/check-dns [domain]`
> **Task**: Query Octarq's `list_domains` MCP tool, inspect SPF/DKIM/DMARC and MX records, and provide a deliverability reputation score (0-100).
>
> **Underlying Command**: `node scripts/step1-check-dns.mjs [domain]`

### `/add-link <target_url> [slug] [host]`
> **Task**: Create a branded shortlink on your custom subdomain, automatically appending standard UTM parameters (`utm_source`, `utm_medium`, `utm_campaign`, `utm_content`).
>
> **Underlying Command**: `node scripts/step2-create-shortlink.mjs <target_url> [slug] [host]`

### `/check-mail [mailbox_id]`
> **Task**: Poll the business mailbox using `list_mailboxes` and `list_emails`, detect unread SaaS confirmation emails, extract 6-digit OTP verification codes or magic links, and output structured JSON.
>
> **Underlying Command**: `node scripts/step3-mail-otp-listener.mjs`

### `/issue-license <buyer_email> <plan_tier> [seats]`
> **Task**: Mint a tamper-proof offline Ed25519 license token, verify its cryptographic validity locally without network roundtrips, and format a customer fulfillment notification email.
>
> **Underlying Command**: `node scripts/step4-issue-license.mjs <buyer_email> <plan_tier> [seats]`

### `/run-solopreneur`
> **Task**: Execute the entire unattended 4-step solopreneur pipeline sequentially.
>
> **Underlying Command**: `node scripts/run-all.mjs`

---

## 4. Example Conversational Prompts

You can also prompt Claude Code directly:

- *"Check my domain deliverability in Octarq and tell me if my SPF and DKIM are ready for cold outreach."*
- *"Create a short link on go.solopreneur.dev pointing to https://solopreneur.dev/pricing with campaign tags for ProductHunt."*
- *"Check if Stripe sent a verification code to ops@solopreneur.dev in the last 5 minutes and tell me the code."*
- *"A customer just purchased the lifetime license on Stripe. Generate an Ed25519 offline license key for customer@acme.com and draft their delivery email."*
