// The Bookkeeping (finance) UIPlugin — the `finance` backend plugin's ledger
// page (transactions), composed through the frontend SDK. Its modals and
// shared helpers are co-located (./modals, ./shared). Absent from the OSS build
// since ../index.pro.ts is the only module that registers it.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { finance } from "./i18n";

export const financePlugin: UIPlugin = {
  name: "finance",
  routes: [{ path: "/finance", Component: lazy(() => import("./page")) }],
  // Commerce area, matching where the static sidebar used to place Bookkeeping.
  menu: [{ id: "finance", label: "Bookkeeping", path: "/finance", icon: "📒", category: "Finance" }],
  i18n: finance,
};
