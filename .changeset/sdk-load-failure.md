---
"@octarq/plugin-sdk": minor
---

Add `LoadFailure`, a single surface for the four states every data-backed page has: 402/404 render the `LockedFeature` upsell, 403 renders `AccessDenied`, and anything else renders a retryable `TableError` with the error's own detail. Pages no longer need to re-derive those branches, which is how 500s and rejected fetches ended up dressed as "not in this build" or as an empty table.

`AccessDenied` is exported on its own for inline sections, and `TableError` now renders the localized title once when it has no detail to show instead of printing the same sentence twice.
