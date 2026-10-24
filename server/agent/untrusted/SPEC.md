# SPEC — agent/untrusted

Tier-2 engineering specification. Not embedded in the binary.

## Purpose

External content — mail bodies, webhook payloads, comments, chat messages,
recalled memory notes, and retrieved RAG documents — is typed, normalized,
and wrapped in an explicit boundary before it can reach a prompt context.

## Honest threat model

This is a **probabilistic** defense. Normalization and escaping remove the
cheap bypasses and make injection attempts visible and expensive, but no
text transformation can make a language model ignore instructions with
certainty. The deterministic decisions live in the tool execution and human
approval gates.

## Contract

- `Untrusted` (string type) is the canonical marker.
- `Source{Kind, ID}` with the closed kind set `mail | webhook | comment | im | tool | run | memory | rag`.
- `Wrap(source, content)` normalizes, escapes, and wraps untrusted text in `<untrusted_external_content source="..." safe_mode="true">...</untrusted_external_content>`.
- `Rules` is the platform iron rule text composed into `safety.Preamble`.
- `PostReminder` reinforces the untrusted data boundary after RAG and memory blocks.
