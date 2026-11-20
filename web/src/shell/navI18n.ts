import type { TFunc } from "@octarq/plugin-sdk";

export function translateGroupLabel(t: TFunc, label: string): string {
  return t(`groups.${label}`, label);
}

export function translateNavItemLabel(t: TFunc, id: string, label: string): string {
  return t(`nav.${id}`, label);
}

export function translateAreaTitle(t: TFunc, id: string, title: string): string {
  return t(`areas.${id}.title`, title);
}

export function translateAreaSubtitle(t: TFunc, id: string, subtitle: string): string {
  return t(`areas.${id}.subtitle`, subtitle);
}
