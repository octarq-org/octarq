import { useSyncExternalStore } from "react";

const KEY = "octarq-show-personal";
const listeners = new Set<() => void>();

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
  }
  listeners.forEach((l) => l());
}

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
