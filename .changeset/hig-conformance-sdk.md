---
"@octarq/plugin-sdk": minor
---

Textarea now takes part in Field, and primary fill tokens changed.

- `Textarea` renders Base UI's `Field.Control` with a textarea render target. A
  textarea inside a `Field` now receives the label and hint: the field was
  labelled on screen and nameless to a screen reader.
- `Button variant="primary"` and the other filled primary surfaces use the new
  `--primary-solid` and `--primary-solid-hover` tokens. White text on the raw
  `--primary` seed measures 4.47:1 in dark mode, below the 4.5:1 minimum.

Host requirement: the host stylesheet must declare `--primary-solid` and
`--primary-solid-hover` in `:root`, in `.dark`, and through `@theme inline`.
Without them a primary button renders with no background.
