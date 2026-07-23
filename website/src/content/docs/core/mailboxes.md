---
title: Mailboxes
description: Receive mail on your own domains via Cloudflare Email Routing — no port 25, no MX ops.
---

Mailboxes give you addresses on your own domains without running an SMTP server.
octarq receives mail through **Cloudflare Email Routing**, so there's no port 25, MX,
or anti-spam operations to manage.

## Receiving mail (Cloudflare)

1. Enable **Email Routing** on your domain in Cloudflare.
2. Deploy [`deploy/cloudflare-email-worker.js`](https://github.com/octarq-org/octarq/blob/main/deploy/cloudflare-email-worker.js)
   as a Worker; set `OCTARQ_ENDPOINT` to the **Inbound Webhook URL** copied from
   Settings (it already embeds your org's inbound token).
3. Point a **catch-all** route at the Worker.

Mark a domain as **Accept email** in the dashboard. If **Catch-all** is enabled in
Settings, mail to any unknown address on that domain automatically creates a new
mailbox.

## Reading and replying

- Read messages with an **attachment list** and raw **`.eml`** download.
- **Reply** to a message, or **mark all read**.
- Send through one of **multiple configured SMTP relays**.

:::tip
Pair mailboxes with [Notification channels](/core/notifications/) to get a
Telegram or webhook ping the moment new mail lands — and with
Inbox AI to have each message summarized automatically.
:::
