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
      // Links → plugin (plugins/links, category "Marketing").
      { label: "Marketing", items: [] },
      // Mail → plugin (plugins/mail, category "Messaging").
      // AI Inbox is a Pro plugin — its menu entry is injected dynamically
      // (@octarq-org/plugin-ai, category "Messaging") only in a composed build.
      { label: "Messaging", items: [] },
      // Abuse Reports → core plugin (plugins/core/abuse.ts, category "Security").
      { label: "Security", items: [] },
      // Audit Log → core plugin (plugins/core/audit.ts, category "System").
      { label: "System", items: [] },
    ],
  },
  // Pro areas (e.g. Commerce) declare group shells dynamically via UIPlugin.areas.
  {
    id: "assets",
    title: "Infrastructure",
    subtitle: "Servers, network & databases",
    Icon: Boxes,
    groups: [
      // DNS → plugin (plugins/dns); Certificates → Pro
      // infra plugin (@octarq-org/plugin-infra). Both use category "Network".
      { label: "Network", items: [] },
      // Servers + SSH Vault are Pro plugins (@octarq-org/plugin-infra,
      // category "Hosting") — injected dynamically only in a composed build.
      { label: "Hosting", items: [] },
      // Databases + Object Storage → Pro plugins (@octarq-org/plugin-infra,
      // @octarq-org/plugin-databases, category "Storage & Databases").
      { label: "Storage & Databases", items: [] },
    ],
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

// Maps a dynamic menu category to an area. Keep the keywords below in sync with
// the Category strings plugins set in their Menus() — see
// website/src/content/docs/writing-a-plugin.md.
export function areaForCategory(cat?: string, pluginAreas: UIArea[] = []): AreaId {
  const c = (cat ?? "").toLowerCase();

  // Product links categorized as "footer"/"resources" land in the sidebar footer.
  if (c === FOOTER_PLACEMENT || c === "resources") return FOOTER_PLACEMENT;

  // Settings holds what the org configures about octarq and itself; the areas
  // below hold what the org runs FOR its own customers. Keep them apart:
  // octarq's own license → Settings, the org issuing licenses → Commerce.
  if (c === "settings" || c === "instance" || c === "account" || c === "personal") return "settings";

  // Category matches a plugin area by id or by one of its declared group
  // labels — never by title. Title is what the sidebar renders and what
  // translateAreaTitle localizes; routing on it would mean renaming an area
  // silently relocates every menu that named it. Guarded by areaForCategory.test.ts.
  const pluginHit = pluginAreas.find(
    (a) =>
      a.id.toLowerCase() === c ||
      (a.groups ?? []).some((g) => g.toLowerCase() === c),
  );
  if (pluginHit) return pluginHit.id;
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute") || c.includes("hosting") || c.includes("storage") || c.includes("database")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("compliance") || c.includes("governance") || c.includes("audit") || c.includes("abuse") || c.includes("security") || c.includes("system")) return "operations";
  return "operations";
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
