import {
  Bell,
  Book,
  Bot,
  Boxes,
  CreditCard,
  Database,
  Globe,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LineChart,
  Link2,
  Mail,
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

// Areas are data-driven: the built-in ids below come from STATIC_AREAS, and a
// plugin may contribute a NEW top-level area (UIPlugin.areas → uiAreas()), so
// the id space is open — plain string, with "settings" special-cased where it
// matters (areaForPath, App's selectArea). Adding a built-in area now means
// editing STATIC_AREAS (+ areaForCategory keywords if it should attract
// dynamic menus) — no separate union to keep in sync.
export type AreaId = string;

export interface NavItem {
  id: string;
  label: string;
  Icon: React.ElementType;
  iconStr?: string;
  path: string;
  badge?: string | number;
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
      // Links → core plugin (plugins/core/links.ts, category "Marketing").
      { label: "Marketing", items: [] },
      // Mail → core plugin (plugins/core/mail.ts, category "Messaging").
      // AI Inbox is a Pro plugin — its menu entry is injected dynamically
      // (@octarq-org/plugin-ai, category "Messaging") only in a composed build.
      { label: "Messaging", items: [] },
      // Abuse Reports → core plugin (plugins/core/abuse.ts, category "Security").
      { label: "Security", items: [] },
      // Audit Log → core plugin (plugins/core/audit.ts, category "System").
      { label: "System", items: [] },
    ],
  },
  // Commerce is a Pro area: the commerce plugins (storefront / billing /
  // finance / issuer / licensing) declare it via UIPlugin.areas with its
  // Sales/Billing/Finance group shells. The OSS core ships no shell for it —
  // an empty one only ever got dropped by App.tsx's empty-area filter anyway.
  {
    id: "assets",
    title: "Infrastructure",
    subtitle: "Servers, network & databases",
    Icon: Boxes,
    groups: [
      // DNS → core plugin (plugins/core/domains.ts); Certificates → core
      // plugin (plugins/core/assets.ts). Both use category "Network".
      { label: "Network", items: [] },
      // Servers + SSH Vault are Pro plugins (@octarq-org/plugin-infra,
      // category "Hosting") — injected dynamically only in a composed build.
      { label: "Hosting", items: [] },
      // Databases + Object Storage → core plugin (plugins/core/assets.ts,
      // category "Storage & Databases").
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
      label: "Account",
      items: [
        { id: "profile",  label: "My Profile", Icon: User,      path: "/settings/profile" },
        { id: "security", label: "Security",   Icon: Shield,    path: "/settings/security" },
        { id: "tokens",   label: "API Tokens", Icon: KeyRound,  path: "/settings/tokens" },
      ],
    },
    // ── octarq-PROVIDED, instance-admin ───────────────────────────────────
    // Configuration of the octarq software/instance itself — shown only to
    // instance admins (App gates this group on isInstanceAdmin). Plugins add
    // here with category "Instance": e.g. the Pro licensing plugin lands the
    // operator's octarq License (activation) here, NOT in the org-outward
    // Commerce area. Passive octarq resources (Help, Docs, About, GitHub) are
    // NOT here — they live in the always-available sidebar footer.
    {
      label: "Instance",
      items: [
        { id: "auth",     label: "Authentication",    Icon: KeyRound, path: "/settings/auth" },
        { id: "instance", label: "Instance Settings", Icon: Server,   path: "/settings/instance" },
        { id: "instance-plugins", label: "Plugins",  Icon: Puzzle,   path: "/settings/instance/plugins" },
      ],
    },
  ],
};

// The path→area mapping is DERIVED from the area definitions (single source of
// truth — the menu data), never a parallel hardcoded map. Callers that have the
// merged runtime areas (static + plugin areas + dynamic menu items — see
// App.tsx) pass them in so plugin-contributed paths resolve too; the default
// covers the static-only case.
export function areaForPath(path: string, areas: Area[] = STATIC_AREAS): AreaId {
  // Settings live in their own area (SETTINGS_AREA), not the areas list.
  if (path.startsWith("/settings")) return "settings";
  const hit = areas
    .flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ prefix: i.path, area: a.id }))))
    .sort((x, y) => y.prefix.length - x.prefix.length) // longest prefix wins
    .find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));
  return hit?.area ?? "operations";
}

// Map a dynamic menu category to an area. A category naming a plugin-declared
// area (by id or title) lands there; otherwise the built-in keyword routing
// applies. Keep the keywords in sync with the Category strings plugins set in
// their Menus() — see docs/PLUGINS.md.
// Placement keyword for the sidebar footer. A menu item whose category is this
// (or "resources") is rendered among the always-available octarq resources in
// the rail footer rather than in any nav area — one more optional placement a
// plugin can pick, alongside the areas and Settings. Not a real Area id.
export const FOOTER_PLACEMENT = "footer";

export function areaForCategory(cat?: string, pluginAreas: UIArea[] = []): AreaId {
  const c = (cat ?? "").toLowerCase();

  // ── octarq-PROVIDED resources → sidebar footer ────────────────────────────
  // Low-frequency, always-available product links (Help, docs, …). A plugin
  // opts into this placement with category "footer"/"resources"; the shell
  // collects these out of the area merge and renders them in the rail footer.
  if (c === FOOTER_PLACEMENT || c === "resources") return FOOTER_PLACEMENT;

  // ── octarq-PROVIDED settings ────────────────────────────────────────────
  // The Settings area (the gear, NOT a top-level tab) holds configuration of
  // the octarq instance/account itself. A plugin lands a page there with
  // category "Instance" (octarq/instance admin — e.g. the operator's octarq
  // license/activation), "Account" (the signed-in user), or a generic
  // "Settings". This is the strict counterpart to the org-OUTWARD areas
  // (operations/assets/insights + the Pro Commerce area) handled below, which
  // carry what the org runs FOR its own users/customers. Keep the two apart:
  // octarq's own license → Settings; the org issuing licenses to its customers
  // → Commerce.
  if (c === "settings" || c === "instance" || c === "account") return "settings";

  // A menu lands in a plugin-declared area when its category matches the area's
  // id, its title, or one of its declared group labels — so a Pro edition can
  // own a whole multi-group area (e.g. Commerce with Sales/Billing/Finance)
  // that the OSS core no longer ships a shell or keyword branch for.
  const pluginHit = pluginAreas.find(
    (a) =>
      a.id.toLowerCase() === c ||
      a.title.toLowerCase() === c ||
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
  bot: Bot,
  boxes: Boxes,
  "credit-card": CreditCard,
  database: Database,
  globe: Globe,
  "hard-drive": HardDrive,
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
