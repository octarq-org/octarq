# SPEC — agent/safety

Tier-2 engineering specification. Not embedded in the binary.

## Purpose

Own the platform iron law that precedes every model turn: `Preamble` (a
const, no setter) and `Prompt(soul)` which renders the preamble first,
followed by a quarantine banner and the subordinate persona.

The agent loop assembles its system prompt here, never from caller-supplied
text. A caller can choose a persona; it cannot remove, reorder, or precede
the platform rules.

`untrusted.Rules` is composed into the preamble, so the fence contract and
the rule that explains it cannot drift apart.
