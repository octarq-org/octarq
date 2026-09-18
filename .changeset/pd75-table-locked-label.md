---
"@octarq/plugin-sdk": patch
---

`TableError` no longer shows the literal string `pro_table` on its 402 upsell.

The locked-feature heading was a raw snake_case identifier, so a user hitting a
licensed-feature gate read "pro_table" as the heading. It now resolves
`proTable.lockedFeature` from the host dictionary, like the rest of the
component's copy — the host already supplies that namespace, so the new key
(`lockedFeature`, in all five locales) is the only addition.

Found while promoting this component into the SDK; the call site was carried over
unchanged from the host app, where the same leak had been shipping.
