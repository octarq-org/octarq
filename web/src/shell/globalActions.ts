import type { ElementType } from "react";
import { Plus } from "lucide-react";
import { Action } from "../api";
import { roleSatisfies } from "./role";
import { menuIcon } from "./areas";

export interface CommandPaletteItem {
  id: string;
  label: string;
  path: string;
  Icon?: ElementType;
  iconStr?: string;
  isAction: boolean;
  order?: number;
  category?: string;
  area?: string;
  group?: string;
}

/**
 * Filters raw actions by requiredRole against user's org role or instance admin bypass.
 */
export function visibleActions(
  actions: Action[] | undefined,
  role: string | undefined,
  isInstanceAdmin: boolean,
): Action[] {
  if (!actions || !Array.isArray(actions)) return [];
  return actions.filter((a) => roleSatisfies(a.requiredRole, role, isInstanceAdmin));
}

/**
 * Merges action candidates ahead of navigation candidates for CommandPalette.
 */
export function mergeCommandItems(
  actions: Action[] = [],
  navItems: CommandPaletteItem[] = [],
): CommandPaletteItem[] {
  const actionItems: CommandPaletteItem[] = (actions || [])
    .map((a) => {
      const KeyIcon = menuIcon(a.icon);
      return {
        id: a.id,
        label: a.label,
        path: a.path,
        Icon: KeyIcon ?? Plus,
        iconStr: KeyIcon ? undefined : a.icon,
        isAction: true,
        order: a.order ?? 0,
        category: a.category,
      };
    })
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0));

  return [...actionItems, ...navItems];
}
