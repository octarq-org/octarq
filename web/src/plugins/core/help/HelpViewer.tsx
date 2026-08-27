import { useCallback, useEffect, useState, useMemo, useRef } from "react";
import { useHref, useLocation, useNavigate } from "react-router-dom";
import DOMPurify from "dompurify";
import { api, HelpCategory, HelpDocMeta } from "../../../api";
import { useTranslation } from "../../../i18n";
import {
  BookOpen,
  ChevronRight,
  ArrowLeft,
  ArrowRight,
  Check,
  ListFilter,
  AlertTriangle,
  Clock,
  Share2,
} from "lucide-react";

interface DocContent {
  title: string;
  html: string;
}

interface TocItem {
  id: string;
  text: string;
  level: number;
}

export default function HelpViewer() {
  const { t, lang } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();

  // Router basename prefix for markdown href links.
  const routerBase = useHref("/").replace(/\/$/, "");

  // Extract current slug from pathname: e.g. /help/services/ddns -> "ddns"
  const currentSlug = useMemo(() => {
    const parts = location.pathname.split("/").filter(Boolean);
    if (parts.length >= 3) return parts[2]; // /help/category/slug
    if (parts.length > 1) return parts[parts.length - 1];
    return "";
  }, [location.pathname]);

  const [docs, setDocs] = useState<HelpDocMeta[]>([]);
  const [_loadingDocs, setLoadingDocs] = useState(true);

  const [content, setContent] = useState<DocContent | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);
  const [error, setError] = useState("");
  // HTTP status + X-Request-Id from the failed load: the operator's log
  // correlation for a self-hosted instance. Both machine values → mono.
  const [errorMeta, setErrorMeta] = useState<{ status?: number; requestId?: string } | null>(null);
  const [retryNonce, setRetryNonce] = useState(0);

  const [copiedLink, setCopiedLink] = useState(false);
  // Set when the requested doc is unavailable (e.g. Pro slug on OSS instance).
  const [notFound, setNotFound] = useState(false);

  const contentRef = useRef<HTMLDivElement>(null);

  // Helper to build doc URL: /help/{category}/{slug}
  const getDocUrl = (d: HelpDocMeta) => {
    const category = (d.category || "services").toLowerCase();
    return `/help/${category}/${d.slug}`;
  };

  // Fetch doc index & handle default redirect
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

  // Fetch doc content
  useEffect(() => {
    if (!currentSlug) return;
    setLoadingContent(true);
    setError("");
    setErrorMeta(null);
    api
      .helpPage(currentSlug, lang)
      .then((res) => setContent(res))
      .catch((err: any) => {
        setError(err.message || "Failed to load documentation");
        setErrorMeta({ status: err?.status, requestId: err?.requestId });
      })
      .finally(() => setLoadingContent(false));
  }, [currentSlug, lang, retryNonce]);

  // Expands 2-segment /help/<slug> links to include category using doc index.
  const resolveInAppPath = useCallback(
    (href: string) => {
      if (!href.startsWith("/help/")) return href;
      const [path, hash] = href.split("#");
      const parts = path.split("/").filter(Boolean); // ["help", …]
      const slug = parts[parts.length - 1];
      const doc = docs.find((d) => d.slug === slug);
      const resolved = doc ? getDocUrl(doc) : path;
      return hash ? `${resolved}#${hash}` : resolved;
    },
    [docs],
  );

  // Post-process HTML content for interactive code blocks, responsive tables, callouts & SPA routing
  useEffect(() => {
    if (!contentRef.current || !content?.html) return;

    // Rewrite in-app links, then intercept their clicks for SPA routing.
    const links = contentRef.current.querySelectorAll("a");
    links.forEach((a) => {
      // Resolution needs the doc index; without it every /help/<slug> would be
      // frozen at its unexpanded form. The effect re-runs when it arrives.
      if (a.dataset.navEnhanced || docs.length === 0) return;
      a.dataset.navEnhanced = "true";
      const href = a.getAttribute("href");
      if (
        href &&
        href.startsWith("/") &&
        !href.startsWith("//")
      ) {
        const target = resolveInAppPath(href);
        // The href carries the basename so the URL the browser exposes is real;
        // navigate() takes the router path, which does not.
        a.setAttribute("href", routerBase + target);
        a.onclick = (e) => {
          e.preventDefault();
          navigate(target);
        };
      }
    });

    // Wrap tables in responsive scroll wrapper
    const tables = contentRef.current.querySelectorAll("table");
    tables.forEach((table) => {
      if (table.parentElement?.classList.contains("table-wrapper")) return;
      const wrapper = document.createElement("div");
      wrapper.className = "table-wrapper";
      table.parentNode?.insertBefore(wrapper, table);
      wrapper.appendChild(table);
    });

    // Add code block headers with language badges & copy buttons
    const preBlocks = contentRef.current.querySelectorAll("pre");
    preBlocks.forEach((pre) => {
      if (pre.dataset.enhanced) return;
      pre.dataset.enhanced = "true";

      const wrapper = document.createElement("div");
      wrapper.className = "code-block-wrapper";

      const header = document.createElement("div");
      header.className = "code-block-header";

      const codeElem = pre.querySelector("code");
      const langClass = Array.from(codeElem?.classList || []).find((c) =>
        c.startsWith("language-"),
      );
      const langText = langClass
        ? langClass.replace("language-", "").toUpperCase()
        : "CODE";

      const langSpan = document.createElement("span");
      langSpan.textContent = langText;
      langSpan.className = "code-block-lang";

      const copyBtn = document.createElement("button");
      copyBtn.className = "code-block-copy-btn";
      copyBtn.innerHTML = `<span>Copy</span>`;

      copyBtn.onclick = () => {
        const text = pre.textContent || "";
        navigator.clipboard.writeText(text);
        copyBtn.innerHTML = `<span style="color:#10b981;font-weight:700">Copied!</span>`;
        setTimeout(() => {
          copyBtn.innerHTML = `<span>Copy</span>`;
        }, 2000);
      };

      header.appendChild(langSpan);
      header.appendChild(copyBtn);

      pre.parentNode?.insertBefore(wrapper, pre);
      wrapper.appendChild(header);
      wrapper.appendChild(pre);
    });

    // Style blockquotes as GFM callouts / alert cards based strictly on [!TYPE] tag at the start
    const blockquotes = contentRef.current.querySelectorAll("blockquote");
    blockquotes.forEach((bq) => {
      if (bq.dataset.enhanced) return;
      bq.dataset.enhanced = "true";
      const text = (bq.textContent || "").trim();

      let type: "tip" | "warning" | "important" | "note" | null = null;
      let label = "";

      const match = text.match(/^\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]/i);
      if (match) {
        const tag = match[1].toUpperCase();
        if (tag === "TIP") {
          type = "tip";
          label = `💡 ${t("help.callout_tip", "TIP")}`;
        } else if (tag === "WARNING" || tag === "CAUTION") {
          type = "warning";
          label = `⚠️ ${t("help.callout_warning", "WARNING")}`;
        } else if (tag === "IMPORTANT") {
          type = "important";
          label = `🚨 ${t("help.callout_important", "IMPORTANT")}`;
        } else if (tag === "NOTE") {
          type = "note";
          label = `ℹ️ ${t("help.callout_note", "NOTE")}`;
        }
      }

      if (type) {
        bq.classList.add("callout", `callout-${type}`);
        const html = bq.innerHTML.replace(/\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]/gi, "");
        bq.innerHTML = `<div class="callout-title">${label}</div><div>${html}</div>`;
      }
    });
  }, [content?.html, t, docs.length, resolveInAppPath, routerBase]);

  // Extract Table of Contents from HTML content
  const toc = useMemo(() => {
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

  // Previous & Next doc calculation
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

  return (
    <div className="h-full flex flex-col bg-background text-foreground overflow-hidden">
      {/* Top Breadcrumb & Share Action Header */}
      <div className="border-b border-border/60 bg-card px-6 py-3.5 flex flex-wrap items-center justify-between gap-4 shrink-0">
        {/* Left: Breadcrumbs */}
        <div className="flex items-center gap-2 text-xs">
          <div className="flex items-center gap-1.5 text-muted-foreground">
            <BookOpen className="w-4 h-4 text-primary" />
            <span className="font-semibold text-foreground">
              {t("help.title", "Help & Documentation")}
            </span>
          </div>
          {currentDocMeta && (
            <>
              <ChevronRight className="w-3.5 h-3.5 text-muted-foreground/50" />
              {categoryObj && (
                <span
                  className="px-2.5 py-0.5 rounded-full text-[11px] font-semibold border bg-primary/10 text-primary border-primary/20"
                >
                  {categoryObj.labels[lang] || categoryObj.labels["en"] || categoryObj.key}
                </span>
              )}
              <ChevronRight className="w-3.5 h-3.5 text-muted-foreground/50" />
              <span className="font-bold text-foreground truncate max-w-xs sm:max-w-sm">
                {currentDocMeta.title}
              </span>
            </>
          )}
        </div>

        {/* Right: Copy Link / Share Action */}
        <div className="flex items-center gap-3">
          <button
            onClick={handleCopyLink}
            title={t("help.copy_page_link", "Copy Page Link")}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-border/80 bg-background hover:bg-surface-hover text-xs font-medium text-muted-foreground hover:text-foreground transition-all shadow-xs"
          >
            {copiedLink ? (
              <Check className="w-3.5 h-3.5 text-emerald-500" /* ui-color-ok */ />
            ) : (
              <Share2 className="w-3.5 h-3.5" />
            )}
            <span className="hidden sm:inline">
              {copiedLink ? t("help.copied", "Copied!") : t("help.copy_code", "Share")}
            </span>
          </button>
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 flex overflow-hidden">
        {/* Document Scroll Area */}
        <div className="flex-1 overflow-y-auto bg-surface-hover/20 scrollbar-thin p-4 sm:p-8 lg:p-12">
          <div className="max-w-4xl mx-auto space-y-8">
            {notFound ? (
              <div className="bg-card rounded-2xl border border-border/80 p-8 shadow-xs space-y-4">
                <div className="flex items-center gap-3 text-foreground">
                  <AlertTriangle className="w-5 h-5 text-warning-fg shrink-0" />
                  <h1 className="text-lg font-bold">{t("help.not_found_title", "That page isn't in this build")}</h1>
                </div>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  {t("help.not_found_body", "No documentation is installed under this address. The plugin that owns it may not be part of this edition, or the page may have been renamed.")}
                </p>
                {docs.length > 0 && (
                  <button
                    onClick={() => navigate(getDocUrl(docs[0]))}
                    className="text-sm font-semibold text-primary hover:underline"
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
              <div className="p-6 bg-destructive/10 text-destructive rounded-lg border border-destructive/20 text-sm flex items-start gap-3">
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
                  className="shrink-0 px-3 py-1.5 rounded-lg border border-destructive/30 text-xs font-semibold hover:bg-destructive/20 transition-colors"
                >
                  {t("help.error_retry", "Retry")}
                </button>
              </div>
            ) : content ? (
              <div className="space-y-8">
                {/* Main Glass/Card Article Container */}
                <div className="bg-card rounded-2xl border border-border/80 p-6 sm:p-10 md:p-12 shadow-xs transition-all">
                  {/* Document Title & Meta Header */}
                  <div className="space-y-4 border-b border-border/50 pb-6 mb-8">
                    <div className="flex flex-wrap items-center justify-between gap-4">
                      {categoryObj && (
                        <span
                          className="text-xs font-bold px-3 py-1 rounded-full border shadow-2xs flex items-center gap-1.5 bg-primary/10 text-primary border-primary/20"
                        >
                          <span className="w-1.5 h-1.5 rounded-full bg-current inline-block" />
                          {categoryObj.labels[lang] || categoryObj.labels["en"] || categoryObj.key}
                        </span>
                      )}

                      <div className="flex items-center gap-3 text-xs text-muted-foreground/80 font-medium">
                        <div className="flex items-center gap-1.5">
                          <Clock className="w-3.5 h-3.5 text-primary" />
                          <span>{readTimeMinutes} {t("help.read_time", "min read")}</span>
                        </div>
                        <span>•</span>
                        <span>
                          {currentIndex} / {totalDocs}
                        </span>
                      </div>
                    </div>

                    <h1 className="text-3xl sm:text-4xl md:text-[2.5rem] font-extrabold tracking-tight text-foreground leading-tight">
                      {content.title}
                    </h1>
                  </div>

                  {/* Rendered Markdown HTML with Open-Source Doc Typography */}
                  <div
                    ref={contentRef}
                    className="markdown-body"
                    dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(content.html) }}
                  />
                </div>

                {/* Bottom Pagination Controls */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
                  {prevDoc ? (
                    <button
                      onClick={() => navigate(getDocUrl(prevDoc))}
                      className="p-4 rounded-2xl border border-border/80 bg-card hover:bg-surface-hover text-left transition-all group flex items-start gap-3 shadow-xs"
                    >
                      <ArrowLeft className="w-4 h-4 text-muted-foreground group-hover:text-primary group-hover:-translate-x-1 transition-all mt-0.5 shrink-0" />
                      <div>
                        <div className="text-[10px] text-muted-foreground uppercase font-semibold">
                          {t("help.prev_doc", "Previous")}
                        </div>
                        <div className="text-xs font-bold text-foreground group-hover:text-primary transition-colors">
                          {prevDoc.title}
                        </div>
                      </div>
                    </button>
                  ) : (
                    <div />
                  )}

                  {nextDoc ? (
                    <button
                      onClick={() => navigate(getDocUrl(nextDoc))}
                      className="p-4 rounded-2xl border border-border/80 bg-card hover:bg-surface-hover text-right transition-all group flex items-start justify-end gap-3 shadow-xs"
                    >
                      <div>
                        <div className="text-[10px] text-muted-foreground uppercase font-semibold">
                          {t("help.next_doc", "Next")}
                        </div>
                        <div className="text-xs font-bold text-foreground group-hover:text-primary transition-colors">
                          {nextDoc.title}
                        </div>
                      </div>
                      <ArrowRight className="w-4 h-4 text-muted-foreground group-hover:text-primary group-hover:translate-x-1 transition-all mt-0.5 shrink-0" />
                    </button>
                  ) : (
                    <div />
                  )}
                </div>
              </div>
            ) : null}
          </div>
        </div>

        {/* Right Table of Contents Sidebar (Desktop) */}
        {toc.length > 0 && (
          <div className="w-64 border-l border-border/60 bg-background p-6 hidden lg:flex flex-col h-full overflow-hidden shrink-0">
            <div className="flex items-center gap-2 text-xs font-bold text-foreground mb-4 shrink-0">
              <ListFilter className="w-4 h-4 text-primary" />
              <span>{t("help.on_this_page", "On this page")}</span>
            </div>
            <nav className="flex-1 overflow-y-auto scrollbar-thin space-y-1.5 text-xs border-l-2 border-border/40 ml-1 pl-2 pr-1">
              {toc.map((item) => (
                <a
                  key={item.id}
                  href={`#${item.id}`}
                  onClick={(e) => {
                    e.preventDefault();
                    document
                      .getElementById(item.id)
                      ?.scrollIntoView({ behavior: "smooth" });
                  }}
                  className={`block py-1 px-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-surface-hover transition-all truncate ${
                    item.level === 3 ? "pl-4 text-[11px]" : "font-medium"
                  }`}
                >
                  {item.text}
                </a>
              ))}
            </nav>
          </div>
        )}
      </div>
    </div>
  );
}
