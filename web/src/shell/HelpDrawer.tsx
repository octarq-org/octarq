import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { AlertTriangle, BookOpen, ExternalLink, List, X } from "lucide-react";
import { Skeleton } from "@octarq/plugin-sdk";
import { api, type HelpDocContent } from "../api";
import { useTranslation } from "../i18n";
import { HelpArticle } from "../plugins/core/help/HelpArticle";
import HelpBrowser from "../plugins/core/help/HelpBrowser";
import {
  isHelpPath,
  parseHelpDeepLink,
  useHelpDrawer,
  useHelpDrawerStore,
} from "./useHelpDrawer";

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

function slugFromPath(path: string): string | null {
  const clean = path.split("#")[0].split("?")[0];
  const match = clean.match(/^\/help\/(?:[^/]+\/)?([^/]+)\/?$/);
  return match ? match[1] : null;
}

function docHref(doc: { slug: string; category?: string }): string {
  return `/help/${(doc.category || "services").toLowerCase()}/${doc.slug}`;
}

function scrollToAnchor(root: HTMLElement | null, anchor: string): void {
  if (!root) return;
  let id = anchor;
  try {
    id = decodeURIComponent(anchor);
  } catch {
    id = anchor;
  }
  const target = document.getElementById(id) ?? document.getElementById(anchor);
  if (!target || !root.contains(target)) return;
  if (typeof target.scrollIntoView === "function") {
    target.scrollIntoView({ block: "start" });
  }
}

export function HelpDrawer({ activeOrgId }: { activeOrgId?: number }) {
  const { t, lang } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { isOpen, slug, anchor, docs, categories, open, browse, close, openForCurrentPage } =
    useHelpDrawer(activeOrgId);

  const [content, setContent] = useState<HelpDocContent | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [retryKey, setRetryKey] = useState(0);

  const panelRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const restoreRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (isHelpPath(window.location.pathname)) return;
    const parsed = parseHelpDeepLink(window.location.search);
    if (parsed) useHelpDrawerStore.getState().open(parsed.slug, parsed.anchor);
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (target) {
        const tag = target.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target.isContentEditable) {
          return;
        }
      }
      if (event.key !== "F1" && event.key !== "?") return;
      if (isHelpPath(location.pathname)) return;
      if (useHelpDrawerStore.getState().isOpen) {
        event.preventDefault();
        close();
        return;
      }
      if (openForCurrentPage()) event.preventDefault();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [location.pathname, openForCurrentPage, close]);

  useEffect(() => {
    if (!isOpen || !slug) {
      setContent(null);
      setError(null);
      return;
    }
    let cancelled = false;
    setContent(null);
    setError(null);
    api
      .helpPage(slug, lang)
      .then((page) => {
        if (!cancelled) setContent(page);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, slug, lang, retryKey]);

  useEffect(() => {
    if (!isOpen) return;
    restoreRef.current = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    panelRef.current?.focus();
    return () => {
      document.body.style.overflow = previousOverflow;
      restoreRef.current?.focus?.();
    };
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen || !content) return;
    const heading = panelRef.current?.querySelector<HTMLElement>("h1");
    if (!heading) return;
    heading.tabIndex = -1;
    heading.focus();
  }, [isOpen, content]);

  useEffect(() => {
    if (!isOpen || !content || !anchor) return;
    scrollToAnchor(scrollRef.current, anchor);
  }, [isOpen, content, anchor]);

  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        close();
        return;
      }
      if (event.key !== "Tab") return;
      const panel = panelRef.current;
      if (!panel) return;
      const focusables = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (focusables.length === 0) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [isOpen, close]);

  const handleNavigate = useCallback(
    (path: string) => {
      const targetSlug = slugFromPath(path);
      if (targetSlug && docs.some((doc) => doc.slug === targetSlug)) {
        open(targetSlug, null);
        return;
      }
      close();
      navigate(path);
    },
    [close, docs, navigate, open],
  );

  const resolveDocPath = useCallback(
    (docSlug: string) => {
      const doc = docs.find((item) => item.slug === docSlug);
      return doc ? docHref(doc) : null;
    },
    [docs],
  );

  const openDirectory = () => {
    close();
    navigate("/help");
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end" data-testid="help-drawer-overlay">
      <div
        className="fixed inset-0 bg-black/40"
        onClick={close}
        aria-hidden="true"
        data-testid="help-drawer-backdrop"
      />

      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="help-drawer-title"
        tabIndex={-1}
        className="relative z-50 flex h-full w-full flex-col border-l border-border bg-background outline-none md:max-w-[560px]"
        data-testid="help-drawer"
      >
        <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-3.5">
          <div className="flex min-w-0 items-center gap-2">
            <BookOpen className="h-4 w-4 shrink-0 text-accent-fg" aria-hidden="true" />
            <h2
              id="help-drawer-title"
              className="truncate font-display text-[15px] font-semibold tracking-[-0.01em] text-foreground"
            >
              {t("help.title", "Help & Documentation")}
            </h2>
            <kbd className="hidden rounded border border-border bg-well px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground sm:inline">
              {t("help.shortcut", "F1")}
            </kbd>
          </div>

          <div className="flex items-center gap-1">
            {slug ? (
              <button
                type="button"
                onClick={browse}
                aria-label={t("help.drawer_browse", "Browse all help")}
                title={t("help.drawer_browse", "Browse all help")}
                data-testid="help-drawer-browse"
                className="rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
              >
                <List className="h-4 w-4" />
              </button>
            ) : (
              <button
                type="button"
                onClick={openDirectory}
                aria-label={t("help.drawer_open_directory", "Open the full directory")}
                title={t("help.drawer_open_directory", "Open the full directory")}
                data-testid="help-drawer-directory"
                className="rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
              >
                <ExternalLink className="h-4 w-4" />
              </button>
            )}
            <button
              type="button"
              onClick={close}
              aria-label={t("common.close", "Close")}
              title={t("common.close", "Close")}
              className="rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        <div
          ref={scrollRef}
          className="min-h-0 flex-1 overflow-y-auto px-4 py-6 scrollbar-thin sm:px-6"
          data-testid="help-drawer-scroll"
        >
          {error ? (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-lg border border-danger-border bg-danger-bg p-3 text-xs text-danger-fg"
            >
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
              <div className="flex-1 space-y-1.5">
                <p className="font-semibold">{t("help.error_title", "Couldn't load this page")}</p>
                <p className="break-words">{error}</p>
                <button
                  type="button"
                  onClick={() => setRetryKey((key) => key + 1)}
                  className="rounded-md border border-danger-border px-3 py-1 text-xs font-semibold"
                >
                  {t("help.error_retry", "Retry")}
                </button>
              </div>
            </div>
          ) : content ? (
            <HelpArticle
              title={content.title}
              html={content.html}
              slug={slug ?? ""}
              lang={lang}
              onNavigate={handleNavigate}
              resolveDocPath={resolveDocPath}
            />
          ) : slug ? (
            <div className="space-y-3" data-testid="help-drawer-loading">
              <Skeleton className="h-5 w-1/2" />
              <Skeleton className="h-3.5 w-full" />
              <Skeleton className="h-3.5 w-11/12" />
              <Skeleton className="h-3.5 w-4/5" />
            </div>
          ) : (
            <HelpBrowser
              docs={docs}
              categories={categories}
              autoFocus
              onSelect={(nextSlug, nextAnchor) => open(nextSlug, nextAnchor)}
            />
          )}
        </div>
      </div>
    </div>
  );
}
