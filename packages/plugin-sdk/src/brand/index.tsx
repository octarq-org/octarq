// @octarq/plugin-sdk/brand — the brand-name context.
//
// The product name is a host-app runtime setting (fetched from the server). The
// SDK only needs to *read* it (e.g. LockedFeature's upsell copy), so it exposes
// a tiny context the app populates via <BrandProvider name={…}>. Plugins call
// useAppName() and get the operator's brand without importing anything
// app-internal. Defaults to "octarq" when no provider is mounted.
import { createContext, useContext, ReactNode, CSSProperties } from "react";

const FALLBACK = "octarq";
const Ctx = createContext<string>(FALLBACK);

// The host's "re-read the brand from the server and re-apply it" callback. The
// SDK cannot own this: the fetch, the module-level cache and the CSS seeds all
// live in the app. A plugin that CHANGES the branding (the white-label editor)
// needs to tell the shell to catch up, otherwise its own save leaves every other
// surface — sidebar mark, page title, accent colours — showing the old brand
// until a manual reload. No-op when the host mounts no provider.
const RefreshCtx = createContext<() => void>(() => {});

export function BrandProvider({
  name,
  onRefresh,
  children,
}: {
  name: string;
  onRefresh?: () => void;
  children: ReactNode;
}) {
  return (
    <Ctx.Provider value={name || FALLBACK}>
      <RefreshCtx.Provider value={onRefresh ?? (() => {})}>{children}</RefreshCtx.Provider>
    </Ctx.Provider>
  );
}

// useBrandRefresh returns a callback that re-reads the operator's branding and
// re-applies it across the shell. Call it after saving a branding change.
export function useBrandRefresh(): () => void {
  return useContext(RefreshCtx);
}

// useAppName returns the operator's product name (or "octarq" if unset).
export function useAppName(): string {
  return useContext(Ctx);
}

// brandInitial is the single-character logo glyph derived from the app name.
export function brandInitial(name: string): string {
  return (name.trim()[0] || "O").toUpperCase();
}

// ARCH_PATH is the SVG path string for the Keystone Arch glyph.
export const ARCH_PATH =
  "M10,26 C10,13.8 19.8,4 32,4 C44.2,4 54,13.8 54,26 V56 H42 V28 C42,23.6 37.6,20 32,20 C26.4,20 22,23.6 22,28 V56 H10 V26 Z M26,33 H38 V46 H26 V33 Z";

export function OctarqMark({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" fill="currentColor" aria-hidden="true" className={className}>
      <path fillRule="evenodd" clipRule="evenodd" d={ARCH_PATH} />
    </svg>
  );
}

export type BrandGlyphSize = "sm" | "md" | "lg";

const BOX: Record<BrandGlyphSize, string> = {
  sm: "h-9 w-9",
  md: "h-10 w-10",
  lg: "h-12 w-12",
};

const TEXT: Record<BrandGlyphSize, string> = {
  sm: "text-sm",
  md: "text-base",
  lg: "text-xl",
};

const GLYPH: Record<BrandGlyphSize, string> = {
  sm: "h-[22px] w-[22px]",
  md: "h-6 w-6",
  lg: "h-[30px] w-[30px]",
};

export interface BrandGlyphProps {
  appName: string;
  logoUrl?: string | null;
  size?: BrandGlyphSize;
  className?: string;
  style?: CSSProperties;
}

// BrandGlyph renders the brand mark following three-tier logic:
//   1. White-label logo URL if provided
//   2. Keystone Arch glyph if name is default "octarq"
//   3. First-letter gradient box if name has been renamed
export function BrandGlyph({
  appName,
  logoUrl,
  size = "sm",
  className = "",
  style,
}: BrandGlyphProps) {
  const box = BOX[size];

  if (logoUrl) {
    return (
      <img
        src={logoUrl}
        alt={appName || FALLBACK}
        className={`${box} rounded-xl object-contain ${className}`}
        style={style}
      />
    );
  }

  const isOctarq = (appName || "").trim().toLowerCase() === FALLBACK;

  // Brand halo, inlined as a component class (light + dark) since the global .shadow-glow utility was removed.
  return (
    <div
      className={`${box} brand-gradient flex items-center justify-center rounded-xl shadow-[0_6px_20px_-8px_color-mix(in_oklab,var(--primary)_35%,transparent)] dark:shadow-[0_0_0_1px_color-mix(in_oklab,var(--primary)_40%,transparent),0_8px_40px_-8px_color-mix(in_oklab,var(--primary)_45%,transparent)] ${className}`}
      style={style}
    >
      {isOctarq ? (
        <OctarqMark className={`${GLYPH[size]} text-white`} />
      ) : (
        <span className={`font-display ${TEXT[size]} font-extrabold text-white`}>
          {brandInitial(appName)}
        </span>
      )}
    </div>
  );
}

