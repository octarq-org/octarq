---
"@octarq/plugin-sdk": minor
---

Add `confirmDialog()` / `useConfirm()` / `ConfirmProvider`, and remove the
`UIPlugin.menu` field along with the `uiMenus()` helper.

**New — `confirmDialog()`.** The imperative replacement for `window.confirm()`,
mirroring how `toast` replaced `alert()`. Native `confirm()` renders unstyled OS
chrome and blocks the event loop; this resolves a promise against the same
dialog the rest of the dashboard uses. Mount `ConfirmProvider` once near the
root — outside a provider the promise resolves `false`, so a missing provider
denies rather than silently confirms.

One hazard the native call did not have: this returns a promise, and a promise
is truthy, so a forgotten `await` makes every guard pass. That is why the
singleton is named `confirmDialog` and not `confirm` — `if (!confirm(...))`
keeps working by accident, `if (!confirmDialog(...))` does not compile away
quietly.

**Breaking — `UIPlugin.menu` and `uiMenus()` are gone.** Sidebar placement now
comes only from the Go plugin's `MenuProvider` (`Menus()` → `/api/menus`). The
host already dropped any entry whose path the backend did not announce — that is
the mechanism that makes a disabled feature's menu disappear — so a
frontend-declared menu never stood alone; it was a hand-maintained copy, and the
copies had drifted (one plugin's category read `Operations` on the Go side and
`Marketing` on the frontend). Plugins that declared `menu` should delete it and
declare the entry in their Go `Menus()`; the field's only remaining merit,
painting the sidebar before the API answers, is handled by the host caching the
last `/api/menus` response.
