import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const audit: UIPlugin = {
  name: "audit",
  routes: [{ path: "/governance/audit", Component: lazy(() => import("../../pages/Audit")) }],
};

export default audit;
