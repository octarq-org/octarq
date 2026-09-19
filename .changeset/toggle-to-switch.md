---
"@octarq/plugin-sdk": minor
---

`Toggle` is removed. `Switch` is the one public switch component.

The kit shipped two names for the same control: `Switch` (the Base UI wrapper with
the conventional `checked`/`onCheckedChange` API) and `Toggle` (a 19-line forwarder
over it with a bespoke `on`/`onChange` API). `Toggle` was never a second
implementation — it rendered `<Switch checked={on} onCheckedChange={onChange} />`
and nothing else. Both were public, which is dual-track API, and the project's
pre-v1.0 rules forbid keeping it.

`Switch` was only public by accident. `ui/index.ts` re-exports the whole `base/`
implementation barrel, and `base/switch.tsx` happened to sit in it, so both names
landed in the published `.d.ts`. `Switch` is now exported deliberately from
`base/index.ts`.

Migration is mechanical and covers the whole kit:

| before | after |
|---|---|
| `<Toggle on={x} onChange={f} />` | `<Switch checked={x} onCheckedChange={f} />` |
| `<Toggle on onChange={f} />` | `<Switch checked onCheckedChange={f} />` |
| `import { Toggle } from "@octarq/plugin-sdk"` | `import { Switch } from "@octarq/plugin-sdk"` |

Two defects in `Switch` itself are fixed in the same release:

- **`aria-label` now reaches the control.** It never did. The component destructured
  four props and spread no rest, and TypeScript does not report excess hyphenated
  JSX attributes — so `aria-label` type-checked, was dropped, and every switch
  using it had no accessible name.
- **The track renders.** Base UI's `Switch.Root` renders a `<span>`, not a
  `<button>`, so `h-5 w-9` on it did nothing: an inline element ignores width and
  height, leaving the track a zero-size box. Only the 16px thumb was visible or
  clickable, and the checked background never painted. `inline-flex` gives the
  control a real box.

The ref type also stops lying: it was `HTMLButtonElement` for an element that has
always been a `<span>`.
