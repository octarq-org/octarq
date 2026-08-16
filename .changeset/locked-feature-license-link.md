---
"@octarq/plugin-sdk": patch
---

Fix the `LockedFeature` upgrade button's navigation target: it pointed at
`/admin/license`, which only worked because the host app happened to redirect
`/license` to `/settings/license` — a fragile coupling that would strand the
paywall button on "Not part of this build" if that redirect were ever removed.
Point it directly at the license-activation page (`/admin/settings/license`,
i.e. router path `/settings/license` under the app's `/admin` basename), the
path the Pro licensing plugin actually registers.
