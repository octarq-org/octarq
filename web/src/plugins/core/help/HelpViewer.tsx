import { useEffect, useState, useMemo, useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";
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

  // Extract current slug from pathname: e.g. /help/services/ddns or /admin/help/services/ddns -> "ddns"
  const currentSlug = useMemo(() => {
    const normPath = location.pathname.startsWith("/admin/help")
      ? location.pathname.replace(/^\/admin\/help/, "/help")
      : location.pathname;
    const parts = normPath.split("/").filter(Boolean);
    if (parts.length >= 3) return parts[2]; // /help/category/slug
    if (parts.length > 1) return parts[parts.length - 1];
    return "";
  }, [location.pathname]);

  const [docs, setDocs] = useState<HelpDocMeta[]>([]);
  const [_loadingDocs, setLoadingDocs] = useState(true);

  const [content, setContent] = useState<DocContent | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);
  const [error, setError] = useState("");

  const [copiedLink, setCopiedLink] = useState(false);

  const contentRef = useRef<HTMLDivElement>(null);

  // Helper to build doc URL: /help/{category}/{slug}
  const getDocUrl = (d: HelpDocMeta) => {
    const category = (d.category || "services").toLowerCase();
    const prefix = location.pathname.startsWith("/admin/help")
      ? "/admin/help"
      : "/help";
    return `${prefix}/${category}/${d.slug}`;
  };

  // Fetch doc index & handle default redirect
  useEffect(() => {
    api
      .helpIndex(lang)
      .then((res) => {
        const list: HelpDocMeta[] = Array.isArray(res) ? res : (res as any)?.body || [];
        setDocs(list);
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
            // Slug not found in active doc index -> redirect to default first doc
            navigate(getDocUrl(list[0]), { replace: true });
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
    api
      .helpPage(currentSlug, lang)
      .then((res) => setContent(res?.html ? res : (res as any)?.body || null))
      .catch((err: any) =>
        setError(err.message || "Failed to load documentation"),
      )
      .finally(() => setLoadingContent(false));
  }, [currentSlug, lang]);

  // Post-process HTML content for interactive code blocks, responsive tables, callouts & SPA routing
  useEffect(() => {
    if (!contentRef.current || !content?.html) return;

    // Intercept internal link clicks for smooth single-page routing
    const links = contentRef.current.querySelectorAll("a");
    links.forEach((a) => {
      if (a.dataset.navEnhanced) return;
      a.dataset.navEnhanced = "true";
      const href = a.getAttribute("href");
      if (
        href &&
        !href.startsWith("http://") &&
        !href.startsWith("https://") &&
        !href.startsWith("mailto:") &&
        !href.startsWith("tel:")
      ) {
        a.onclick = (e) => {
          e.preventDefault();
          navigate(href);
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
          label = `💡 ${t("help.callout_tip", "提示")}`;
        } else if (tag === "WARNING" || tag === "CAUTION") {
          type = "warning";
          label = `⚠️ ${t("help.callout_warning", "警告")}`;
        } else if (tag === "IMPORTANT") {
          type = "important";
          label = `🚨 ${t("help.callout_important", "注意")}`;
        } else if (tag === "NOTE") {
          type = "note";
          label = `ℹ️ ${t("help.callout_note", "说明")}`;
        }
      }

      if (type) {
        bq.classList.add("callout", `callout-${type}`);
        const html = bq.innerHTML.replace(/\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]/gi, "");
        bq.innerHTML = `<div class="callout-title">${label}</div><div>${html}</div>`;
      }
    });
  }, [content?.html, t]);

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
      <div className="border-b border-border/60 bg-card/60 backdrop-blur-md px-6 py-3.5 flex flex-wrap items-center justify-between gap-4 shadow-xs shrink-0">
        {/* Left: Breadcrumbs */}
        <div className="flex items-center gap-2 text-xs">
          <div className="flex items-center gap-1.5 text-muted-foreground">
            <BookOpen className="w-4 h-4 text-primary" />
            <span className="font-semibold text-foreground">
              {t("help.title", "帮助与文档")}
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
              {copiedLink ? t("help.copied", "已复制") : t("help.copy_code", "分享")}
            </span>
          </button>
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 flex overflow-hidden">
        {/* Document Scroll Area */}
        <div className="flex-1 overflow-y-auto bg-surface-hover/20 scrollbar-thin p-4 sm:p-8 lg:p-12">
          <div className="max-w-4xl mx-auto space-y-8">
            {loadingContent ? (
              <div className="flex flex-col items-center justify-center h-80 gap-3">
                <div className="w-8 h-8 rounded-full border-2 border-primary border-t-transparent animate-spin" />
                <p className="text-xs text-muted-foreground animate-pulse">
                  {t("help.loading", "加载文档内容...")}
                </p>
              </div>
            ) : error ? (
              <div className="p-6 bg-destructive/10 text-destructive rounded-2xl border border-destructive/20 text-sm flex items-center gap-3 shadow-xs">
                <AlertTriangle className="w-5 h-5 shrink-0" />
                <span>{error}</span>
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
                          <span>{readTimeMinutes} {t("help.read_time", "分钟阅读")}</span>
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
                    dangerouslySetInnerHTML={{ __html: content.html }}
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
                          {t("help.prev_doc", "上一篇")}
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
                          {t("help.next_doc", "下一篇")}
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
          <div className="w-64 border-l border-border/60 bg-background/40 backdrop-blur-xs p-6 hidden lg:flex flex-col h-full overflow-hidden shrink-0">
            <div className="flex items-center gap-2 text-xs font-bold text-foreground mb-4 shrink-0">
              <ListFilter className="w-4 h-4 text-primary" />
              <span>{t("help.on_this_page", "本文目录")}</span>
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
