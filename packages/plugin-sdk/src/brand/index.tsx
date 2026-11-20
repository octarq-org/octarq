import { createContext, useContext, ReactNode, CSSProperties } from "react";

const FALLBACK = "octarq";
const Ctx = createContext<string>(FALLBACK);

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

export function useBrandRefresh(): () => void {
  return useContext(RefreshCtx);
}

export function useAppName(): string {
  return useContext(Ctx);
}

export function brandInitial(name: string): string {
  return (name.trim()[0] || "O").toUpperCase();
}

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

  return (
    <div
      className={`${box} brand-gradient flex items-center justify-center rounded-lg ${className}`}
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

