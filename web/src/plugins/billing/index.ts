// The Billing UIPlugin — the `billing` backend plugin's config page (Stripe /
// Polar, price map), composed through the frontend SDK. Absent from the OSS
// build since ../index.pro.ts is the only module that registers it.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { billing } from "./i18n";

export const billingPlugin: UIPlugin = {
  name: "billing",
  routes: [{ path: "/billing", Component: lazy(() => import("./page")) }],
  // Commerce area, matching where the static sidebar used to place Billing.
  menu: [{ id: "billing", label: "Billing", path: "/billing", icon: "💳", category: "Billing" }],
  i18n: billing,
};
