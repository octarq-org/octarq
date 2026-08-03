// Shell brand glyph wrapper (custom logo -> Octarq mark -> app initial fallback).
import { useAppName, useBrandLogo } from "../brand";
import { BrandGlyph, BrandGlyphSize } from "@octarq/plugin-sdk";

export type Size = BrandGlyphSize;

export function BrandMark({ size = "sm", className = "" }: { size?: Size; className?: string }) {
  const appName = useAppName();
  const logoUrl = useBrandLogo();

  return <BrandGlyph appName={appName} logoUrl={logoUrl} size={size} className={className} />;
}

