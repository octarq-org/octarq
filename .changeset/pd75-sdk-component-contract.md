---
"@octarq/plugin-sdk": minor
---

Every component now follows one authoring contract: `forwardRef`, a `displayName`,
an exported `*Props` type, and a `className` prop.

The kit was already one surface — one definition per component, one import
specifier, and a strict token palette — but the *way components were written* was
not uniform. Only `Alert`, `Badge` and `Button` forwarded refs, set a display
name, and exported their props; the other 25 did none of it. Measured before
changing anything:

| convention | before | after |
|---|---|---|
| `forwardRef` | 3 / 28 | 28 / 28 |
| `displayName` | 3 / 28 | 28 / 28 |
| exported `*Props` | 8 / 28 | 28 / 28 |
| accepts `className` | 25 / 28 | 28 / 28 |

What that fixes in practice:

- **`<Input ref>`, `<Textarea ref>`, `<Select ref>`, `<Table ref>` and the rest now
  work.** Previously only `<Button ref>` did, which is the kind of gap you only
  find when you need to focus a field.
- **Components are named in React DevTools** instead of showing as anonymous.
- **Prop types are importable** — `import type { InputProps } from "@octarq/plugin-sdk"`.
- **`Modal`, `Toggle` and the Toaster are stylable by the caller** via `className`,
  which they previously ignored.

Each ref is documented at its declaration and points at the element the component
is named after — the input, the table, the switch control, the dialog card. The
few multi-element wrappers (`Modal`, `Select`, `Tabs`, `Tooltip`, `Field`) forward
to their primary element (popup, trigger, container), noted in place. Providers and
hooks are deliberately excluded: they render no single element, so a ref would be
meaningless.

`LockedFeature` also stops hand-rolling `twMerge` and uses the shared `cn`, so
class merging has one implementation the way everything else does.

Purely additive — no existing call site changes.
