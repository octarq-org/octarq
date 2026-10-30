---
"@octarq/plugin-sdk": minor
---

Panel is now the one section primitive. It takes `description`, `action`, and `icon`, and its heading row matches PageHeader's title/description/action steps. A panel can render a heading alone, for a section that is a single control. The icon takes the accent color unless the caller sets its own. A caller's `className` spacing replaces the default `space-y-4` instead of adding a second utility.

Empty takes the message in `reason` and the control in `action`. `children` is the icon glyph only, and it is optional.
