// Brand name resolution. The product name is a runtime setting on the server
// (Settings → General → app_name), surfaced via GET /api/auth/config. We fetch
// it once, cache it module-wide, and publish it into the SDK's brand context via
// <BrandBridge> so SDK components (e.g. LockedFeature) and app components read
// one source. `useAppName`/`brandInitial` are re-exported from the SDK, so every
// `import { useAppName } from "./brand"` keeps working and reads that context.
import { useEffect, useState, ReactNode } from "react";
import { api } from "./api";
import { BrandProvider, useAppName, brandInitial } from "../../packages/plugin-sdk/src";

export { useAppName, brandInitial };

const FALLBACK = "octarq";
let cached: string | null = null;
let inflight: Promise<void> | null = null;
const listeners = new Set<() => void>();

function load(): Promise<void> {
  if (!inflight) {
    inflight = api
      .authConfig()
      .then((c) => {
        cached = c.appName || FALLBACK;
      })
      .catch(() => {
        cached = FALLBACK;
      })
      .then(() => {
        document.title = cached!;
        listeners.forEach((l) => l());
      });
  }
  return inflight;
}

// Resolves the operator's product name from the server, re-rendering when it
// arrives. Internal — only BrandBridge consumes it, to feed the SDK context.
function useAppNameSource(): string {
  const [name, setName] = useState(cached ?? FALLBACK);
  useEffect(() => {
    if (cached !== null) {
      setName(cached);
      return;
    }
    const notify = () => setName(cached ?? FALLBACK);
    listeners.add(notify);
    load();
    return () => {
      listeners.delete(notify);
    };
  }, []);
  return name;
}

// BrandBridge publishes the fetched product name into the SDK brand context.
// Mount it near the app root, inside the tree that renders branded components.
export function BrandBridge({ children }: { children: ReactNode }) {
  const name = useAppNameSource();
  return <BrandProvider name={name}>{children}</BrandProvider>;
}
