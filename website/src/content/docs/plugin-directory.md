---
title: Plugin Directory
description: Official and community plugins available for Octarq.
---

Explore official and reference plugins available for Octarq from the [octarq-plugins](https://github.com/octarq-org/octarq-plugins) repository.

## Official & Reference Plugins

### 1. 🤖 Telegram (`octarq-plugin-telegram`)
- **Category**: Integration / Notifications
- **Description**: Connects your Octarq instance with Telegram bots. Send instant notification alerts, receive system status updates, and trigger automated tasks directly from Telegram chat commands.
- **Repository**: [octarq-org/octarq-plugin-telegram](https://github.com/octarq-org/octarq-plugins)

---

### 2. 🪝 Webhook (`octarq-plugin-webhook`)
- **Category**: Integration / Webhooks
- **Description**: Outbound webhook delivery system for event-driven workflows. Features exponential backoff retries, secret signature verification (HMAC-SHA256), payload customization, and event filtering.
- **Repository**: [octarq-org/octarq-plugin-webhook](https://github.com/octarq-org/octarq-plugins)

---

### 3. ✉️ Mail Links (`octarq-plugin-maillink`)
- **Category**: Agent-Native / Automation
- **Description**: An agent-native reference plugin that intercepts incoming OTP and magic link emails via `OnEmail` hooks, automatically generates short links using Octarq's links service, and exposes them as tools over the MCP server for Claude Code and Cursor to consume.
- **Repository**: [octarq-org/octarq-plugins](https://github.com/octarq-org/octarq-plugins)

---

## 🛠 Starter Template

Want to create your own plugin? Start with the official plugin template:

- **`_template`**: Pre-configured Go module + React TS frontend workspace with `@octarq/plugin-sdk` ready for instant copy-and-edit development.
