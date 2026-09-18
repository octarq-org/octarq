---
"@octarq/plugin-sdk": minor
---

Code, Guide and timeAgo are now part of the SDK's UI surface.

These were the last three app-coupled extras the host's facade had to union in
(`web/src/plugin-sdk/ui.tsx`), which is why a published plugin package could not
use them — a Pro plugin rendering a copy-to-clipboard value or a collapsible
guidance panel had no equivalent to reach for.

All three turned out to be pure:

- `Code` / `Guide` only needed `useTranslation`, which the SDK already owns and
  the host already feeds the `uiCommon.*` dictionary into.
- `timeAgo` has no dependencies at all.

Promoting them lets the host's facade shrink to a single hop, so the SDK is now
the only UI surface in the product rather than one of two.

`timeAgo` keeps its existing English-only strings: it is a plain function with no
i18n context to read, so localizing it is a `useTimeAgo()` hook (a change to
every call site), not a move. That is deliberately left for a separate change.
