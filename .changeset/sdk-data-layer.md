---
"@octarq/plugin-sdk": minor
---

Add one HTTP surface for plugins and the host: `req`, `ApiError`, `configurePluginHttp` and `pluginURL`. Add `usePluginData`, `usePluginMutation` and `usePluginStream` over the host's single react-query client. Add the `./testing` subpath with `TestHarness` and `mockPluginApi`. Add `UIWidget.requires` so a widget disappears with a disabled dependency, and `useExtensionSlotContext` for typed slot context. A crashed widget now logs its slot name in development.
