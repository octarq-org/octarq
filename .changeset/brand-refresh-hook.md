---
"@octarq/plugin-sdk": patch
---

Add `useBrandRefresh()`, the host callback that re-reads and re-applies the
operator's branding. A plugin that changes the branding can now tell the shell to
catch up instead of leaving the sidebar mark, page title and accent colours on
the old brand until a manual reload.
