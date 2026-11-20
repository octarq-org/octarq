export type TokenGroup =
  | "surface"
  | "accent"
  | "status"
  | "line"
  | "radius"
  | "typography"
  | "elevation"
  | "motion";

export interface TokenDef {
  name: string;
  group: TokenGroup;
  root: boolean;
  dark: boolean;
  themed: boolean;
  seed?: boolean;
}

export const TOKENS: TokenDef[] = [
  { name: "--radius", group: "radius", root: true, dark: false, themed: false },
  { name: "--row-height", group: "line", root: true, dark: false, themed: false },

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
  { name: "--sidebar", group: "surface", root: true, dark: true, themed: true },
  { name: "--sidebar-foreground", group: "surface", root: true, dark: true, themed: true },
  { name: "--sidebar-accent", group: "surface", root: true, dark: true, themed: true },
  { name: "--sidebar-accent-foreground", group: "surface", root: true, dark: true, themed: true },
  { name: "--sidebar-border", group: "line", root: true, dark: true, themed: true },
  { name: "--sidebar-ring", group: "line", root: true, dark: true, themed: true },

  { name: "--primary", group: "accent", root: true, dark: true, themed: true, seed: true },
  { name: "--accent-violet", group: "accent", root: true, dark: false, themed: true, seed: true },
  { name: "--primary-foreground", group: "accent", root: true, dark: false, themed: true, seed: true },
  { name: "--primary-hover", group: "accent", root: true, dark: true, themed: true },
  { name: "--primary-solid", group: "accent", root: true, dark: true, themed: true },
  { name: "--primary-solid-hover", group: "accent", root: true, dark: true, themed: true },
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
  { name: "--danger-solid", group: "status", root: true, dark: true, themed: true },
  { name: "--danger-solid-fg", group: "status", root: true, dark: true, themed: true },
  { name: "--review-bg", group: "status", root: true, dark: true, themed: true },
  { name: "--review-border", group: "status", root: true, dark: true, themed: true },
  { name: "--review-fg", group: "status", root: true, dark: true, themed: true },

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

  { name: "--font-sans", group: "typography", root: false, dark: false, themed: true },
  { name: "--font-display", group: "typography", root: false, dark: false, themed: true },
  { name: "--font-mono", group: "typography", root: false, dark: false, themed: true },

  { name: "--shadow-card", group: "elevation", root: true, dark: true, themed: false },
  { name: "--shadow-float", group: "elevation", root: true, dark: true, themed: false },
  { name: "--shadow-accent", group: "elevation", root: true, dark: true, themed: false },

  { name: "--ease-out", group: "motion", root: true, dark: false, themed: false },
  { name: "--ease-in-out", group: "motion", root: true, dark: false, themed: false },
  { name: "--dur-fast", group: "motion", root: true, dark: false, themed: false },
  { name: "--dur-base", group: "motion", root: true, dark: false, themed: false },
  { name: "--dur-slow", group: "motion", root: true, dark: false, themed: false },
];

export const SEED_TOKENS: TokenDef[] = TOKENS.filter((t) => t.seed);

export function tokensByGroup(group: TokenGroup): TokenDef[] {
  return TOKENS.filter((t) => t.group === group);
}

export function tokenByName(name: string): TokenDef | undefined {
  return TOKENS.find((t) => t.name === name);
}

export function themeAlias(token: TokenDef): string | null {
  if (!token.themed) return null;
  return token.group === "radius" || token.group === "typography"
    ? token.name
    : `--color-${token.name.slice(2)}`;
}

export function readTokenValues(names: string[] = TOKENS.map((t) => t.name)): Record<string, string> {
  if (typeof document === "undefined") return {};
  const computed = getComputedStyle(document.documentElement);
  const out: Record<string, string> = {};
  for (const name of names) out[name] = computed.getPropertyValue(name).trim();
  return out;
}
