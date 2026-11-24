import type { HelpDocMeta } from "../api";

export interface DocRouteMatch {
  doc: HelpDocMeta;
  route: string;
}

export function createDocRoutes(docs: HelpDocMeta[]) {
  const entries: DocRouteMatch[] = [];
  for (const doc of docs) {
    for (const route of doc.routes ?? []) {
      const normalized = route.trim().replace(/\/+$/, "");
      if (!normalized || !normalized.startsWith("/") || normalized === "/help") continue;
      entries.push({ doc, route: normalized });
    }
  }
  entries.sort((a, b) => b.route.length - a.route.length);

  return {
    match(pathname: string): DocRouteMatch | undefined {
      const path = pathname.replace(/\/+$/, "") || "/";
      return entries.find(
        (entry) => path === entry.route || path.startsWith(`${entry.route}/`),
      );
    },
    size: entries.length,
  };
}
