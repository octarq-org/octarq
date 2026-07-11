// The Servers (VPS) UIPlugin — the `infra` backend plugin's server page,
// composed through the frontend SDK. Absent from the OSS build (no route, no
// sidebar entry) since ../index.pro.ts is the only module that registers it.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { vps } from "./i18n";

export const vpsPlugin: UIPlugin = {
  name: "vps",
  routes: [{ path: "/vps", Component: lazy(() => import("./page")) }],
  // Assets area, matching where the static sidebar used to place Servers.
  menu: [{ id: "vps", label: "Servers", path: "/vps", icon: "🖥️", category: "Hosting" }],
  i18n: vps,
};
