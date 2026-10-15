---
title: Mailboxes
description: Receive inbound emails via webhooks, route messages to custom addresses, and configure outbound SMTP senders.
category: operations
order: 20
---

Octarq Mailboxes gives your workspace dedicated email addresses on your custom domains. You can receive transactional incoming mail via webhooks, route unassigned addresses with catch-all rules, and send outbound messages through external SMTP relays.

:::note
Octarq receives mail through webhooks and delivers outgoing mail through external SMTP servers. It does not run an IMAP or POP3 mail server.
:::

## Create a Mailbox

1. Go to **Operations → Messaging → Mail** and open the **Mailboxes** tab (or press `⌘K` / `Ctrl+K` and search "Mail").
2. Click **New Mailbox**.
3. Select a verified domain from [DNS Management](/help/dns) and enter an address prefix (such as `support`).
4. Click **Create Mailbox**.

Inbound messages sent to this address appear directly in this mailbox.

## Route Incoming Email with Catch-All

Catch-all routing captures messages sent to addresses you have not explicitly created:

1. Go to **Operations → Messaging → Mail** and open the **Settings** tab.
2. Turn on **Enable Catch-All Routing**.
3. (Optional) In **Reserved Mailbox Prefixes**, enter comma-separated prefixes (such as `admin, postmaster, abuse, security`) that bypass auto-creation.
4. Click **Save Changes**.

When catch-all is active, messages sent to unconfigured addresses on your domain automatically create a mailbox tagged as `auto (catch-all)`. Reserved prefixes are excluded to protect critical addresses.

## Receive Inbound Email

Octarq receives inbound messages through HTTPS webhooks from edge email services.

### Set Up an Inbound Webhook

1. Go to **Operations → Messaging → Mail** and open the **Settings** tab.
2. Copy your workspace **Inbound Webhook URL** under **Inbound Configuration**.
3. Configure your mail provider (such as Cloudflare Email Routing, SendGrid, or Mailgun) to POST incoming messages to this URL.

Requests are authenticated using a secret token in the URL path.

### Message Limits

Octarq processes incoming messages with the following limits:

- **Message size**: Up to 25 MiB total per email.
- **Attachments and parts**: Up to 10 MiB per file, with up to 200 parts per message.
- **Error resilience**: If a message has malformed headers or corrupt boundaries, Octarq saves it with the subject `(unparseable message)` so data is never lost.

## Configure Outbound SMTP Senders

To send transactional emails or invitations from your workspace:

1. Go to **Operations → Messaging → Mail** and open the **Settings** tab.
2. Under **SMTP Gateways**, click **Add SMTP Sender**.
3. Enter your SMTP **Host**, **Port** (`587` or `465`), **Username**, **Password**, and default **From Email**.
4. Click **Save Sender**.

Credentials are encrypted at rest. Active SSRF protection blocks outbound connections to private networks, loopback addresses, and cloud metadata services.

## Test Connection and Handle Failures

After adding an SMTP sender, verify that your credentials work:

1. Find your sender under **SMTP Gateways**.
2. Click **Test Connection**.
3. Review the result. Octarq opens a connection to the server, runs a TLS handshake, and reports the status immediately.

If the test fails, check the following:

- **Authentication error**: Verify that the username, password, or API key are correct.
- **Connection timeout**: Check the host address and port number (usually `587` for STARTTLS or `465` for TLS).
- **Security rejection**: Outbound connections to private or internal IP addresses are blocked by SSRF protection.

## Manage Suppressions and Bounces

Octarq tracks delivery failures to protect your sender reputation:

- **Hard bounces and complaints**: When an email permanently bounces or receives a spam complaint, Octarq adds the recipient to the workspace suppression list automatically.
- **Soft bounces**: Temporary delivery issues (such as a full inbox) are logged in activity history and trigger alerts without suppressing the address.
- **Manual suppression**: Open the **Suppressions** tab under **Operations → Messaging → Mail** to add or remove addresses. Outbound messages to suppressed addresses are rejected before transmission.

## Auto-Wrap Outbound Links

To track clicks in outgoing emails without manually modifying URLs:

1. Go to **Operations → Messaging → Mail** and open the **Settings** tab.
2. Turn on **Auto-wrap links in outgoing mail**.
3. Click **Save Changes**.

External links in outgoing HTML emails are converted into tracked [Short Links](/help/short-links). Internal domain links and `mailto:` links remain unchanged.

:::caution This is a Pro feature
Octarq Core stores email bodies and attachments in the local database. With **Octarq Pro**, incoming messages and attachments can be stored directly in Amazon S3, Cloudflare R2, or MinIO object storage.
:::

## Related Guides

- [DNS Management](/help/dns)
- [Short Links](/help/short-links)
- [Alerts](/help/notifications)
- [API Tokens](/help/api-tokens)
