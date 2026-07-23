---
title: Notifications
description: Telegram and webhook alerts, managed from the dashboard.
---

octarq can push alerts to **Telegram** or a **webhook** when something happens — most
commonly when new mail arrives. Channels are configured entirely from the
dashboard; no redeploy needed.

## Channels

| Channel | Setup |
| --- | --- |
| **Telegram** | Add a bot token and chat ID. |
| **Webhook** | Add a URL; octarq POSTs a JSON payload on each event. |

## What triggers a notification

- **New mail** in any mailbox.
- **Server monitoring** events (up / down alerts).
- **Transaction & financial** updates.
- **AI briefings** and OTP extraction events from AI notifier plugins.
