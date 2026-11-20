import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const abuse: UIPlugin = {
  name: "abuse",
  routes: [{ path: "/abuse", Component: lazy(() => import("../../pages/Abuse")) }],
};

export default abuse;
