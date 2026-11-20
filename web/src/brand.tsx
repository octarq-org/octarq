import { useEffect, useState, ReactNode, createContext, useContext } from "react";
import { api } from "./api";
import { BrandProvider, useAppName, brandInitial, ARCH_PATH } from "../../packages/plugin-sdk/src";

export { useAppName, brandInitial };

const FALLBACK = "octarq";

type Brand = { name: string; logoUrl: string };
let cached: Brand | null = null;
let inflight: Promise<void> | null = null;
let brandColor = "";
let brandColor2 = "";
const listeners = new Set<() => void>();

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

const HEX_COLOR = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;

function validColor(color: string): string | null {
  const c = color.trim().toLowerCase();
  return HEX_COLOR.test(c) ? c : null;
}

function brandFavicon(color: string, color2: string): string | null {
  const c1 = validColor(color);
  if (!c1) return null;
  const c2 = validColor(color2) ?? c1;
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" fill="none">` +
    `<defs><linearGradient id="g" x1="10" y1="10" x2="54" y2="54" gradientUnits="userSpaceOnUse">` +
    `<stop stop-color="${c1}"/><stop offset="1" stop-color="${c2}"/>` +
    `</linearGradient></defs>` +
    `<path fill="url(#g)" fill-rule="evenodd" clip-rule="evenodd" d="${ARCH_PATH}"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

let defaultFavicon: { href: string; type: string | null } | null = null;

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
    link.removeAttribute("type");
  } else if (branded) {
    link.setAttribute("href", branded);
    link.setAttribute("type", "image/svg+xml");
  }
  document.querySelector('link[rel="alternate icon"]')?.remove();
}

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

export function refreshBrand(): Promise<void> {
  cached = null;
  inflight = null;
  return load();
}

function useBrandSource(): Brand {
  const [brand, setBrand] = useState<Brand>(cached ?? { name: FALLBACK, logoUrl: "" });
  useEffect(() => {
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

const LogoContext = createContext<string>("");

export function useBrandLogo(): string {
  return useContext(LogoContext);
}

export function BrandBridge({ children }: { children: ReactNode }) {
  const brand = useBrandSource();
  return (
    <BrandProvider name={brand.name} onRefresh={refreshBrand}>
      <LogoContext.Provider value={brand.logoUrl}>{children}</LogoContext.Provider>
    </BrandProvider>
  );
}
