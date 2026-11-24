import { uiRoutes } from "@octarq/plugin-sdk";

const SHELL_ROUTES = [
  "/overview",
  "/notifications",
  "/settings",
  "/data",
  "/onboarding",
  "/admin/invite/accept",
  "/admin/reset",
];

function firstSegment(path: string): string {
  const bare = path.split(/[?#]/)[0];
  return bare.split("/").filter(Boolean)[0] ?? "";
}

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
