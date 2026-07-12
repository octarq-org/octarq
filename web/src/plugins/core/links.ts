// Links — a core feature packaged as a first-class UIPlugin. Core features flow
// through the exact same registry/route/menu pipeline as Pro plugins (see
// ./index.ts); only the composition point differs (always-on, not manifest-
// selected). `category` must equal the STATIC_AREAS group label the item lives
// in, and `icon` is a key from the shared lucide map in shell/areas.tsx.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";

const links: UIPlugin = {
  name: "links",
  routes: [{ path: "/links", Component: lazy(() => import("../../pages/Links")) }],
  menu: [{ id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" }],
};

export default links;
