import {
  Bell,
  Book,
  BookOpen,
  Bot,
  Boxes,
  CreditCard,
  Database,
  Globe,
  HardDrive,
  Key,
  KeyRound,
  LayoutDashboard,
  LineChart,
  Link2,
  Mail,
  Palette,
  Puzzle,
  ScrollText,
  ShoppingCart,
  Ticket,
  Send,
  Server,
  Settings,
  Shield,
  ShieldAlert,
  Sparkles,
  Store,
  User,
  Users,
  Wallet,
  Webhook,
  Workflow,
} from "lucide-react";
import type { UIArea } from "@octarq/plugin-sdk";
import type { MenuItem } from "../api";
import { NavigationTree, BUILTIN_AREA_GROUPS, BuildNavigationTreeOptions } from "./NavigationTree";
export { NavigationTree, BUILTIN_AREA_GROUPS };
export type { BuildNavigationTreeOptions };
export { useNavigation, clearCachedNav, readCachedNav, writeCachedNav } from "./useNavigation";
export type { UseNavigationOptions, CachedNav } from "./useNavigation";

// ─── Area definitions ──────────────────────────────────────────────────────

// Built-in area IDs come from STATIC_AREAS; plugins can contribute new top-level areas via UIPlugin.areas.
export type AreaId = string;

export interface NavItem {
  id: string;
  label: string;
  Icon: React.ElementType;
  iconStr?: string;
  path: string;
  badge?: string | number;
  // Full-page navigation to a different basename (e.g. the /instance console):
  // rendered as a plain <a href> by the shell instead of a router NavLink.
  external?: boolean;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export interface Area {
  id: AreaId;
  title: string;
  subtitle: string;
  Icon: React.ElementType;
  groups: NavGroup[];
}

export const STATIC_AREAS: Area[] = [
  {
    id: "operations",
    title: "Operations",
    subtitle: "Daily traffic & communication",
    Icon: Workflow,
    groups: [
      {
        label: "Workspace",
        items: [
          { id: "overview", label: "Overview", Icon: LayoutDashboard, path: "/overview" },
        ],
      },
    ],
  },
  {
    id: "assets",
    title: "Infrastructure",
    subtitle: "Servers, network & databases",
    Icon: Boxes,
    groups: [],
  },
];

export const SETTINGS_AREA: Area = {
  id: "settings",
  title: "Settings",
  subtitle: "Workspace & profile configurations",
  Icon: Settings,
  groups: [
    {
      label: "Instance",
      items: [
        {
          id: "instance-console",
          label: "Instance Management",
          Icon: Server,
          path: "/instance",
          external: true,
        },
      ],
    },
    {
      label: "Workspace",
      items: [
        { id: "general",       label: "General",     Icon: Settings,    path: "/settings/general" },
        { id: "plugins",       label: "Features",    Icon: Puzzle,      path: "/settings/plugins" },
        { id: "members",       label: "Members",     Icon: Users,       path: "/settings/members" },
        { id: "webhooks",      label: "Webhooks",    Icon: Webhook,     path: "/settings/webhooks" },
        { id: "notifications", label: "Alerts",      Icon: Bell,        path: "/settings/notifications" },
      ],
    },
    {
      label: "Personal",
      items: [
        { id: "profile",  label: "My Profile", Icon: User,      path: "/settings/profile" },
        { id: "security", label: "Security",   Icon: Shield,    path: "/settings/security" },
        { id: "tokens",   label: "API Tokens", Icon: KeyRound,  path: "/settings/tokens" },
        { id: "appearance", label: "Appearance", Icon: Palette, path: "/settings/appearance" },
      ],
    },
  ],
};

// Derived from the area/menu data — never reintroduce a parallel hardcoded
// path→area map. Callers pass the merged runtime areas so plugin-contributed
// paths resolve too; the default covers the static-only case.
export function areaForPath(path: string, areas: Area[] = STATIC_AREAS): AreaId {
  // Settings live in their own area (SETTINGS_AREA), not the areas list.
  if (path.startsWith("/settings")) return "settings";
  if (path.startsWith("/help") || path.startsWith("/admin/help")) return "help";
  const hit = areas
    .flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ prefix: i.path, area: a.id }))))
    .sort((x, y) => y.prefix.length - x.prefix.length) // longest prefix wins
    .find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));
  return hit?.area ?? "operations";
}

// Placement keyword for sidebar footer items. Not a real Area id.
export const FOOTER_PLACEMENT = "footer";

// Maps a dynamic menu category to an area deterministically via NavigationTree.
// Eliminates keyword substring heuristics in favor of exact matching against area IDs and declared groups.
export function areaForCategory(cat?: string, pluginAreas: UIArea[] = []): AreaId {
  return NavigationTree.resolveCategoryToArea(cat, pluginAreas);
}

// Maps a dynamic menu item (prioritizing explicit area metadata) to an area deterministically via NavigationTree.
export function areaForMenu(menu: MenuItem, pluginAreas: UIArea[] = []): AreaId {
  return NavigationTree.resolveMenuToArea(menu, pluginAreas);
}

// ─── Plugin-contributed icons & areas ───────────────────────────────────────

// The contract keeps plugin icons (UIArea.icon, PluginMenuItem.icon) as string
// keys so it stays icon-library-free; the app maps them to lucide HERE — the
// single icon-key→component table for both plugin areas and plugin menu items
// (core plugins use these keys too). A menu icon that isn't a known key is
// rendered literally (emoji); an unknown AREA icon falls back to Puzzle.
const PLUGIN_ICONS: Record<string, React.ElementType> = {
  bell: Bell,
  book: Book,
  "book-open": BookOpen,
  bot: Bot,
  boxes: Boxes,
  "credit-card": CreditCard,
  database: Database,
  globe: Globe,
  "hard-drive": HardDrive,
  key: Key,
  "key-round": KeyRound,
  "layout-dashboard": LayoutDashboard,
  "line-chart": LineChart,
  "link-2": Link2,
  mail: Mail,
  puzzle: Puzzle,
  "shopping-cart": ShoppingCart,
  "ticket": Ticket,
  "scroll-text": ScrollText,
  send: Send,
  server: Server,
  settings: Settings,
  shield: Shield,
  "shield-alert": ShieldAlert,
  sparkles: Sparkles,
  store: Store,
  user: User,
  users: Users,
  wallet: Wallet,
  webhook: Webhook,
  workflow: Workflow,
};

// Resolve a plugin menu icon key to its lucide component, or undefined when
// the string isn't a known key (then the sidebar renders it as literal text /
// emoji via NavItem.iconStr — the pre-existing dynamic-menu behavior).
export function menuIcon(key?: string): React.ElementType | undefined {
  return key ? PLUGIN_ICONS[key.toLowerCase()] : undefined;
}

// Materialize a plugin-declared area (UIPlugin.areas) into the app's Area
// shape: an empty shell whose groups are filled by the same menu-merge pipeline
// (areaForCategory) as every other area; empty shells are dropped at runtime.
export function pluginAreaToArea(a: UIArea): Area {
  return {
    id: a.id,
    title: a.title,
    subtitle: a.subtitle ?? "",
    Icon: menuIcon(a.icon) ?? Puzzle,
    // Seed ordered group shells from the declaration so the sidebar groups keep
    // the plugin's intended order; menus fill them via the category-match in
    // App.tsx. Empty (no declared groups) → groups synthesized from menus.
    groups: (a.groups ?? []).map((label) => ({ label, items: [] })),
  };
}
