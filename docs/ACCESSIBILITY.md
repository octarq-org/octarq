# Accessibility audit — octarq dashboard shared UI

Scope: the shared primitives (`web/src/ui/primitives.tsx`, `web/src/ui/base/*`) and
the app shell (`web/src/App.tsx`) after the migration to Base UI / shadcn-style
primitives. Paired with the theme-token + animation pass in `web/src/styles.css`.

Line references are against the `worktree-agent-*` branch tree at audit time.

Legend: **[FIXED-CSS]** already changed in `styles.css` (theme surface I own) ·
**[REC]** recommended component edit (owned by the in-flight plugin-sdk stream) ·
**[FREE]** now handled for us by the Base UI migration.

---

## 1. What the Base UI migration gives us for free  [FREE]

- **Dialog** (`web/src/ui/base/dialog.tsx`) — `BaseDialog.Root/Portal/Backdrop/Popup`
  supplies focus trap, scroll lock, `Escape`-to-close, backdrop-click close, and
  `role="dialog"` + `aria-modal` + `aria-labelledby` wiring (the `BaseDialog.Title`
  at line 40 becomes the accessible name). The previous hand-rolled portal only
  approximated these.
- **Switch** (`web/src/ui/base/switch.tsx`) — `BaseSwitch.Root` gives `role="switch"`,
  `aria-checked`, full keyboard operability (Space/Enter), and a `data-[checked]`
  driven visual state. `Toggle` in `primitives.tsx:263` inherits all of it.
- **Focus-visible rings** are present on the interactive primitives:
  `Button` (`primitives.tsx:64`), `Switch` (`switch.tsx:26`) both use
  `focus-visible:ring-2 focus-visible:ring-indigo-400/60`, and a global
  `*:focus-visible` outline exists in `styles.css`.

---

## 2. Fixed in this pass (styles.css)  [FIXED-CSS]

- **Reduced motion** — added a `@media (prefers-reduced-motion: reduce)` block that
  near-zeroes `animation-duration/delay`, `transition-duration/delay` and
  `iteration-count` globally. This now covers every CSS-driven animation:
  `modal-enter`, `overlay-enter`, `.animate-expand`, the `.btn`/`.input` transitions,
  and all `tw-animate-css` enter/exit utilities. Durations are set to `0.01ms` (not
  `0`) so `animationend`/`transitionend` listeners still fire and open/close state
  settles. **Caveat:** framer-motion animations are JS inline transforms and are NOT
  covered — see §5.
- **Focus ring tokenised** — `*:focus-visible` outline now reads `var(--ring)` so the
  global outline and the primitives' `ring-indigo-400` share one accent source.
- **Muted-text floor** — `--muted-foreground` is pinned at `rgba(255,255,255,0.50)`
  (the value `.label` already used) and exposed as the `text-muted-foreground`
  utility. This is the **lowest opacity that still passes WCAG AA** for body text on
  the glass theme (≈4.9:1, see §3), so it is the sanctioned token for muted copy.

---

## 3. Colour contrast — glass theme text tones

Ratios computed for `text-white/NN` over the effective glass/app background
(`#07070b`; the semi-transparent glass fills barely lift it). WCAG AA needs **4.5:1**
for normal text and **3:1** for large text (≥24px, or ≥18.66px bold).

| Class | Approx ratio | Normal-text AA | Notes |
|-------|-------------|----------------|-------|
| `text-white/50` (`.label`, `--muted-foreground`) | ~4.9:1 | **PASS** | sanctioned floor |
| `text-white/45` | ~4.1:1 | **FAIL** | passes large only |
| `text-white/40` | ~3.5:1 | **FAIL** | passes large only |
| `text-white/35` | ~2.9:1 | **FAIL** | fails large too |
| `text-white/30` | ~2.4:1 | **FAIL** | fails everything |

### Real WCAG AA failures on actual text content  [REC — component edits]

These are small text (`text-xs` / `text-[12px]` / `text-sm`) below 4.5:1. Fix by
raising to at least `text-white/50` (or `text-muted-foreground`); use `/55`–`/60` for
comfort:

- `web/src/ui/charts.tsx:18` — "no data yet", `text-white/35 text-sm` → ~2.9:1. Real content, worst offender.
- `web/src/ui/charts.tsx:60` — empty message, `text-white/35 text-sm` → ~2.9:1.
- `web/src/ui/primitives.tsx:243` — `Field` hint, `text-xs text-white/40` → ~3.5:1.
- `web/src/ui/primitives.tsx:147` — `StatCard` label, `text-white/45` → ~4.1:1.
- `web/src/ui/primitives.tsx:252` — `Empty` body, `text-white/45` → ~4.1:1.
- `web/src/ui/HostList.tsx:48,105` — helper text, `text-white/40` → ~3.5:1.
- `web/src/ui/HostList.tsx:56,61,94` — disabled/suffix text, `text-white/35` → ~2.9:1.
- `web/src/App.tsx:665` — "Workspaces" heading, `text-white/40` → ~3.5:1 (uppercase 11px).

### Advisory / decorative (lower priority)

- `styles.css:151` `.input::placeholder` at `0.30` (~2.4:1) — placeholders are exempt
  from WCAG SC 1.4.3, but consider `0.45`–`0.50` for usability. This one I own; left
  unchanged to avoid a restyle, flagged here.
- `primitives.tsx:148` StatCard icon, `primitives.tsx:290` Guide chevron ▾/▸,
  `HostList.tsx:72` delete affordance — decorative glyphs, contrast advisory only.
- `.btn-danger` `rgba(252,165,165,0.85)` (rose-300-ish) on the dark surface clears AA;
  no action.

---

## 4. Keyboard operability & ARIA  [REC — component edits]

- **Clickable `<code>`** — `web/src/ui/primitives.tsx:303-318` (`Code`) is a
  copy-on-click `<code>` with `onClick` but no `role="button"`, `tabIndex`, or
  keydown handler. Not keyboard-operable and not announced as interactive.
  Fix: render a `<button>`, or add `role="button" tabIndex={0}` +
  `onKeyDown` (Enter/Space) + `aria-label` (e.g. "Copy to clipboard").
- **Clickable StatCard** — `web/src/ui/primitives.tsx:136-145`: when `onClick` is
  passed, a `motion.div` becomes clickable but has no `role`, `tabIndex`, or keydown —
  mouse-only. Fix: add `role="button" tabIndex={0}` + keydown, or render a button.
- **Rail nav active state** — `web/src/App.tsx:538` (`RailButton`) has `aria-label`
  but the active item (passed `active` at `App.tsx:703`) exposes no
  `aria-current="page"`. Fix: add `aria-current={active ? "page" : undefined}`.
  Also consider wrapping the rail in the existing `<nav>` with an `aria-label`.
- **Workspace switcher** (`web/src/App.tsx:629`) and **Account menu**
  (`web/src/App.tsx:721`) — toggles have `aria-label` but lack
  `aria-expanded={open}` and `aria-haspopup="menu"`. The dropdown panels
  (`App.tsx:658`, and the account menu) are `motion.div`s of plain buttons with **no
  `role="menu"`/`menuitem`, no arrow-key navigation, no `Escape`, and no focus
  return**; the close affordance is a mouse-only backdrop `<div>` (`App.tsx:657`,
  `:747`). Fix: migrate both to Base UI `Menu`/`Popover` (consistent with the Dialog
  and Switch migration) to get keyboard nav, focus management, and aria for free.
- **Guide disclosure** — `web/src/ui/primitives.tsx:282-291`: the toggle `<button>` is
  good but lacks `aria-expanded={show}` and `aria-controls` for the region at
  `primitives.tsx:293`; the ▾/▸ is decorative. Fix: add `aria-expanded`/`aria-controls`.
- **Dialog close glyph** — `web/src/ui/base/dialog.tsx:43`: `BaseDialog.Close` renders
  a button whose only content is "✕", giving it no accessible name. Fix: add
  `aria-label="Close"` (or i18n key) to the `Close`.

---

## 5. Reduced motion — remaining gap  [REC]

CSS animations are now guarded (§2). framer-motion is **not**: its animations are
JS-applied inline transforms that ignore the CSS media query. Affected:

- `web/src/ui/primitives.tsx:136` `StatCard` (initial/animate y+opacity, staggered).
- `web/src/ui/primitives.tsx:192` `ScreenWrap` (per-screen enter).
- `web/src/App.tsx:658` workspace dropdown, and the account dropdown (scale/opacity).

Fix (one line, app root): wrap the tree in framer-motion's
`<MotionConfig reducedMotion="user">`, which makes every `motion.*` respect
`prefers-reduced-motion`. Alternatively guard individual components with
`useReducedMotion()`. (The `Switch`/`Toggle` thumb uses CSS `transition`, so it is
already covered by the §2 block.)

---

## Summary

- **Free from migration:** dialog focus-trap/scroll-lock/escape/aria, switch role +
  keyboard, focus-visible rings.
- **Fixed in CSS:** reduced-motion for all CSS animation, tokenised focus ring, muted
  text floor token at the AA-passing `/50`.
- **Must fix in components:** raise the sub-`/50` text tones on real content (§3),
  keyboard-enable `Code` and clickable `StatCard`, `aria-current` on active rail item,
  `aria-expanded`/menu semantics (or Base UI `Menu`) for the two dropdowns,
  `aria-label="Close"` on the dialog ✕, `aria-expanded` on the `Guide` disclosure.
- **Reduced motion:** CSS handled; add `<MotionConfig reducedMotion="user">` for
  framer-motion.
