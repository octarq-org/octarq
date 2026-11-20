import {
  Bell,
  Blocks,
  Brain,
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
  MessageSquare,
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


export type AreaId = string;

export interface NavItem {
  id: string;
  label: string;
  Icon: React.ElementType;
  iconStr?: string;
  path: string;
  badge?: string | number;
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
  {
    id: "personal",
    title: "Personal",
    subtitle: "Accounts & credentials that belong to you",
    Icon: User,
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
        { id: "gateway",       label: "Model Gateway", Icon: Brain,     path: "/settings/gateway" },
        { id: "plugins",       label: "Features",    Icon: Puzzle,      path: "/settings/plugins" },
        { id: "members",       label: "Members",     Icon: Users,       path: "/settings/members" },
        { id: "notifications", label: "Notifications & Alerts", Icon: Bell, path: "/settings/notifications" },
      ],
    },
    {
      label: "Integrations",
      items: [
        { id: "integrations",  label: "Integrations Hub", Icon: Blocks,     path: "/settings/integrations" },
        { id: "webhooks",      label: "Webhooks & Relays", Icon: Webhook, path: "/settings/webhooks" },
      ],
    },
    {
      label: "Account",
      items: [
        { id: "profile",  label: "My Profile", Icon: User,      path: "/settings/profile" },
        { id: "security", label: "Security",   Icon: Shield,    path: "/settings/security" },
        { id: "tokens",   label: "API Tokens", Icon: KeyRound,  path: "/settings/tokens" },
        { id: "appearance", label: "Appearance", Icon: Palette, path: "/settings/appearance" },
      ],
    },
  ],
};

export function areaForPath(path: string, areas: Area[] = STATIC_AREAS): AreaId {
  if (path.startsWith("/settings")) return "settings";
  if (path.startsWith("/help") || path.startsWith("/admin/help")) return "help";
  const hit = areas
    .flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ prefix: i.path, area: a.id }))))
    .sort((x, y) => y.prefix.length - x.prefix.length)
    .find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));
  return hit?.area ?? "operations";
}

export const FOOTER_PLACEMENT = "footer";

export function areaForCategory(cat?: string, pluginAreas: UIArea[] = []): AreaId {
  return NavigationTree.resolveCategoryToArea(cat, pluginAreas);
}

export function areaForMenu(menu: MenuItem, pluginAreas: UIArea[] = []): AreaId {
  return NavigationTree.resolveMenuToArea(menu, pluginAreas);
}


const PLUGIN_ICONS: Record<string, React.ElementType> = {
  bell: Bell,
  book: Book,
  "book-open": BookOpen,
  bot: Bot,
  brain: Brain,
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
  "message-square": MessageSquare,
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

export function menuIcon(key?: string): React.ElementType | undefined {
  return key ? PLUGIN_ICONS[key.toLowerCase()] : undefined;
}

export function pluginAreaToArea(a: UIArea): Area {
  return {
    id: a.id,
    title: a.title,
    subtitle: a.subtitle ?? "",
    Icon: menuIcon(a.icon) ?? Puzzle,
    groups: (a.groups ?? []).map((label) => ({ label, items: [] })),
  };
}
