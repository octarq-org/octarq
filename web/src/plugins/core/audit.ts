// Audit log — core feature as a UIPlugin (see ./index.ts for the convention).
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const audit: UIPlugin = {
  name: "audit",
  routes: [{ path: "/audit", Component: lazy(() => import("../../pages/Audit")) }],
};

export default audit;
