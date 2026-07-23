---
title: Accessibility Guide
description: Accessibility standards, guidelines, and compliance details for the Octarq dashboard.
---

This guide outlines the accessibility (a11y) standards, color contrast parameters, and keyboard operability requirements for the Octarq dashboard and plugins.

---

## 1. Primitives & Components

By default, Octarq uses **Base UI** (`@base-ui/react`) and **shadcn/ui** to build accessible interactive elements.

- **Dialogs / Modals**: Built with `BaseDialog` which provides focus trapping, scroll locking, `Escape` key close, backdrop-click close, and correct `role="dialog"` + `aria-modal` + `aria-labelledby` attributes.
- **Switches / Toggles**: Built with `BaseSwitch` which handles `role="switch"`, `aria-checked` states, and full keyboard interaction (Space/Enter).
- **Focus Rings**: Interactive elements must include clear `focus-visible` styles using the application's shared token:
  ```css
  /* Example styling */
  focus-visible:ring-2 focus-visible:ring-indigo-400/60
  ```

---

## 2. Color Contrast (Glass Theme)

Octarq's dark "glass" theme targets WCAG AA standards:
- **Normal text**: Requires a contrast ratio of at least **4.5:1** against the background.
- **Large text**: Requires at least **3:1** (defined as $\ge$ 24px, or $\ge$ 18.6px bold).

### Reference Contrast Tones (over dark surface `#07070b`):

| Opacity / Style | Contrast Ratio | Compliance | Role |
|-----------------|----------------|------------|------|
| `text-white/50` / `--muted-foreground` | ~4.9:1 | **PASS** | Sanctioned minimum for muted body text |
| `text-white/45` | ~4.1:1 | **FAIL** (Normal text) | Large headings only |
| `text-white/40` | ~3.5:1 | **FAIL** (Normal text) | Large headings only |
| `text-white/35` | ~2.9:1 | **FAIL** (All) | Decorative elements only |
| `text-white/30` | ~2.4:1 | **FAIL** (All) | Decorative elements only |

> [!IMPORTANT]
> Always use `text-white/50` (or `text-muted-foreground`) as the absolute contrast floor for readable content, captions, and descriptions.

---

## 3. Keyboard Operability & ARIA Requirements

Every interactive element contributed by a plugin must support keyboard navigation:

### 3.1 Copyable Code Blocks
Do not place click listeners on plain `<code>` tags. Instead, wrap them in a `<button>` or add `role="button" tabIndex={0}` along with keydown listeners for Space/Enter and a clear `aria-label` (e.g., "Copy to clipboard").

### 3.2 Interactivity Metadata
- **State indicators**: Expandable components (menus, accordions, disclosures) must expose `aria-expanded={isOpen}` and `aria-controls="region-id"`.
- **Active links**: The active item in the navigation rail must carry `aria-current="page"`.
- **Dialogs**: The close button in dialog headers should have an explicit `aria-label="Close"` instead of just rendering "✕".
