// @octarq/plugin-sdk/brand — the brand-name context.
//
// The product name is a host-app runtime setting (fetched from the server). The
// SDK only needs to *read* it (e.g. LockedFeature's upsell copy), so it exposes
// a tiny context the app populates via <BrandProvider name={…}>. Plugins call
// useAppName() and get the operator's brand without importing anything
// app-internal. Defaults to "octarq" when no provider is mounted.
import { createContext, useContext, ReactNode } from "react";

const FALLBACK = "octarq";
const Ctx = createContext<string>(FALLBACK);

export function BrandProvider({ name, children }: { name: string; children: ReactNode }) {
  return <Ctx.Provider value={name || FALLBACK}>{children}</Ctx.Provider>;
}

// useAppName returns the operator's product name (or "octarq" if unset).
export function useAppName(): string {
  return useContext(Ctx);
}

// brandInitial is the single-character logo glyph derived from the app name.
export function brandInitial(name: string): string {
  return (name.trim()[0] || "O").toUpperCase();
}
