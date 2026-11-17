import type { TFunc } from "@octarq/plugin-sdk";
import type { Area } from "./areas";
import { translateAreaTitle, translateGroupLabel, translateNavItemLabel } from "./navI18n";

export interface BreadcrumbCrumb {
  label: string;
  path?: string;
}

export function resolveBreadcrumbs({
  pathname,
  areas,
  settingsArea,
  helpArea,
  t,
}: {
  pathname: string;
  areas: Area[];
  settingsArea?: Area;
  helpArea?: Area;
  t: TFunc;
}): BreadcrumbCrumb[] {
  const candidates: Array<{ area: Area; groupLabel: string; itemId: string; itemLabel: string; path: string }> = [];

  const collect = (area: Area | undefined) => {
    if (!area) return;
    for (const group of area.groups) {
      for (const item of group.items) {
        if (item.external) continue;
        candidates.push({
          area,
          groupLabel: group.label,
          itemId: item.id,
          itemLabel: item.label,
          path: item.path,
        });
      }
    }
  };

  for (const a of areas) collect(a);
  collect(settingsArea);
  collect(helpArea);

  candidates.sort((a, b) => b.path.length - a.path.length);

  const hit = candidates.find(
    (c) => pathname === c.path || pathname.startsWith(c.path + "/"),
  );

  if (!hit) {
    if (pathname.startsWith("/settings") && settingsArea) {
      return [{ label: translateAreaTitle(t, settingsArea.id, settingsArea.title), path: "/settings/general" }];
    }
    if ((pathname.startsWith("/help") || pathname.startsWith("/admin/help")) && helpArea) {
      return [{ label: translateAreaTitle(t, helpArea.id, helpArea.title), path: "/help" }];
    }
    const first = areas[0];
    return first ? [{ label: translateAreaTitle(t, first.id, first.title), path: "/overview" }] : [];
  }

  const areaTitle = translateAreaTitle(t, hit.area.id, hit.area.title);
  const groupTitle = translateGroupLabel(t, hit.groupLabel);
  const itemTitle = translateNavItemLabel(t, hit.itemId, hit.itemLabel);
  const firstPath = hit.area.groups[0]?.items[0]?.path ?? hit.path;

  const crumbs: BreadcrumbCrumb[] = [{ label: areaTitle, path: firstPath }];
  if (groupTitle && groupTitle.toLowerCase() !== areaTitle.toLowerCase() && groupTitle.toLowerCase() !== itemTitle.toLowerCase()) {
    crumbs.push({ label: groupTitle });
  }
  if (itemTitle.toLowerCase() !== areaTitle.toLowerCase()) {
    crumbs.push({ label: itemTitle, path: hit.path });
  }
  return crumbs;
}
