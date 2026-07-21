// Audit log — core feature as a UIPlugin (see ./index.ts for the convention).
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const audit: UIPlugin = {
  name: "audit",
  routes: [{ path: "/audit", Component: lazy(() => import("../../pages/Audit")) }],
  menu: [{ id: "audit", label: "Audit Log", path: "/audit", icon: "scroll-text", category: "System" }],
};

export default audit;
