import {
  Bell,
  Boxes,
  CreditCard,
  Database,
  Globe,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  Link2,
  Mail,
  Puzzle,
  ScrollText,
  Server,
  Settings,
  Shield,
  ShieldAlert,
  User,
  Users,
  Wallet,
  Webhook,
  Workflow,
} from "lucide-react";

// ─── Area definitions ──────────────────────────────────────────────────────

export type AreaId = "operations" | "commerce" | "assets" | "insights" | "settings";

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
    title: "Workspace",
    subtitle: "Daily traffic & communication",
    Icon: Workflow,
    groups: [
      {
        label: "Workspace",
        items: [
          { id: "overview", label: "Overview", Icon: LayoutDashboard, path: "/overview" },
        ],
      },
      {
        label: "Marketing",
        items: [
          { id: "links",   label: "Links",    Icon: Link2,  path: "/links" },
        ],
      },
      {
        label: "Messaging",
        items: [
          { id: "mail",    label: "Mail",     Icon: Mail,   path: "/mail" },
          // AI Inbox is a Pro plugin now — its menu entry is injected dynamically
          // (plugins/inbox-ai, category "Messaging") only in a composed build.
        ],
      },
    ],
  },
  {
    id: "commerce",
    title: "Commerce",
    subtitle: "Revenue, store & cost analysis",
    Icon: Wallet,
    // Every commerce feature is a Pro plugin. These group shells stay so the
    // dynamic plugin menus land in the right group/order (matched by label) in a
    // composed build; in the OSS build all groups are empty and the whole
    // Commerce area is dropped by the empty-area filter in App.tsx.
    groups: [
      // Storefront + Licenses → plugins/storefront, plugins/licenses (category "Sales").
      { label: "Sales", items: [] },
      // Billing → plugins/billing (category "Billing").
      { label: "Billing", items: [] },
      // Bookkeeping → plugins/finance (category "Finance").
      { label: "Finance", items: [] },
    ],
  },
  {
    id: "assets",
    title: "Infrastructure",
    subtitle: "Servers, network & databases",
    Icon: Boxes,
    groups: [
      {
        label: "Network",
        items: [
          { id: "domains", label: "DNS",          Icon: Globe,    path: "/domains" },
          { id: "certs",   label: "Certificates", Icon: Shield,  path: "/assets/certificates" },
        ],
      },
      {
        // Servers + SSH Vault are Pro plugins (plugins/vps, plugins/ssh-keys,
        // category "Hosting") — injected dynamically only in a composed build.
        label: "Hosting",
        items: [],
      },
      {
        label: "Storage & Databases",
        items: [
          { id: "databases", label: "Databases", Icon: Database,  path: "/assets/databases" },
          { id: "storage",   label: "Object Storage", Icon: HardDrive, path: "/assets/storage" },
        ],
      },
    ],
  },
  {
    id: "insights",
    title: "Security & Admin",
    subtitle: "Abuse defense & activity logs",
    Icon: ShieldAlert,
    groups: [
      {
        label: "Security",
        items: [
          { id: "abuse",    label: "Abuse Reports",      Icon: ShieldAlert, path: "/abuse" },
        ],
      },
      {
        label: "System",
        items: [
          { id: "audit", label: "Audit Log", Icon: ScrollText, path: "/audit" },
        ],
      },
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
        { id: "plugins",       label: "Plugins",     Icon: Puzzle,      path: "/settings/plugins" },
        { id: "members",       label: "Members",     Icon: Users,       path: "/settings/members" },
        { id: "webhooks",      label: "Webhooks",    Icon: Webhook,     path: "/settings/webhooks" },
        { id: "notifications", label: "Alerts",      Icon: Bell,        path: "/settings/notifications" },
      ],
    },
    {
      label: "Account",
      items: [
        { id: "profile",  label: "My Profile", Icon: User,      path: "/personal/profile" },
        { id: "security", label: "Security",   Icon: Shield,    path: "/settings/security" },
        { id: "tokens",   label: "API Tokens", Icon: KeyRound,  path: "/personal/tokens" },
      ],
    },
    {
      label: "Subscriptions",
      items: [
        { id: "billing", label: "Billing & Plan", Icon: CreditCard, path: "/settings/billing" },
        { id: "license", label: "License",        Icon: KeyRound,   path: "/settings/license" },
      ],
    },
    {
      label: "Instance",
      items: [
        { id: "instance", label: "Instance Settings", Icon: Server, path: "/settings/instance" },
      ],
    },
  ],
};

// PATH_TO_AREA is derived from STATIC_AREAS so the path→area mapping has a single
// source of truth (the menu definition) instead of a parallel if/else chain.
const PATH_TO_AREA: { prefix: string; area: AreaId }[] = STATIC_AREAS
  .flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ prefix: i.path, area: a.id }))))
  .sort((x, y) => y.prefix.length - x.prefix.length); // longest prefix wins

export function areaForPath(path: string): AreaId {
  // Settings/personal live in their own area (SETTINGS_AREA), not STATIC_AREAS.
  if (path.startsWith("/settings") || path.startsWith("/personal")) return "settings";
  const hit = PATH_TO_AREA.find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));
  return hit?.area ?? "operations";
}

// Map a dynamic menu category to an area. Keep this in sync with the Category
// strings plugins set in their Menus() — see docs/SIDEBAR-MENU.md.
export function areaForCategory(cat?: string): AreaId {
  const c = (cat ?? "").toLowerCase();
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute") || c.includes("hosting")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("compliance") || c.includes("governance") || c.includes("audit") || c.includes("abuse") || c.includes("system")) return "insights";
  if (c.includes("commerce") || c.includes("sell") || c.includes("sale") || c.includes("billing") || c.includes("storefront") || c.includes("license") || c.includes("finance")) return "commerce";
  return "operations";
}
