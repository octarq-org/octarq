---
"@octarq/plugin-sdk": minor
---

Add `PasswordConfirmProvider` / `usePasswordConfirm` / `confirmPassword` — a
shared password re-authentication dialog, mirroring the existing confirm dialog.
It resolves the password the user typed (or `null` if dismissed) so a sensitive
action can hand it straight to the server call that verifies it, instead of each
page growing its own password field.
