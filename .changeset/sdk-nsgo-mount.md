---
"@octarq/plugin-sdk": minor
---

Add `NsgoMount` and `useNsgoBridge`: an nsgo web build runs inside a React element, with the host's startup parameters, appearance, locale and host-call answers. The manifest, loader and compiled WebAssembly are cached per app, so a second mount costs one call. A failed load renders the fallback (or an alert), never a blank frame.
