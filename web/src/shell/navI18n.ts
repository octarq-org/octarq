import type { TFunc } from "@octarq/plugin-sdk";

/**
 * Translate a navigation group label (sidebar category heading).
 * Lookups use `groups.<label>`, falling back to `label`.
 */
export function translateGroupLabel(t: TFunc, label: string): string {
  return t(`groups.${label}`, label);
}

/**
 * Translate a navigation item label (sidebar / command palette item).
 * Lookups use `nav.<id>`, falling back to `label`.
 */
export function translateNavItemLabel(t: TFunc, id: string, label: string): string {
  return t(`nav.${id}`, label);
}

/**
 * Translate a top-level area title.
 * Lookups use `areas.<id>.title`, falling back to `title`.
 */
export function translateAreaTitle(t: TFunc, id: string, title: string): string {
  return t(`areas.${id}.title`, title);
}

/**
 * Translate a top-level area subtitle.
 * Lookups use `areas.<id>.subtitle`, falling back to `subtitle`.
 */
export function translateAreaSubtitle(t: TFunc, id: string, subtitle: string): string {
  return t(`areas.${id}.subtitle`, subtitle);
}
