// The Storefront UIPlugin — the `product` backend plugin's storefront page
// (products, plans, releases), composed through the frontend SDK. Absent from
// the OSS build since ../index.pro.ts is the only module that registers it.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { storefront } from "./i18n";

export const storefrontPlugin: UIPlugin = {
  name: "storefront",
  routes: [{ path: "/storefront", Component: lazy(() => import("./page")) }],
  // Commerce area, matching where the static sidebar used to place Storefront.
  menu: [{ id: "storefront", label: "Storefront", path: "/storefront", icon: "🏪", category: "Sales" }],
  i18n: storefront,
};
