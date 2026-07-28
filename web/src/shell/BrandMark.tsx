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
import { useAppName, useBrandLogo } from "../brand";
import { BrandGlyph, BrandGlyphSize } from "@octarq/plugin-sdk";

export type Size = BrandGlyphSize;

export function BrandMark({ size = "sm", className = "" }: { size?: Size; className?: string }) {
  const appName = useAppName();
  const logoUrl = useBrandLogo();

  return <BrandGlyph appName={appName} logoUrl={logoUrl} size={size} className={className} />;
}

