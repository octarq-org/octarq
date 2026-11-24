import { useCallback, useEffect, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { create } from "zustand";
import { api, type HelpDocMeta } from "../api";
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

function docsCacheKey(orgId: number | undefined, lang: string): string {
  return `${orgId ?? 0}:${lang}`;
}

export function clearHelpDocsCache(): void {
  docsCache.clear();
  docsInflight.clear();
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

export interface HelpDrawerState {
  isOpen: boolean;
  slug: string | null;
  anchor: string | null;
  docs: HelpDocMeta[];
  docsKey: string | null;
  open: (slug: string, anchor?: string | null) => void;
  close: () => void;
  setDocs: (key: string, docs: HelpDocMeta[]) => void;
}

export const useHelpDrawerStore = create<HelpDrawerState>((set) => ({
  isOpen: false,
  slug: null,
  anchor: null,
  docs: [],
  docsKey: null,
  open: (slug, anchor = null) => set({ isOpen: true, slug, anchor }),
  close: () => set({ isOpen: false, slug: null, anchor: null }),
  setDocs: (key, docs) => set({ docsKey: key, docs }),
}));

export function useHelpDrawer(orgId?: number) {
  const { lang } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();

  const isOpen = useHelpDrawerStore((s) => s.isOpen);
  const slug = useHelpDrawerStore((s) => s.slug);
  const anchor = useHelpDrawerStore((s) => s.anchor);
  const docs = useHelpDrawerStore((s) => s.docs);
  const docsKey = useHelpDrawerStore((s) => s.docsKey);
  const openState = useHelpDrawerStore((s) => s.open);
  const closeState = useHelpDrawerStore((s) => s.close);
  const setDocs = useHelpDrawerStore((s) => s.setDocs);

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

  const match = useMemo(() => {
    if (isHelpPath(location.pathname)) return undefined;
    return createDocRoutes(docs).match(location.pathname);
  }, [docs, location.pathname]);

  const open = useCallback(
    (nextSlug: string, nextAnchor: string | null = null) => {
      syncHelpUrl(nextSlug, nextAnchor);
      openState(nextSlug, nextAnchor);
    },
    [openState],
  );

  const close = useCallback(() => {
    syncHelpUrl(null, null);
    closeState();
  }, [closeState]);

  const openForCurrentPage = useCallback((): boolean => {
    if (isHelpPath(location.pathname)) return false;
    const current = createDocRoutes(useHelpDrawerStore.getState().docs).match(location.pathname);
    if (!current) {
      navigate("/help");
      return true;
    }
    open(current.doc.slug, null);
    return true;
  }, [location.pathname, navigate, open]);

  return {
    isOpen,
    slug,
    anchor,
    docs,
    docsKey,
    match,
    open,
    close,
    openForCurrentPage,
  };
}
