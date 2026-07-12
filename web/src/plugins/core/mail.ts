// Mail — core feature as a UIPlugin (see ./index.ts for the convention).
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";

const mail: UIPlugin = {
  name: "mail",
  routes: [{ path: "/mail", Component: lazy(() => import("../../pages/Mail")) }],
  menu: [{ id: "mail", label: "Mail", path: "/mail", icon: "mail", category: "Messaging" }],
};

export default mail;
