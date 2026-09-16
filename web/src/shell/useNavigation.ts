import { useCallback, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { BookOpen } from "lucide-react";
import { api, MenuItem, PluginInfo, Action, HelpDocMeta, HelpCategory } from "../api";
import { useTranslation } from "../i18n";
import { uiAreas } from "../plugin-sdk";
import { Area, AreaId, NavGroup, NavItem, STATIC_AREAS, menuIcon } from "./areas";
import { NavigationTree } from "./NavigationTree";
import { visibleActions } from "./globalActions";

// Cache last api.menus()/api.plugins() response for instant first paint; replaced when live data arrives.
export const NAV_CACHE_KEY = "octarq:nav-cache:v1";

export interface CachedNav {
  menus: MenuItem[];
  plugins: PluginInfo[];
  actions?: Action[];
}

export function readCachedNav(): CachedNav {
  try {
    const raw = localStorage.getItem(NAV_CACHE_KEY);
    if (!raw) return { menus: [], plugins: [], actions: [] };
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed?.menus) || !Array.isArray(parsed?.plugins)) {
      return { menus: [], plugins: [], actions: [] };
    }
    return {
      menus: parsed.menus,
      plugins: parsed.plugins,
      actions: Array.isArray(parsed?.actions) ? parsed.actions : [],
    };
  } catch {
    return { menus: [], plugins: [], actions: [] };
  }
}

export function writeCachedNav(nav: CachedNav) {
  try {
    localStorage.setItem(NAV_CACHE_KEY, JSON.stringify(nav));
  } catch {
    /* quota or private mode — the cache is optional, the fetch is not */
  }
}

export function clearCachedNav() {
  try {
    localStorage.removeItem(NAV_CACHE_KEY);
  } catch {
    /* nothing to do — a stale cache is replaced on the next successful fetch */
  }
}

export interface UseNavigationOptions {
  role?: string;
  isInstanceAdmin?: boolean;
  activeOrgId?: number;
  lang?: string;
}

export function useNavigation({
  role,
  isInstanceAdmin = false,
  activeOrgId = 0,
  lang: langProp,
}: UseNavigationOptions = {}) {
  const location = useLocation();
  const navigate = useNavigate();
  const { t, lang: ctxLang } = useTranslation();
  const lang = langProp ?? ctxLang;

  const [backendNav, setBackendNav] = useState<CachedNav>(readCachedNav);
  const [backendLoaded, setBackendLoaded] = useState(false);
  const [isProBuild, setIsProBuild] = useState(false);

  const [helpDocsNav, setHelpDocsNav] = useState<HelpDocMeta[]>([]);
  const [helpCategories, setHelpCategories] = useState<HelpCategory[]>([]);

  // Fetch help documentation categories
  useEffect(() => {
    api.helpCategories()
      .then(setHelpCategories)
      .catch(() => {});
  }, []);

  // Fetch help documentation index for current active workspace and language
  useEffect(() => {
    api.helpIndex(lang)
      .then(setHelpDocsNav)
      .catch(() => {});
  }, [activeOrgId, lang]);

  // Fetch menus, plugins, actions on workspace switch
  const refreshNav = useCallback(() => {
    Promise.all([
      api.menus().catch(() => []),
      api.plugins().catch(() => []),
      api.actions().catch(() => []),
    ])
      .then(([menus, plugins, actions]) => {
        setIsProBuild(plugins.length > 0);
        setBackendNav({ menus, plugins, actions });
        setBackendLoaded(true);
        writeCachedNav({ menus, plugins, actions });
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refreshNav();
  }, [activeOrgId, refreshNav]);

  // Listen for dynamic plugin change events
  useEffect(() => {
    const handlePluginsChanged = () => {
      refreshNav();
    };
    window.addEventListener("octarq:plugins-changed", handlePluginsChanged);
    return () => {
      window.removeEventListener("octarq:plugins-changed", handlePluginsChanged);
    };
  }, [refreshNav]);

  // Build Help area from docs and categories
  const helpArea: Area = useMemo(() => {
    const catDocsMap = new Map<string, HelpDocMeta[]>();

    helpDocsNav.forEach((d: HelpDocMeta) => {
      const category = (d.category || "services").toLowerCase();
      if (!catDocsMap.has(category)) {
        catDocsMap.set(category, []);
      }
      catDocsMap.get(category)!.push(d);
    });

    const groups: NavGroup[] = [];

    const categoriesToRender =
      helpCategories.length > 0
        ? helpCategories
        : Array.from(catDocsMap.keys()).map((k, idx) => ({
            key: k,
            order: (idx + 1) * 10,
            icon: "boxes",
            labels: { en: k },
          }));

    categoriesToRender.forEach((cat) => {
      const docsList = catDocsMap.get(cat.key);
      if (!docsList || docsList.length === 0) return;

      const sortedDocsList = [...docsList].sort((a, b) => {
        if ((a.order ?? 0) !== (b.order ?? 0)) return (a.order ?? 0) - (b.order ?? 0);
        return (a.title || "").localeCompare(b.title || "");
      });

      const catLabel = cat.labels[lang] || cat.labels["en"] || cat.key;
      const CatIcon = menuIcon(cat.icon) || BookOpen;

      const items: NavItem[] = sortedDocsList.map((doc) => ({
        id: `help-${doc.slug}`,
        label: doc.title,
        Icon: CatIcon,
        path: `/help/${cat.key}/${doc.slug}`,
      }));

      groups.push({
        label: catLabel,
        items,
      });
    });

    return {
      id: "help",
      title: t("help.title", "Help & Documentation"),
      subtitle: t("help.platform_subtitle", "Guides, tutorials and platform reference"),
      Icon: BookOpen,
      groups:
        groups.length > 0
          ? groups
          : [
              {
                label: "Help",
                items: [
                  {
                    id: "help-root",
                    label: "Help",
                    Icon: BookOpen,
                    path: "/help",
                  },
                ],
              },
            ],
    };
  }, [helpDocsNav, helpCategories, lang, t]);

  // Build the pure NavigationTree domain model
  const tree = useMemo(() => {
    const pluginAreas = uiAreas().filter(
      (pa) => pa.id !== "settings" && !STATIC_AREAS.some((sa) => sa.id === pa.id),
    );

    return NavigationTree.build({
      menus: backendNav.menus,
      plugins: backendNav.plugins,
      pluginAreas,
      role,
      isInstanceAdmin,
      backendLoaded,
      helpArea,
    });
  }, [
    backendNav.menus,
    backendNav.plugins,
    role,
    isInstanceAdmin,
    backendLoaded,
    helpArea,
  ]);

  // Visible top-level areas
  const areas = tree.areas;
  const currentSettingsArea = tree.settingsArea;
  const footerItems = tree.footerItems;

  // Active area resolution
  const helpActive = tree.isHelpPath(location.pathname);
  const settingsActive = tree.isSettingsPath(location.pathname);

  const activeArea: AreaId = helpActive
    ? "help"
    : settingsActive
    ? "settings"
    : tree.areaForPath(location.pathname);

  const currentArea = helpActive
    ? helpArea
    : settingsActive
    ? currentSettingsArea
    : (tree.findArea(activeArea) ?? areas[0] ?? STATIC_AREAS[0]);

  // Filtered global create actions
  const filteredActions = useMemo(
    () => visibleActions(backendNav.actions, role, isInstanceAdmin),
    [backendNav.actions, role, isInstanceAdmin],
  );

  // Plugin gate context value
  const pluginGateCtxValue = useMemo(() => {
    return {
      disabledPlugins: tree.disabledPlugins,
      disabledPaths: tree.disabledPaths,
      loaded: backendLoaded,
    };
  }, [tree.disabledPlugins, tree.disabledPaths, backendLoaded]);

  // Navigate when switching areas
  const selectArea = useCallback(
    (id: AreaId) => {
      if (id === "settings") {
        navigate("/settings");
        return;
      }
      if (id === "help") {
        const firstHelpPath = helpArea.groups[0]?.items[0]?.path ?? "/help";
        navigate(firstHelpPath);
        return;
      }
      const area = tree.findArea(id);
      navigate(area?.groups[0]?.items[0]?.path ?? "/overview");
    },
    [navigate, helpArea, tree],
  );

  return {
    tree,
    areas,
    activeArea,
    currentArea,
    currentSettingsArea,
    footerItems,
    filteredActions,
    pluginGateCtxValue,
    settingsActive,
    helpActive,
    isProBuild,
    backendLoaded,
    backendNav,
    helpDocsNav,
    helpCategories,
    helpArea,
    selectArea,
    refreshNav,
    clearCachedNav,
  };
}
