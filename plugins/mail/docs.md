Octarq includes an integrated email management engine designed for solo developers and teams. It processes inbound transactional messages, provides raw EML inbox viewing, handles automatic Catch-All mailbox creation, and dispatches outbound transactional emails via custom SMTP senders.

## Key Capabilities

- **Inbound Webhook Email Receiving**: Receive emails directly from Cloudflare Email Workers or HTTP webhook relays without operating a complex mail server daemon.
- **Automatic Catch-All Routing**: Automatically provision inboxes on-the-fly whenever a message arrives for an unconfigured local address under your domain.
- **Reserved Mailbox Protection**: Define reserved local-part prefixes (e.g., `admin`, `postmaster`) to exclude system addresses from catch-all auto-creation.
- **Email Authentication Verification**: Inspect incoming message authentication status (SPF, DKIM, DMARC results) directly in the message viewer.
- **Raw EML & Attachment Inspection**: Preview formatted HTML/Plaintext body text, download raw `.eml` message files, and inspect email attachments.
- **Custom Outbound SMTP Senders**: Configure one or multiple SMTP senders with AES-GCM encrypted credential storage for sending outbound emails.
- **System Event Notifications**: Trigger instant webhooks or Telegram alerts whenever an email is delivered or an outbound SMTP dispatch fails.

## How to Use

### 1. Connecting Inbound Email via Cloudflare Worker
1. Go to **Settings $\rightarrow$ Workspace** to find your unique **Inbound Webhook URL** (`/api/webhook/{orgSlug}/email/inbound/{token}`).
2. In your Cloudflare Dashboard under **Email Routing $\rightarrow$ Email Workers**, deploy a worker script that POSTs incoming raw emails to this endpoint.
3. Once configured, inbound emails arriving at your custom domain appear instantly in the Octarq Mail UI.

### 2. Setting Up Catch-All Mailboxes
1. Navigate to **Settings $\rightarrow$ Workspace**.
2. Toggle **Enable Catch-All Routing**.
3. (Optional) In **Reserved Mailbox Prefixes**, enter comma-separated prefixes (e.g., `admin, support, security`) to prevent catch-all from capturing reserved system addresses.

### 3. Adding an Outbound SMTP Sender
1. Navigate to **Mail $\rightarrow$ Settings $\rightarrow$ SMTP Senders**.
2. Click **Add SMTP Sender**.
3. Enter your SMTP Host (e.g., `smtp.mailgun.org`), Port (e.g., `587`), Username, Password, and default From Email address (e.g., `noreply@example.com`).
4. Click **Save**. The password is encrypted at rest using AES-GCM and never exposed via API endpoints.

:::note
Outbound transactional emails (such as workspace member invitations, password resets, and email verification links) automatically utilize the first configured SMTP sender in your workspace.
:::

## Why It Matters

Instead of paying for expensive per-seat Google Workspace or Microsoft 365 inboxes for every domain alias, Octarq allows you to receive and inspect unlimited custom domain emails centrally while sending outbound transactional mail cleanly through dedicated SMTP relays.

## Related Links

- [DNS Management](/help/dns) — Verify SPF, DKIM, and DMARC records for your mail domains.
- [Notification Channels](/help/notifications) — Get notified instantly on Telegram when new emails arrive.
- [Webhooks](/help/webhooks) — Stream `email.receive` events to your downstream services.
