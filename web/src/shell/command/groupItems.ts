import type { CommandPaletteItem } from "../globalActions";

export interface CommandGroup {
  key: string;
  label: string;
  items: CommandPaletteItem[];
}

export function groupCommandItems(
  items: CommandPaletteItem[],
  quickActionsLabel: string,
  areaOrder: string[],
): CommandGroup[] {
  const actions: CommandPaletteItem[] = [];
  const byArea = new Map<string, CommandPaletteItem[]>();

  for (const item of items) {
    if (item.isAction) {
      actions.push(item);
      continue;
    }
    const areaKey = item.area || quickActionsLabel;
    const bucket = byArea.get(areaKey);
    if (bucket) {
      bucket.push(item);
    } else {
      byArea.set(areaKey, [item]);
    }
  }

  const groups: CommandGroup[] = [];
  if (actions.length > 0) {
    groups.push({
      key: "quick-actions",
      label: quickActionsLabel,
      items: actions,
    });
  }

  const seen = new Set<string>();
  for (const title of areaOrder) {
    const areaItems = byArea.get(title);
    if (areaItems && areaItems.length > 0) {
      groups.push({
        key: `area:${title}`,
        label: title,
        items: areaItems,
      });
      seen.add(title);
    }
  }

  for (const [title, areaItems] of byArea.entries()) {
    if (seen.has(title) || areaItems.length === 0) continue;
    groups.push({
      key: `area:${title}`,
      label: title,
      items: areaItems,
    });
  }

  return groups;
}
