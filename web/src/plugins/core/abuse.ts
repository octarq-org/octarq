// Abuse reports — core feature as a UIPlugin (see ./index.ts for the convention).
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const abuse: UIPlugin = {
  name: "abuse",
  routes: [{ path: "/abuse", Component: lazy(() => import("../../pages/Abuse")) }],
  menu: [{ id: "abuse", label: "Abuse Reports", path: "/abuse", icon: "shield-alert", category: "Security" }],
};

export default abuse;
