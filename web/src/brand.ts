// Brand name resolution. The product name is env-driven on the server
// (LED_APP_NAME) and surfaced via GET /api/auth/config. We fetch it once,
// cache it module-wide, and re-render subscribers when it resolves. Components
// call useAppName(); nothing hardcodes "led" anymore.
import { useEffect, useState } from "react";
import { api } from "./api";

const FALLBACK = "led";
let cached: string | null = null;
let inflight: Promise<void> | null = null;
const listeners = new Set<() => void>();

function load(): Promise<void> {
  if (!inflight) {
    inflight = api
      .authConfig()
      .then((c) => { cached = c.appName || FALLBACK; })
      .catch(() => { cached = FALLBACK; })
      .then(() => {
        document.title = cached!;
        listeners.forEach((l) => l());
      });
  }
  return inflight;
}

// useAppName returns the product name, defaulting to "led" until the config
// resolves, then re-rendering the caller with the real value.
export function useAppName(): string {
  const [name, setName] = useState(cached ?? FALLBACK);
  useEffect(() => {
    if (cached !== null) {
      setName(cached);
      return;
    }
    const notify = () => setName(cached ?? FALLBACK);
    listeners.add(notify);
    load();
    return () => { listeners.delete(notify); };
  }, []);
  return name;
}

// brandInitial is the single-character logo glyph derived from the app name.
export function brandInitial(name: string): string {
  return (name.trim()[0] || "L").toUpperCase();
}
