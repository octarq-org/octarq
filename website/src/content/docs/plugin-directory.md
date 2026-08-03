---
title: Plugin Directory
description: Official and community plugins available for Octarq.
sidebar:
  order: 2
  group:
    label: "Build a Plugin"
---


Explore official and reference plugins available for Octarq from the [octarq-plugins](https://github.com/octarq-org/octarq-plugins) repository.

## Official & Reference Plugins

### 1. Telegram (`octarq-plugin-telegram`)
- **Category**: Integration / Notifications
- **Description**: Connects your Octarq instance with Telegram bots to send notification alerts and handle commands.
- **Repository**: [octarq-org/octarq-plugins/tree/main/telegram](https://github.com/octarq-org/octarq-plugins/tree/main/telegram)

---

### 2. Webhook (`octarq-plugin-webhook`)
- **Category**: Integration / Webhooks
- **Description**: Outbound webhook delivery system with retries, signature verification (HMAC-SHA256), and event filtering.
- **Repository**: [octarq-org/octarq-plugins/tree/main/webhook](https://github.com/octarq-org/octarq-plugins/tree/main/webhook)

---

### 3. Mail Links (`octarq-plugin-maillink`)
- **Category**: Agent-Native / Automation
- **Description**: Reference plugin that intercepts incoming OTP and magic link emails via `OnEmail` hooks, creates short links, and exposes them as tools over MCP for Claude Code and Cursor.
- **Repository**: [octarq-org/octarq-plugins/tree/main/maillink](https://github.com/octarq-org/octarq-plugins/tree/main/maillink)

---

## Starter Template

Want to create your own plugin? Start with the official plugin template:

- **`_template`**: Pre-configured Go module and React TS frontend workspace using `@octarq/plugin-sdk`.

