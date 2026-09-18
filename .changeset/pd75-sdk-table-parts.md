---
"@octarq/plugin-sdk": minor
---

The presentational table parts are now part of the SDK: `TableEmpty`,
`TableError`, `TableSkeleton` and `TablePagination`.

These lived only in the host app's `pro-table`, so a plugin rendering a table had
no way to reach them: ten Pro plugins and the `twofa` plugin all hand-roll raw
`<table>` markup with no empty/loading/error state at all.

Deliberately **not** the whole concern: `ProTable` itself is a data grid built on
`@tanstack/react-table` + `react-query` + `zod`, and it stays app-side. Moving it
would push three heavy dependencies into this package — which holds seven — and
into consumer plugins that need none of it: none of the ten hand-rolled tables
sorts, and not one of them uses a table library. These four parts depend on
nothing beyond what the SDK already ships.

- Icons are drawn inline, keeping the package icon-library-free (same reasoning
  as `LockedFeature`'s key glyph); `TableEmpty` accepts an `icon` override.
- Copy resolves from the host dictionary (`proTable.*`) through the SDK's own
  i18n context — the host-fed contract `LockedFeature` and `FormError` already use.
- `TableError` also drops an `as any` cast when reading an axios-style
  `{ response: { status } }`, narrowing it properly.
