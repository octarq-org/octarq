// Theme toggle. The default is LIGHT (Wise/Cloudflare idiom); `.dark` on <html>
// restores the original frosted-glass theme. The persisted choice is applied
// pre-paint by an inline script in index.html so there's no flash; this module
// owns reading/flipping it at runtime.
import { useSyncExternalStore } from "react";

export type Theme = "light" | "dark";

const KEY = "octarq-theme";
const listeners = new Set<() => void>();

function current(): Theme {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function setTheme(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
  try {
    localStorage.setItem(KEY, theme);
  } catch {
    /* private mode / storage disabled — the DOM class still applies for this session */
  }
  listeners.forEach((l) => l());
}

export function toggleTheme() {
  setTheme(current() === "dark" ? "light" : "dark");
}

// Subscribe React components to theme changes so an icon/label re-renders on flip.
export function useTheme(): Theme {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    current,
    () => "light",
  );
}
