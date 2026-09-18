// Design-token inventory.
//
// Names, groups and roles only — deliberately NOT values. The values are
// declared once, in the host stylesheet (web/src/styles.css), and this module
// must not become a second copy of them: a duplicated brand hex is exactly what
// the colour lint forbids, and a copy would also go stale the moment a
// white-label rebrand overrides --primary at runtime. Consumers that need a
// value read it from the live DOM with readTokenValues().
//
// What this buys: a consumer — the UI workbench's token inspector, or a plugin
// choosing a token by name — can enumerate the token set instead of scraping the
// stylesheet, tell the three rebrandable seeds from the tokens derived from them,
// and tell theme-invariant tokens from ones with a dark override.
//
// `root` / `dark` / `themed` record WHERE each token is declared, not what it is
// set to. They are listed explicitly rather than derived, so the parity test
// (web/src/test/tokensParity.test.ts) compares real data against the stylesheet
// instead of re-running this module's own assumptions back at itself.
export type TokenGroup = "surface" | "accent" | "status" | "line" | "radius" | "typography";

export interface TokenDef {
  // The CSS custom property name, e.g. "--background".
  name: string;
  group: TokenGroup;
  // Declared in `:root` (the light values).
  root: boolean;
  // Overridden in `.dark`; false means it is identical in both modes.
  dark: boolean;
  // Exposed to Tailwind utilities through `@theme inline`.
  themed: boolean;
  // One of the three literals a white-label rebrand overrides at runtime
  // (brand.tsx applyAccents). Every other brand-tinted token derives from
  // --primary, which is what makes a rebrand a two-variable write.
  seed?: boolean;
}

export const TOKENS: TokenDef[] = [
  { name: "--radius", group: "radius", root: true, dark: false, themed: false },

  { name: "--background", group: "surface", root: true, dark: true, themed: true },
  { name: "--foreground", group: "surface", root: true, dark: true, themed: true },
  { name: "--card", group: "surface", root: true, dark: true, themed: true },
  { name: "--card-foreground", group: "surface", root: true, dark: true, themed: true },
  { name: "--popover", group: "surface", root: true, dark: true, themed: true },
  { name: "--popover-foreground", group: "surface", root: true, dark: true, themed: true },
  { name: "--muted", group: "surface", root: true, dark: true, themed: true },
  { name: "--muted-foreground", group: "surface", root: true, dark: true, themed: true },
  { name: "--surface-hover", group: "surface", root: true, dark: true, themed: true },
  { name: "--well", group: "surface", root: true, dark: true, themed: true },

  { name: "--primary", group: "accent", root: true, dark: true, themed: true, seed: true },
  { name: "--accent-violet", group: "accent", root: true, dark: false, themed: true, seed: true },
  { name: "--primary-foreground", group: "accent", root: true, dark: false, themed: true, seed: true },
  { name: "--primary-hover", group: "accent", root: true, dark: true, themed: true },
  { name: "--accent-indigo", group: "accent", root: true, dark: false, themed: true },
  { name: "--accent-fg", group: "accent", root: true, dark: true, themed: true },
  { name: "--accent-soft", group: "accent", root: true, dark: true, themed: true },
  { name: "--accent-border", group: "accent", root: true, dark: true, themed: true },
  { name: "--gradient-primary", group: "accent", root: true, dark: false, themed: false },

  { name: "--info-bg", group: "status", root: true, dark: true, themed: true },
  { name: "--info-border", group: "status", root: true, dark: true, themed: true },
  { name: "--info-fg", group: "status", root: true, dark: true, themed: true },
  { name: "--success-bg", group: "status", root: true, dark: true, themed: true },
  { name: "--success-border", group: "status", root: true, dark: true, themed: true },
  { name: "--success-fg", group: "status", root: true, dark: true, themed: true },
  { name: "--warning-bg", group: "status", root: true, dark: true, themed: true },
  { name: "--warning-border", group: "status", root: true, dark: true, themed: true },
  { name: "--warning-fg", group: "status", root: true, dark: true, themed: true },
  { name: "--danger-bg", group: "status", root: true, dark: true, themed: true },
  { name: "--danger-border", group: "status", root: true, dark: true, themed: true },
  { name: "--danger-fg", group: "status", root: true, dark: true, themed: true },

  { name: "--border", group: "line", root: true, dark: true, themed: true },
  { name: "--border-strong", group: "line", root: true, dark: true, themed: true },
  { name: "--input", group: "line", root: true, dark: true, themed: true },
  { name: "--ring", group: "line", root: true, dark: true, themed: true },

  { name: "--radius-sm", group: "radius", root: false, dark: false, themed: true },
  { name: "--radius-md", group: "radius", root: false, dark: false, themed: true },
  { name: "--radius-lg", group: "radius", root: false, dark: false, themed: true },
  { name: "--radius-xl", group: "radius", root: false, dark: false, themed: true },
  { name: "--radius-2xl", group: "radius", root: false, dark: false, themed: true },
  { name: "--radius-3xl", group: "radius", root: false, dark: false, themed: true },

  { name: "--font-mono", group: "typography", root: false, dark: false, themed: true },
];

// The three literals a rebrand may override. Everything brand-tinted is derived
// from --primary, so this is the whole write surface.
export const SEED_TOKENS: TokenDef[] = TOKENS.filter((t) => t.seed);

export function tokensByGroup(group: TokenGroup): TokenDef[] {
  return TOKENS.filter((t) => t.group === group);
}

export function tokenByName(name: string): TokenDef | undefined {
  return TOKENS.find((t) => t.name === name);
}

// The `@theme inline` key a token reaches Tailwind through: colours and status
// families become --color-<name>, while the radius/font scales keep their own.
// Null when the token has no utility bridge.
export function themeAlias(token: TokenDef): string | null {
  if (!token.themed) return null;
  return token.group === "radius" || token.group === "typography"
    ? token.name
    : `--color-${token.name.slice(2)}`;
}

// Current computed values, keyed by token name. Read from the live document on
// purpose: it is the only source that reflects a runtime rebrand (applyAccents
// writes inline on <html>) and the active light/dark mode. Returns {} outside a
// DOM (SSR, tests).
export function readTokenValues(names: string[] = TOKENS.map((t) => t.name)): Record<string, string> {
  if (typeof document === "undefined") return {};
  const computed = getComputedStyle(document.documentElement);
  const out: Record<string, string> = {};
  for (const name of names) out[name] = computed.getPropertyValue(name).trim();
  return out;
}
