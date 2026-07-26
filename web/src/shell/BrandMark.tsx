// BrandMark is the single brand glyph used across the shell (app loader, login
// card, top bar). Three tiers, most specific first:
//
//   1. the operator's white-label logo, when the Pro white-label plugin set one
//   2. the Octarq mark, when the instance still calls itself Octarq
//   3. the gradient initial of whatever the product was renamed to
//
// Tier 3 matters: an operator who renames the app but sets no logo must not get
// Octarq's glyph on their product. The gradient behind tiers 2 and 3 reads the
// --gradient-primary design token, so white-label accent colors recolor it.
import { useAppName, brandInitial, useBrandLogo } from "../brand";
import { OctarqMark } from "./OctarqMark";

type Size = "sm" | "md" | "lg";

const BOX: Record<Size, string> = {
  sm: "h-9 w-9",
  md: "h-10 w-10",
  lg: "h-12 w-12",
};

const TEXT: Record<Size, string> = {
  sm: "text-sm",
  md: "text-base",
  lg: "text-xl",
};

// The glyph sits at ~60% of the tile so it keeps optical padding at every size.
const GLYPH: Record<Size, string> = {
  sm: "h-[22px] w-[22px]",
  md: "h-6 w-6",
  lg: "h-[30px] w-[30px]",
};

const DEFAULT_NAME = "octarq";

export function BrandMark({ size = "sm", className = "" }: { size?: Size; className?: string }) {
  const appName = useAppName();
  const logoUrl = useBrandLogo();
  const box = BOX[size];

  if (logoUrl) {
    return (
      <img
        src={logoUrl}
        alt={appName}
        className={`${box} rounded-xl object-contain shadow-glow ${className}`}
      />
    );
  }

  const isOctarq = appName.trim().toLowerCase() === DEFAULT_NAME;

  return (
    <div
      className={`${box} brand-gradient flex items-center justify-center rounded-xl shadow-glow ${className}`}
    >
      {isOctarq ? (
        <OctarqMark className={`${GLYPH[size]} text-white`} strokeWidth={8} />
      ) : (
        <span className={`font-display ${TEXT[size]} font-extrabold text-white`}>{brandInitial(appName)}</span>
      )}
    </div>
  );
}
