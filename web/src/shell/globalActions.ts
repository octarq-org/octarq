import type { ElementType } from "react";
import { Download, Plus, Sparkles } from "lucide-react";
import { Action } from "../api";
import { roleSatisfies } from "./role";
import { menuIcon } from "./areas";

export type QuickActionType = "export_data" | "open_ai_chat";

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
  actionType?: QuickActionType | string;
  actionHandler?: () => void;
  shortcut?: string;
}

export interface BuiltinQuickActionOptions {
  t?: (key: string, fallback?: string) => string;
  onNavigate?: (path: string) => void;
  onOpenCopilot?: () => void;
  onExportData?: () => void;
}

export function createBuiltinQuickActions(
  options?: BuiltinQuickActionOptions,
): CommandPaletteItem[] {
  const t = options?.t || ((_key: string, fb?: string) => fb || _key);
  return [
    {
      id: "action.export_data",
      label: t("command.action.exportData", "Export workspace data"),
      path: "/settings/workspace",
      Icon: Download,
      isAction: true,
      actionType: "export_data",
      order: 3,
      category: "Quick Actions",
      shortcut: "⌘E",
      actionHandler: () => {
        if (typeof window !== "undefined") {
          window.dispatchEvent(new CustomEvent("octarq:export-data"));
        }
        if (options?.onExportData) {
          options.onExportData();
        } else if (options?.onNavigate) {
          options.onNavigate("/settings/workspace");
        }
      },
    },
    {
      id: "action.ask_copilot",
      label: t("command.action.askCopilot", "Open AI Copilot"),
      path: "",
      Icon: Sparkles,
      isAction: true,
      actionType: "open_ai_chat",
      order: 4,
      category: "Quick Actions",
      shortcut: "⌘J",
      actionHandler: () => {
        if (typeof window !== "undefined") {
          window.dispatchEvent(new CustomEvent("octarq:open-copilot"));
        }
        if (options?.onOpenCopilot) {
          options.onOpenCopilot();
        }
      },
    },
  ];
}

export function visibleActions(
  actions: Action[] | undefined,
  role: string | undefined,
  isInstanceAdmin: boolean,
): Action[] {
  if (!actions || !Array.isArray(actions)) return [];
  return actions.filter((a) => roleSatisfies(a.requiredRole, role, isInstanceAdmin));
}

export interface MergeCommandOptions extends BuiltinQuickActionOptions {
  includeBuiltin?: boolean;
}

export function mergeCommandItems(
  actions: Action[] = [],
  navItems: CommandPaletteItem[] = [],
  options?: boolean | MergeCommandOptions,
): CommandPaletteItem[] {
  const includeBuiltin = typeof options === "boolean" ? options : !!options?.includeBuiltin;
  const opts = typeof options === "object" ? options : undefined;

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

  const builtinItems = includeBuiltin ? createBuiltinQuickActions(opts) : [];

  return [...actionItems, ...builtinItems, ...navItems];
}
