---
"@octarq-org/plugin-sdk": minor
---

Add `NotificationChannelFormContext` and `useNotificationChannelForm`.

A plugin that registers a notification channel provider can now contribute its
config form to `settings-notification-channel:<type>` and read/write the
channel's config through the hook, instead of shipping a separate settings card
of its own. Core's own telegram and webhook forms go through the same slot.
