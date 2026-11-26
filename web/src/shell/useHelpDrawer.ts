import { useCallback, useEffect, useMemo } from "react";
import { useLocation } from "react-router-dom";
import { create } from "zustand";
import { api, type HelpCategory, type HelpDocMeta } from "../api";
import { useTranslation } from "../i18n";
import { createDocRoutes } from "./docRoutes";

export interface HelpDeepLink {
  slug: string;
  anchor: string | null;
}

export function parseHelpDeepLink(search: string): HelpDeepLink | null {
  const params = new URLSearchParams(search);
  const slug = params.get("help")?.trim();
  if (!slug) return null;
  const anchor = params.get("anchor")?.trim();
  return { slug, anchor: anchor || null };
}

export function buildHelpQuery(search: string, slug: string | null, anchor: string | null): string {
  const params = new URLSearchParams(search);
  if (slug) {
    params.set("help", slug);
    if (anchor) params.set("anchor", anchor);
    else params.delete("anchor");
  } else {
    params.delete("help");
    params.delete("anchor");
  }
  const next = params.toString();
  return next ? `?${next}` : "";
}

export function syncHelpUrl(slug: string | null, anchor: string | null): void {
  if (typeof window === "undefined") return;
  const query = buildHelpQuery(window.location.search, slug, anchor);
  const url = `${window.location.pathname}${query}${window.location.hash}`;
  window.history.replaceState(window.history.state, "", url);
}

export function isHelpPath(pathname: string): boolean {
  return pathname === "/help" || pathname.startsWith("/help/");
}

const docsCache = new Map<string, HelpDocMeta[]>();
const docsInflight = new Map<string, Promise<HelpDocMeta[]>>();
let categoriesCache: HelpCategory[] | null = null;
let categoriesInflight: Promise<HelpCategory[]> | null = null;

function docsCacheKey(orgId: number | undefined, lang: string): string {
  return `${orgId ?? 0}:${lang}`;
}

export function clearHelpDocsCache(): void {
  docsCache.clear();
  docsInflight.clear();
  categoriesCache = null;
  categoriesInflight = null;
}

export function loadHelpDocs(orgId: number | undefined, lang: string): Promise<HelpDocMeta[]> {
  const key = docsCacheKey(orgId, lang);
  const cached = docsCache.get(key);
  if (cached) return Promise.resolve(cached);
  const pending = docsInflight.get(key);
  if (pending) return pending;
  const request = api
    .helpIndex(lang)
    .then((docs) => {
      docsCache.set(key, docs);
      return docs;
    })
    .finally(() => {
      docsInflight.delete(key);
    });
  docsInflight.set(key, request);
  return request;
}

export function loadHelpCategories(): Promise<HelpCategory[]> {
  if (categoriesCache) return Promise.resolve(categoriesCache);
  if (categoriesInflight) return categoriesInflight;
  const request = api
    .helpCategories()
    .then((categories) => {
      categoriesCache = categories;
      return categories;
    })
    .finally(() => {
      categoriesInflight = null;
    });
  categoriesInflight = request;
  return request;
}

export interface HelpDrawerState {
  isOpen: boolean;
  slug: string | null;
  anchor: string | null;
  docs: HelpDocMeta[];
  docsKey: string | null;
  categories: HelpCategory[];
  open: (slug: string, anchor?: string | null) => void;
  browse: () => void;
  close: () => void;
  setDocs: (key: string, docs: HelpDocMeta[]) => void;
  setCategories: (categories: HelpCategory[]) => void;
}

export const useHelpDrawerStore = create<HelpDrawerState>((set) => ({
  isOpen: false,
  slug: null,
  anchor: null,
  docs: [],
  docsKey: null,
  categories: [],
  open: (slug, anchor = null) => set({ isOpen: true, slug, anchor }),
  browse: () => set({ isOpen: true, slug: null, anchor: null }),
  close: () => set({ isOpen: false, slug: null, anchor: null }),
  setDocs: (key, docs) => set({ docsKey: key, docs }),
  setCategories: (categories) => set({ categories }),
}));

export function useHelpDrawer(orgId?: number) {
  const { lang } = useTranslation();
  const location = useLocation();

  const isOpen = useHelpDrawerStore((s) => s.isOpen);
  const slug = useHelpDrawerStore((s) => s.slug);
  const anchor = useHelpDrawerStore((s) => s.anchor);
  const docs = useHelpDrawerStore((s) => s.docs);
  const docsKey = useHelpDrawerStore((s) => s.docsKey);
  const categories = useHelpDrawerStore((s) => s.categories);
  const openState = useHelpDrawerStore((s) => s.open);
  const browseState = useHelpDrawerStore((s) => s.browse);
  const closeState = useHelpDrawerStore((s) => s.close);
  const setDocs = useHelpDrawerStore((s) => s.setDocs);
  const setCategories = useHelpDrawerStore((s) => s.setCategories);

  useEffect(() => {
    const key = docsCacheKey(orgId, lang);
    if (useHelpDrawerStore.getState().docsKey === key) return;
    let cancelled = false;
    loadHelpDocs(orgId, lang)
      .then((list) => {
        if (!cancelled) setDocs(key, list);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [orgId, lang, setDocs]);

  useEffect(() => {
    if (useHelpDrawerStore.getState().categories.length > 0) return;
    let cancelled = false;
    loadHelpCategories()
      .then((list) => {
        if (!cancelled) setCategories(list);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [setCategories]);

  const onHelpPage = isHelpPath(location.pathname);

  const match = useMemo(() => {
    if (onHelpPage) return undefined;
    return createDocRoutes(docs).match(location.pathname);
  }, [docs, onHelpPage, location.pathname]);

  const open = useCallback(
    (nextSlug: string, nextAnchor: string | null = null) => {
      syncHelpUrl(nextSlug, nextAnchor);
      openState(nextSlug, nextAnchor);
    },
    [openState],
  );

  const browse = useCallback(() => {
    syncHelpUrl(null, null);
    browseState();
  }, [browseState]);

  const close = useCallback(() => {
    syncHelpUrl(null, null);
    closeState();
  }, [closeState]);

  const openForCurrentPage = useCallback((): boolean => {
    if (onHelpPage) return false;
    const current = createDocRoutes(useHelpDrawerStore.getState().docs).match(location.pathname);
    if (!current) {
      browse();
      return true;
    }
    open(current.doc.slug, null);
    return true;
  }, [browse, location.pathname, onHelpPage, open]);

  return {
    isOpen,
    slug,
    anchor,
    docs,
    docsKey,
    categories,
    onHelpPage,
    match,
    open,
    browse,
    close,
    openForCurrentPage,
  };
}
