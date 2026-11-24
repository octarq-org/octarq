---
"@octarq/plugin-sdk": minor
---

Add copilot extension points to `UIPlugin`: `cards` for tool cards and `artifacts` for artifact editors. The registry exposes `uiToolCards()`, `uiArtifactEditors()`, `resolveToolCard(toolName)`, and `resolveArtifactEditor(type)`. Tool card matches accept an exact name or one trailing `*` wildcard. Duplicate matches, duplicate artifact types, and artifact types outside the owning plugin namespace are rejected.
