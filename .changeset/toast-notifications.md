---
"@octarq-org/plugin-sdk": minor
---

Add a toast notification system to the shared UI surface: `ToastProvider`,
the `useToast()` hook, and an imperative `toast` singleton (`toast.success` /
`toast.error` / `toast.info`). Non-blocking, glass-themed, `aria-live`
announced — the intended replacement for native `alert()` in dashboards and
plugins. Mount `<ToastProvider>` once at the app root; call `toast.*` (or
`useToast()`) anywhere below it.
