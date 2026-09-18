---
"@octarq/plugin-sdk": minor
---

Button is now the single Button definition for the whole product.

It previously existed twice: this package shipped a gradient (indigo→violet)
`primary`, while the host app carried a flat one in
`web/src/components/ui/Button.tsx` that its barrel re-exported *in place of*
this one — an explicit named export beats `export *`. Core pages therefore
rendered the flat button, every plugin rendered the gradient one, and `size`
and `secondary` existed on only one of them.

- `primary` is now FLAT (`bg-primary` / `hover:bg-primary-hover` / `text-primary-foreground`).
  A gradient end is a hardcoded hue that cannot follow the `--primary` seed, so
  it stayed indigo on a white-label rebranded instance.
- New `size` axis: `sm | md | lg` (default `md`), exported as `ButtonSize`.
- New `secondary` variant: `bg-muted` + `border-border`.
- `danger` derives from the `--danger-*` tokens instead of a literal `rose`.
- `Button` forwards its ref to the underlying `<button>`.
- `ButtonProps` is now exported as an interface (was inline).

Visual change for consumers on the previous gradient primary: it is flat now.
