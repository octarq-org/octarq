// Client-side preferences that belong to the browser, not the account.
// Storage is best effort: a private-mode failure must not throw, and the
// in-memory value keeps the preference working for the rest of the session.
import { useSyncExternalStore } from "react";

const KEY = "octarq-show-personal";
const listeners = new Set<() => void>();

// Last value seen or set this session. Used when localStorage reads throw.
let memory = true;

function readStored(): boolean | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw === null) return null;
    return raw !== "false";
  } catch {
    return null;
  }
}

function current(): boolean {
  const stored = readStored();
  if (stored !== null) memory = stored;
  return memory;
}

export function setShowPersonalArea(value: boolean) {
  memory = value;
  try {
    localStorage.setItem(KEY, value ? "true" : "false");
  } catch {
    /* private mode / storage disabled — memory carries the session */
  }
  listeners.forEach((l) => l());
}

// Defaults to ON: a user who never touched the preference keeps the area.
export function useShowPersonalArea(): boolean {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    current,
    () => true,
  );
}
