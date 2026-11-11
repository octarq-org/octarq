---
"@octarq/plugin-sdk": minor
---

Move the business plugin UI primitives into the SDK and publish CJS alongside ESM.

- Add `AreaChart`, `BarList`, `ListSkeleton`, and the `SetupStep` family so plugin web packages do not carry private copies.
- Export the shared hooks from the package root.
- Emit CommonJS builds so the package works from both ESM and CJS consumers.
