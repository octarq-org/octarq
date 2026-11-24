import { useEffect, useState, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { api, HelpCategory, HelpDocMeta } from "../../../api";
import { useTranslation } from "../../../i18n";
import {
  BookOpen,
  ChevronRight,
  Check,
  AlertTriangle,
  Clock,
  Share2,
} from "lucide-react";
import { HelpArticle } from "./HelpArticle";
import { HelpDocToc, HelpDocTocMobile, type TocItem } from "./HelpDocToc";
import { HelpDocPagination } from "./HelpDocPagination";

interface DocContent {
  title: string;
  html: string;
}

export default function HelpViewer() {
  const { t, lang } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();

  const currentSlug = useMemo(() => {
    const parts = location.pathname.split("/").filter(Boolean);
    if (parts.length >= 3) return parts[2];
    if (parts.length > 1) return parts[parts.length - 1];
    return "";
  }, [location.pathname]);

  const [docs, setDocs] = useState<HelpDocMeta[]>([]);
  const [_loadingDocs, setLoadingDocs] = useState(true);

  const [content, setContent] = useState<DocContent | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);
  const [error, setError] = useState("");
  const [errorMeta, setErrorMeta] = useState<{ status?: number; requestId?: string } | null>(null);
  const [retryNonce, setRetryNonce] = useState(0);

  const [copiedLink, setCopiedLink] = useState(false);
  const [notFound, setNotFound] = useState(false);

  const getDocUrl = (d: HelpDocMeta) => {
    const category = (d.category || "services").toLowerCase();
    return `/help/${category}/${d.slug}`;
  };

  useEffect(() => {
    api
      .helpIndex(lang)
      .then((list) => {
        setDocs(list);
        setNotFound(false);
        if (!currentSlug && list.length > 0) {
          navigate(getDocUrl(list[0]), { replace: true });
        } else if (currentSlug && list.length > 0) {
          const matched = list.find((d) => d.slug === currentSlug);
          if (matched) {
            const canonicalUrl = getDocUrl(matched);
            if (location.pathname !== canonicalUrl) {
              navigate(canonicalUrl, { replace: true });
            }
          } else {
            setNotFound(true);
          }
        }
      })
      .catch((err) => console.error("failed to load docs", err))
      .finally(() => setLoadingDocs(false));
  }, [lang, currentSlug]);

  useEffect(() => {
    if (!currentSlug) return;
    setLoadingContent(true);
    setError("");
    setErrorMeta(null);
    api
      .helpPage(currentSlug, lang)
      .then((res) => setContent(res))
      .catch((err: unknown) => {
        const errorObj = err as { message?: string; status?: number; requestId?: string };
        setError(errorObj.message || "Failed to load documentation");
        setErrorMeta({ status: errorObj.status, requestId: errorObj.requestId });
      })
      .finally(() => setLoadingContent(false));
  }, [currentSlug, lang, retryNonce]);

  const toc = useMemo<TocItem[]>(() => {
    if (!content?.html) return [];
    const div = document.createElement("div");
    div.innerHTML = content.html;
    const headings = div.querySelectorAll("h2, h3");
    const items: TocItem[] = [];
    headings.forEach((h, idx) => {
      const text = h.textContent || "";
      const id = h.id || `heading-${idx}`;
      const level = h.tagName.toLowerCase() === "h2" ? 2 : 3;
      items.push({ id, text, level });
    });
    return items;
  }, [content?.html]);

  const { prevDoc, nextDoc, currentIndex, totalDocs } = useMemo(() => {
    const idx = docs.findIndex((d) => d.slug === currentSlug);
    return {
      prevDoc: idx > 0 ? docs[idx - 1] : null,
      nextDoc: idx >= 0 && idx < docs.length - 1 ? docs[idx + 1] : null,
      currentIndex: idx + 1,
      totalDocs: docs.length,
    };
  }, [docs, currentSlug]);

  const [categories, setCategories] = useState<HelpCategory[]>([]);
  useEffect(() => {
    api.helpCategories().then(setCategories).catch(() => {});
  }, []);

  const currentDocMeta = useMemo(() => {
    return docs.find((d) => d.slug === currentSlug);
  }, [docs, currentSlug]);

  const categoryObj = useMemo(() => {
    if (!currentDocMeta) return null;
    return categories.find((c) => c.key === currentDocMeta.category);
  }, [categories, currentDocMeta]);

  const readTimeMinutes = useMemo(() => {
    if (!content?.html) return 1;
    const text = content.html.replace(/<[^>]*>/g, " ");
    const words = text.trim().split(/\s+/).filter(Boolean).length;
    return Math.max(1, Math.ceil(words / 200));
  }, [content?.html]);

  const handleCopyLink = () => {
    navigator.clipboard.writeText(window.location.href);
    setCopiedLink(true);
    setTimeout(() => setCopiedLink(false), 2000);
  };

  const resolveDocPath = (slug: string) => {
    const doc = docs.find((d) => d.slug === slug);
    return doc ? getDocUrl(doc) : null;
  };

  return (
    <div className="h-full flex flex-col bg-background text-foreground overflow-hidden">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-6">
        <div className="flex min-w-0 flex-1 items-center gap-2 text-xs">
          <div className="flex items-center gap-1.5 text-muted-foreground">
            <BookOpen className="w-4 h-4 text-accent-fg" />
            <span className="font-semibold text-foreground">
              {t("help.title", "Help & Documentation")}
            </span>
          </div>
          {currentDocMeta && (
            <>
              <span className="hidden items-center gap-2 sm:flex">
                <ChevronRight className="w-3.5 h-3.5 text-muted-foreground/50" />
                {categoryObj && (
                  <span className="rounded-full border border-accent-border bg-accent-soft px-2.5 py-0.5 text-[11px] font-semibold text-accent-fg">
                    {categoryObj.labels[lang] || categoryObj.labels["en"] || categoryObj.key}
                  </span>
                )}
              </span>
              <ChevronRight className="w-3.5 h-3.5 shrink-0 text-muted-foreground/50" />
              <span className="min-w-0 flex-1 truncate font-semibold text-foreground">
                {currentDocMeta.title}
              </span>
            </>
          )}
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={handleCopyLink}
            title={t("help.copy_page_link", "Copy Page Link")}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-border bg-background hover:bg-surface-hover text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
          >
            {copiedLink ? (
              <Check className="w-3.5 h-3.5 text-success-fg" />
            ) : (
              <Share2 className="w-3.5 h-3.5" />
            )}
            <span className="hidden sm:inline">
              {copiedLink ? t("help.copied", "Copied!") : t("help.copy_code", "Share")}
            </span>
          </button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        <div className="flex-1 overflow-y-auto scrollbar-thin">
          <div className="mx-auto w-full max-w-[72ch] px-4 py-8 sm:px-6 lg:px-8">
            <HelpDocTocMobile items={toc} />

            {notFound ? (
              <div className="space-y-4 rounded-lg border border-border p-6 sm:p-8">
                <div className="flex items-center gap-3 text-foreground">
                  <AlertTriangle className="w-5 h-5 text-warning-fg shrink-0" />
                  <h1 className="text-lg font-bold">{t("help.not_found_title", "That page isn't in this build")}</h1>
                </div>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  {t(
                    "help.not_found_body",
                    "No documentation is installed under this address. The plugin that owns it may not be part of this edition, or the page may have been renamed.",
                  )}
                </p>
                {docs.length > 0 && (
                  <button
                    onClick={() => navigate(getDocUrl(docs[0]))}
                    className="text-sm font-semibold text-accent-fg hover:underline"
                  >
                    {t("help.not_found_action", "Browse the documentation index")}
                  </button>
                )}
              </div>
            ) : loadingContent ? (
              <div className="flex items-center justify-center h-80">
                <p className="text-xs text-muted-foreground">
                  {t("help.loading", "Loading documentation…")}
                </p>
              </div>
            ) : error ? (
              <div className="p-6 bg-danger-bg text-danger-fg rounded-lg border border-danger-border text-sm flex items-start gap-3">
                <AlertTriangle className="w-5 h-5 shrink-0 mt-0.5" />
                <div className="flex-1 space-y-1.5 min-w-0">
                  <p className="font-semibold">{t("help.error_title", "Couldn't load this page")}</p>
                  <p className="break-words">{error}</p>
                  <div className="font-mono text-[11px] opacity-80 space-y-0.5">
                    <p>{t("help.error_path", "Path: {{path}}", { path: location.pathname })}</p>
                    {errorMeta?.status !== undefined && (
                      <p>{t("help.error_status", "Status: {{status}}", { status: errorMeta.status })}</p>
                    )}
                    {errorMeta?.requestId && (
                      <p>{t("help.error_request", "Request ID: {{requestId}}", { requestId: errorMeta.requestId })}</p>
                    )}
                  </div>
                </div>
                <button
                  onClick={() => setRetryNonce((n) => n + 1)}
                  className="shrink-0 px-3 py-1.5 rounded-md border border-danger-border text-xs font-semibold hover:bg-danger-bg transition-colors"
                >
                  {t("help.error_retry", "Retry")}
                </button>
              </div>
            ) : content ? (
              <div>
                <div className="mb-6 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
                  {categoryObj ? (
                    <span className="inline-flex items-center gap-1.5 rounded-full border border-accent-border bg-accent-soft px-2.5 py-0.5 text-[11px] font-semibold text-accent-fg">
                      <span className="w-1.5 h-1.5 rounded-full bg-current inline-block" />
                      {categoryObj.labels[lang] || categoryObj.labels["en"] || categoryObj.key}
                    </span>
                  ) : (
                    <span />
                  )}

                  <div className="flex items-center gap-3 font-medium">
                    <div className="flex items-center gap-1.5">
                      <Clock className="w-3.5 h-3.5 text-accent-fg" />
                      <span>
                        {readTimeMinutes} {t("help.read_time", "min read")}
                      </span>
                    </div>
                    <span>•</span>
                    <span>
                      {currentIndex} / {totalDocs}
                    </span>
                  </div>
                </div>

                <HelpArticle
                  title={content.title}
                  html={content.html}
                  slug={currentSlug}
                  lang={lang}
                  onNavigate={(path) => navigate(path)}
                  resolveDocPath={resolveDocPath}
                />

                <div className="mt-10">
                  <HelpDocPagination
                    prevDoc={prevDoc}
                    nextDoc={nextDoc}
                    onNavigateDoc={(doc) => navigate(getDocUrl(doc))}
                  />
                </div>
              </div>
            ) : null}
          </div>
        </div>

        <HelpDocToc items={toc} />
      </div>
    </div>
  );
}
