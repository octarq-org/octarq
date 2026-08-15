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

// applyAccents overrides the brand accent design tokens with the operator's
// colors. It writes only the two SEED tokens: everything else brand-tinted
// (--primary-hover, --accent-fg, --accent-soft, --accent-border, --ring,
// --gradient-primary, --info-*) is mixed from these in styles.css and follows
// automatically. Do NOT re-add derived tokens here — a value set in both places
// drifts, and the JS copy silently wins.
//
// The write lands inline on <html>, the same element `.dark` sits on, so the
// operator seed outranks both the :root and .dark declarations in either theme.
//
// Blank values REMOVE the inline seeds rather than returning early. Branding is
// per workspace and the shell switches workspaces in-app, so this runs again with
// whatever the new workspace has — and a workspace that sets no colour must fall
// back to the octarq default, not inherit the previous workspace's accent. An
// early return left the old colour painted on someone else's workspace.
function applyAccents(color: string, color2: string) {
  const root = document.documentElement.style;
  if (!color) {
    root.removeProperty("--primary");
    root.removeProperty("--accent-violet");
    return;
  }
  root.setProperty("--primary", color);
  root.setProperty("--accent-violet", color2 || color);
}

// applyFavicon points the tab icon at the operator's white-label logo. Without
// one the markup default in index.html (the Octarq mark) stands — so an OSS
// instance never pays for this, and a branded one doesn't show Octarq's glyph
// in the tab while showing the operator's logo in the app.
//
// The markup default is captured once, before the first override, so switching
// to a workspace with no logo restores it instead of keeping the previous
// workspace's icon in the tab.
let defaultFavicon: { href: string; type: string | null } | null = null;

function applyFavicon(logoUrl: string) {
  const existing = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (defaultFavicon === null && existing) {
    defaultFavicon = { href: existing.href, type: existing.getAttribute("type") };
  }
  if (!logoUrl) {
    if (existing && defaultFavicon) {
      existing.href = defaultFavicon.href;
      if (defaultFavicon.type) existing.setAttribute("type", defaultFavicon.type);
      else existing.removeAttribute("type");
    }
    return;
  }
  const link =
    existing ??
    document.head.appendChild(Object.assign(document.createElement("link"), { rel: "icon" }));
  link.href = logoUrl;
  link.removeAttribute("type"); // the operator's logo may be png/jpeg, not svg
  document.querySelector('link[rel="alternate icon"]')?.remove();
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
        applyFavicon(cached!.logoUrl);
        listeners.forEach((l) => l());
      });
  }
  return inflight;
}

// refreshBrand re-reads the brand and re-applies it. Branding is PER WORKSPACE,
// and `cached`/`inflight` are module-level: without an invalidation point the
// first workspace's colours, name and logo stayed on screen for the rest of the
// session.
//
// That was invisible while switching workspaces did a full window.location.reload()
// — the page teardown was the invalidation. App.tsx replaced it with an in-app
// remount, which is faster but leaves module state alive, so the shell has to say
// so explicitly. Call this whenever the active workspace changes or its branding
// is edited.
export function refreshBrand(): Promise<void> {
  cached = null;
  inflight = null;
  return load();
}

// useBrandSource resolves the operator's brand from the server, re-rendering
// when it arrives.
function useBrandSource(): Brand {
  const [brand, setBrand] = useState<Brand>(cached ?? { name: FALLBACK, logoUrl: "" });
  useEffect(() => {
    // Subscribe ALWAYS, including when the brand is already cached. Returning
    // early on a warm cache meant a mounted shell never heard about a later
    // refreshBrand(), so a workspace switch repainted the accent colours (a DOM
    // side effect) while the product name and logo stayed on the old workspace's.
    const notify = () => setBrand(cached ?? { name: FALLBACK, logoUrl: "" });
    listeners.add(notify);
    if (cached !== null) setBrand(cached);
    else load();
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
    <BrandProvider name={brand.name} onRefresh={refreshBrand}>
      <LogoContext.Provider value={brand.logoUrl}>{children}</LogoContext.Provider>
    </BrandProvider>
  );
}
