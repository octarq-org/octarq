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

### 1. Short Links (`octarq-plugin-links`)
- **Category**: Links / Analytics
- **Description**: Short links, routing rules, QR codes, and click analytics.
- **Repository**: [octarq-org/octarq-plugins/tree/main/links](https://github.com/octarq-org/octarq-plugins/tree/main/links)

---

### 2. Mail (`octarq-plugin-mail`)
- **Category**: Email
- **Description**: Mailboxes, SMTP senders, inbound webhooks, and templates.
- **Repository**: [octarq-org/octarq-plugins/tree/main/mail](https://github.com/octarq-org/octarq-plugins/tree/main/mail)

---

### 3. DNS (`octarq-plugin-dns`)
- **Category**: Network
- **Description**: Domain management, DNS providers, records, and DDNS tokens.
- **Repository**: [octarq-org/octarq-plugins/tree/main/dns](https://github.com/octarq-org/octarq-plugins/tree/main/dns)

---

### 4. 2FA Vault (`octarq-plugin-twofa`)
- **Category**: Security / Utilities
- **Description**: Centralized TOTP authenticator vault for infrastructure and shared accounts with AES-256-GCM encryption.
- **Repository**: [octarq-org/octarq-plugins/tree/main/twofa](https://github.com/octarq-org/octarq-plugins/tree/main/twofa)

---

### 5. Mail Links (`octarq-plugin-maillink`)
- **Category**: Agent-Native / Automation
- **Description**: Reference plugin that intercepts incoming OTP and magic link emails via `OnEmail` hooks, creates short links, and exposes them as tools over MCP for Claude Code and Cursor.
- **Repository**: [octarq-org/octarq-plugins/tree/main/maillink](https://github.com/octarq-org/octarq-plugins/tree/main/maillink)

---

## Starter Template

Want to create your own plugin? Start with the official plugin template:

- **`_template`**: Pre-configured Go module and React TS frontend workspace using `@octarq/plugin-sdk`.

