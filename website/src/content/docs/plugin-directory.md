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

### 1. 2FA Vault (`octarq-plugin-twofa`)
- **Category**: Security / Utilities
- **Description**: Centralized TOTP authenticator vault for infrastructure and shared accounts with AES-256-GCM encryption.
- **Repository**: [octarq-org/octarq-plugins/tree/main/twofa](https://github.com/octarq-org/octarq-plugins/tree/main/twofa)

---

### 2. Mail Links (`octarq-plugin-maillink`)
- **Category**: Agent-Native / Automation
- **Description**: Reference plugin that intercepts incoming OTP and magic link emails via `OnEmail` hooks, creates short links, and exposes them as tools over MCP for Claude Code and Cursor.
- **Repository**: [octarq-org/octarq-plugins/tree/main/maillink](https://github.com/octarq-org/octarq-plugins/tree/main/maillink)

---

## Starter Template

Want to create your own plugin? Start with the official plugin template:

- **`_template`**: Pre-configured Go module and React TS frontend workspace using `@octarq/plugin-sdk`.

