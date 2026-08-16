---
title: Accessibility Guide
description: Accessibility requirements and patterns for the Octarq dashboard.
sidebar:
  order: 2
  group:
    label: "Guides"
---


Octarq dashboard and plugin UI requirements for keyboard navigation, ARIA attributes, and color contrast.

---

## Primitives & Components

By default, Octarq uses **Base UI** (`@base-ui/react`) and **shadcn/ui** to build accessible interactive elements.

- **Dialogs / Modals**: Built with `BaseDialog` which provides focus trapping, scroll locking, `Escape` key close, backdrop-click close, and `role="dialog"` + `aria-modal` + `aria-labelledby` attributes.
- **Switches / Toggles**: Built with `BaseSwitch` which handles `role="switch"`, `aria-checked` states, and keyboard interaction (Space/Enter).
- **Focus Rings**: Interactive elements must include clear `focus-visible` styles using the application's shared token:
  ```css
  /* Example styling */
  focus-visible:ring-2 focus-visible:ring-indigo-400/60
  ```

## Motion

CSS animations are guarded by a `@media (prefers-reduced-motion: reduce)` block,
and framer-motion respects user preference via `<MotionConfig reducedMotion="user">`
in `main.tsx`. Do not add animations that cannot be disabled this way.

---

## Color Contrast (Glass Theme)

Octarq's dark "glass" theme targets WCAG AA standards:
- **Normal text**: Contrast ratio of at least **4.5:1** against the background.
- **Large text**: Contrast ratio of at least **3:1** ($\ge$ 24px, or $\ge$ 18.6px bold).

### Reference Contrast Tones (over dark surface `#07070b`)

| Opacity / Style | Contrast Ratio | Compliance | Role |
|-----------------|----------------|------------|------|
| `text-white/50` / `--muted-foreground` | ~4.9:1 | **PASS** | Sanctioned minimum for muted body text |
| `text-white/45` | ~4.1:1 | **FAIL** (Normal text) | Large headings only |
| `text-white/40` | ~3.5:1 | **FAIL** (Normal text) | Large headings only |
| `text-white/35` | ~2.9:1 | **FAIL** (All) | Decorative elements only |
| `text-white/30` | ~2.4:1 | **FAIL** (All) | Decorative elements only |

> [!IMPORTANT]
> Use `text-white/50` (or `text-muted-foreground`) as the contrast floor for readable content, captions, and descriptions.

---

## Keyboard Operability & ARIA Requirements

Interactive elements contributed by plugins must support keyboard navigation:

### Copyable Code Blocks
Do not place click listeners on plain `<code>` tags. Instead, wrap them in a `<button>` or add `role="button" tabIndex={0}` along with keydown listeners for Space/Enter and an `aria-label` (e.g., "Copy to clipboard").

### Interactivity Metadata
- **State indicators**: Expandable components (menus, accordions, disclosures) must set `aria-expanded={isOpen}` and `aria-controls="region-id"`.
- **Active links**: The active item in the navigation rail must set `aria-current="page"`.
- **Dialogs**: The close button in dialog headers requires an explicit `aria-label="Close"` instead of rendering a plain "✕".

