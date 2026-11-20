import type { NavigateFunction } from "react-router-dom";
import { dashboardOwnsPath } from "./routeOwnership";

export function safeNextPath(candidate: string | null | undefined): string | null {
  if (!candidate || typeof candidate !== "string") return null;
  const trimmed = candidate.trim();
  if (!trimmed.startsWith("/") || trimmed.startsWith("//") || trimmed.includes("\\")) {
    return null;
  }

  try {
    const decoded = decodeURIComponent(trimmed);
    if (decoded.startsWith("//") || decoded.includes("\\")) {
      return null;
    }
  } catch {
    return null;
  }

  try {
    const parsed = new URL(trimmed, "http://localhost");
    if (parsed.origin !== "http://localhost" || parsed.protocol !== "http:") {
      return null;
    }
    if (!parsed.pathname.startsWith("/") || parsed.pathname.startsWith("//")) {
      return null;
    }
    return parsed.pathname + parsed.search + parsed.hash;
  } catch {
    return null;
  }
}

export type PostLoginDestination =
  | { type: "spa"; path: string }
  | { type: "external"; url: string };

export function resolvePostLoginNavigation(
  nextCandidate: string | null | undefined,
  currentPath = "/",
  ownsPath: (path: string) => boolean = dashboardOwnsPath,
): PostLoginDestination {
  const safe = safeNextPath(nextCandidate);
  if (safe) {
    if (safe === "/admin" || safe === "/admin/" || safe === "/" || safe === "/login" || safe === "/admin/login") {
      return { type: "spa", path: "/overview" };
    }
    if (safe.startsWith("/admin/")) {
      const sub = safe.slice("/admin".length);
      if (!sub || sub === "/" || sub === "/login") {
        return { type: "spa", path: "/overview" };
      }
      return { type: "spa", path: sub };
    }
    if (!ownsPath(safe)) {
      return { type: "external", url: safe };
    }
    return { type: "spa", path: safe };
  }

  if (currentPath === "/" || currentPath === "/login") {
    return { type: "spa", path: "/overview" };
  }
  return { type: "spa", path: currentPath };
}

export function performPostLoginNavigation(
  navigate: NavigateFunction,
  nextCandidate: string | null | undefined,
  currentPath = "/",
): void {
  const dest = resolvePostLoginNavigation(nextCandidate, currentPath);
  if (dest.type === "external") {
    if (typeof window !== "undefined") {
      window.location.replace(dest.url);
    }
    return;
  }
  navigate(dest.path, { replace: true });
}
