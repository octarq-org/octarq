// The licenses UIPlugin — the PoC that validates the frontend plugin contract
// against a real existing Pro page. `name: "licenses"` matches the backend
// issuer plugin's identifier; the page, its route, its sidebar entry, and its
// i18n namespace are all composed through the SDK.
//
// Composed only in a commercial build (see ../index.ts). In the OSS build this
// module is never registered, so `/licenses` 404-degrades via the app's neutral
// plugin fallback — exactly the "plugin not in this build" convention.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { licenses } from "./i18n";

export const licensesPlugin: UIPlugin = {
  name: "licenses",
  routes: [
    { path: "/licenses", Component: lazy(() => import("./page")) },
  ],
  // Commerce area, matching where the static sidebar already places Licenses.
  menu: [
    { id: "licenses", label: "Licenses", path: "/licenses", icon: "🔑", category: "commerce" },
  ],
  i18n: licenses,
};
