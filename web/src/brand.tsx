// Brand resolution. The product name is a runtime setting on the server
// (Settings → General → app_name); the white-label logo and accent colors are
// runtime settings written only by the Pro white-label plugin (brand_logo /
// brand_color / brand_color_2). All are surfaced together via
// GET /api/auth/config. We fetch once, cache module-wide, publish the name into
// the SDK's brand context via <BrandBridge> (so SDK components like
// LockedFeature read one source), publish the logo into a core-local context
// (shell marks read it), and apply the accent colors as CSS variables — an OSS
// build leaves them blank, keeping the default indigo→violet look.
import { useEffect, useState, ReactNode, createContext, useContext } from "react";
import { api } from "./api";
import { BrandProvider, useAppName, brandInitial } from "../../packages/plugin-sdk/src";

export { useAppName, brandInitial };

const FALLBACK = "octarq";

type Brand = { name: string; logoUrl: string };
let cached: Brand | null = null;
let inflight: Promise<void> | null = null;
const listeners = new Set<() => void>();

// applyAccents overrides the brand accent design tokens (styles.css :root) with
// the operator's colors. Only the gradient/accent/ring tokens move; the rest of
// the palette is untouched. Blank values leave the defaults in place.
function applyAccents(color: string, color2: string) {
  if (!color) return;
  const c1 = color;
  const c2 = color2 || color;
  const root = document.documentElement.style;
  root.setProperty("--accent-indigo", c1);
  root.setProperty("--accent-violet", c2);
  root.setProperty("--primary", c1);
  root.setProperty("--ring", c1);
  root.setProperty("--gradient-primary", `linear-gradient(135deg, ${c1} 0%, ${c2} 100%)`);
}

function load(): Promise<void> {
  if (!inflight) {
    inflight = api
      .authConfig()
      .then((c) => {
        cached = { name: c.appName || FALLBACK, logoUrl: c.logoUrl || "" };
        applyAccents(c.brandColor || "", c.brandColor2 || "");
      })
      .catch(() => {
        cached = { name: FALLBACK, logoUrl: "" };
      })
      .then(() => {
        document.title = cached!.name;
        listeners.forEach((l) => l());
      });
  }
  return inflight;
}

// useBrandSource resolves the operator's brand from the server, re-rendering
// when it arrives.
function useBrandSource(): Brand {
  const [brand, setBrand] = useState<Brand>(cached ?? { name: FALLBACK, logoUrl: "" });
  useEffect(() => {
    if (cached !== null) {
      setBrand(cached);
      return;
    }
    const notify = () => setBrand(cached ?? { name: FALLBACK, logoUrl: "" });
    listeners.add(notify);
    load();
    return () => {
      listeners.delete(notify);
    };
  }, []);
  return brand;
}

// LogoContext carries the white-label logo URL to the shell marks. Empty string
// means "no custom logo" — render the gradient initial instead.
const LogoContext = createContext<string>("");

// useBrandLogo returns the operator's white-label logo URL, or "" when unset.
export function useBrandLogo(): string {
  return useContext(LogoContext);
}

// BrandBridge publishes the fetched brand into both the SDK name context (for
// branded plugin components) and the core logo context (for shell marks), and
// drives the accent-color side effect. Mount it near the app root.
export function BrandBridge({ children }: { children: ReactNode }) {
  const brand = useBrandSource();
  return (
    <BrandProvider name={brand.name}>
      <LogoContext.Provider value={brand.logoUrl}>{children}</LogoContext.Provider>
    </BrandProvider>
  );
}
