// The Audit UIPlugin — the `ai`/audit backend plugin's audit-log page, composed
// through the frontend SDK. Absent from the OSS build since ../index.pro.ts is
// the only module that registers it.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { audit } from "./i18n";

export const auditPlugin: UIPlugin = {
  name: "audit",
  routes: [{ path: "/audit", Component: lazy(() => import("./page")) }],
  // Insights area, matching where the static sidebar used to place Audit.
  menu: [{ id: "audit", label: "Audit", path: "/audit", icon: "📜", category: "System" }],
  i18n: audit,
};
