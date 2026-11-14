import { uiRoutes } from "@octarq/plugin-sdk";

// Routes the dashboard shell renders itself, as declared in AppRoutes.tsx.
// Keep this list equal to the static <Route> table there.
const SHELL_ROUTES = [
  "/overview",
  "/notifications",
  "/settings",
  "/data",
  "/onboarding",
  "/admin/invite/accept",
  "/admin/reset",
];

// firstSegment is the part of a path that decides which application serves it:
// "/aiduu/?room=3" -> "aiduu", "/settings/general" -> "settings".
function firstSegment(path: string): string {
  const bare = path.split(/[?#]/)[0];
  return bare.split("/").filter(Boolean)[0] ?? "";
}

/**
 * Reports whether the dashboard shell serves a path.
 *
 * The shell owns its static routes and every route a composed frontend plugin
 * registered. Other paths on the same origin belong to another application: a
 * plugin's own UI (served by the backend under its own prefix, such as
 * /aiduu) or a page built outside the dashboard (such as /status). Those paths
 * need a full page load, not a route inside the shell.
 */
export function dashboardOwnsPath(path: string): boolean {
  const segment = firstSegment(path);
  if (!segment) return true;
  for (const route of SHELL_ROUTES) {
    if (firstSegment(route) === segment) return true;
  }
  for (const route of uiRoutes()) {
    if (firstSegment(route.path) === segment) return true;
  }
  return false;
}
