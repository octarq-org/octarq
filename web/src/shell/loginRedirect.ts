import type { NavigateFunction } from "react-router-dom";
import { dashboardOwnsPath } from "./routeOwnership";

/**
 * Validates a redirect target to protect against open-redirect attacks.
 *
 * Rules:
 * - Must be a non-empty string starting with a single '/'
 * - Must NOT start with '//' (protocol-relative URL)
 * - Must NOT contain '\\' (prevent backslash bypass in browsers)
 * - Must NOT decode to '//' or contain '\\'
 * - Must resolve to the local origin with 'http:' protocol when parsed against dummy base
 *
 * Returns the normalized path (pathname + search + hash) or null if invalid.
 */
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

/**
 * Resolves the destination after login.
 *
 * If a safe `next` parameter is present:
 * - /admin/foo -> SPA route /foo
 * - /admin or /admin/ -> SPA route /overview
 * - /login or /admin/login -> SPA route /overview
 * - a path the dashboard shell serves -> SPA route relative to the /admin basename
 * - any other path -> external page load (a plugin's own UI, /status, …)
 *
 * If no `next` parameter is present (or invalid):
 * - If current path is root ("/") or "/login", defaults to "/overview".
 * - Otherwise stays on current path.
 */
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
    // A path the shell does not serve belongs to another application on the
    // same origin (a plugin's UI under /aiduu, the status page under /status).
    // A route inside the shell would not exist, so load the page itself.
    if (!ownsPath(safe)) {
      return { type: "external", url: safe };
    }
    return { type: "spa", path: safe };
  }

  // Fallback when no next param is given:
  if (currentPath === "/" || currentPath === "/login") {
    return { type: "spa", path: "/overview" };
  }
  return { type: "spa", path: currentPath };
}

/**
 * Performs post-login navigation using either React Router or window.location.
 */
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
