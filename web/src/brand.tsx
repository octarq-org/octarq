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
import { BrandProvider, useAppName, brandInitial, ARCH_PATH } from "../../packages/plugin-sdk/src";

export { useAppName, brandInitial };

const FALLBACK = "octarq";

type Brand = { name: string; logoUrl: string };
let cached: Brand | null = null;
let inflight: Promise<void> | null = null;
// The brand colours live in module state alongside the name/logo cache so the
// favicon, theme-color meta and CSS seeds all read the SAME values on every
// recompute — a workspace switch repaints the tab and the chrome with exactly
// what reset the CSS variables.
let brandColor = "";
let brandColor2 = "";
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

// Brand colours are tenant-writable (the Pro white-label plugin writes them),
// so any value destined for markup goes through this gate: only #rgb / #rrggbb
// hex is accepted (case-insensitive), anything else counts as "no colour" and
// the caller falls back to the Octarq default. Pro's whitelabel plugin checks
// the same shape on write; this is defence in depth — a raw string must never
// be spliced into SVG markup.
const HEX_COLOR = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;

function validColor(color: string): string | null {
  const c = color.trim().toLowerCase();
  return HEX_COLOR.test(c) ? c : null;
}

// brandFavicon builds an SVG data URI for the Keystone Arch in the brand's
// accent colours: same structure as web/public/favicon.svg, with the shape from
// the single-source ARCH_PATH and the two hardcoded stop colours swapped for the
// brand's. Without a white-label logo this is what keeps one workspace's tab
// distinguishable from another's. The whole SVG is encodeURIComponent'd — a raw
// '#' in a data URI would read as a URL fragment separator and the icon would
// never load. Returns null when the colour isn't a valid hex triplet.
function brandFavicon(color: string, color2: string): string | null {
  const c1 = validColor(color);
  if (!c1) return null;
  const c2 = validColor(color2) ?? c1; // blank/invalid color2 → solid c1, like applyAccents' color2 || color
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" fill="none">` +
    `<defs><linearGradient id="g" x1="10" y1="10" x2="54" y2="54" gradientUnits="userSpaceOnUse">` +
    `<stop stop-color="${c1}"/><stop offset="1" stop-color="${c2}"/>` +
    `</linearGradient></defs>` +
    `<path fill="url(#g)" fill-rule="evenodd" clip-rule="evenodd" d="${ARCH_PATH}"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

let defaultFavicon: { href: string; type: string | null } | null = null;

// applyFavicon points the tab icon at, in order of precedence: the operator's
// white-label logo, a favicon generated from the brand's accent colour, or the
// markup default from index.html (the Octarq mark). A workspace that only sets
// brand colours therefore still gets a colour-matched icon instead of Octarq's
// glyph, while a workspace with neither keeps the default. The markup default
// is captured once, before the first override, so switching to an unbranded
// workspace restores it instead of keeping the previous workspace's icon in the
// tab.
function applyFavicon(logoUrl: string, color: string, color2: string) {
  const existing = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (defaultFavicon === null && existing) {
    defaultFavicon = { href: existing.href, type: existing.getAttribute("type") };
  }
  const branded = brandFavicon(color, color2);
  if (!logoUrl && !branded) {
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
  if (logoUrl) {
    link.setAttribute("href", logoUrl);
    link.removeAttribute("type"); // the operator's logo may be png/jpeg, not svg
  } else if (branded) {
    link.setAttribute("href", branded);
    link.setAttribute("type", "image/svg+xml");
  }
  document.querySelector('link[rel="alternate icon"]')?.remove();
}

// applyThemeColor keeps the browser chrome (mobile address bar, tab strip) in
// the brand's accent colour. index.html ships no theme-color meta, so there is
// no markup default to restore: "no brand colour" REMOVES the meta rather than
// leaving an empty or stale value behind after a workspace switch.
function applyThemeColor(color: string) {
  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
  if (!color) {
    meta?.remove();
    return;
  }
  if (meta) meta.content = color;
  else
    document.head.appendChild(
      Object.assign(document.createElement("meta"), { name: "theme-color", content: color }),
    );
}

function load(): Promise<void> {
  if (!inflight) {
    inflight = api
      .authConfig()
      .then((c) => {
        cached = { name: c.appName || FALLBACK, logoUrl: c.logoUrl || "" };
        brandColor = c.brandColor || "";
        brandColor2 = c.brandColor2 || "";
        applyAccents(brandColor, brandColor2);
      })
      .catch(() => {
        // The fetch failed: this workspace's branding is unknown, so the whole
        // surface falls back to the colourless Octarq default. In particular
        // the previous workspace's colour must not linger in the tab — that
        // exact failure is what the test file's header documents.
        cached = { name: FALLBACK, logoUrl: "" };
        brandColor = "";
        brandColor2 = "";
        applyAccents("", "");
      })
      .then(() => {
        document.title = cached!.name;
        applyFavicon(cached!.logoUrl, brandColor, brandColor2);
        applyThemeColor(validColor(brandColor) ?? "");
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
