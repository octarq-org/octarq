import { useEffect, useState, useMemo, useRef } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { api } from "../../../api";
import { useTranslation } from "../../../i18n";
import { toast } from "../../../ui";
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
  Sparkles,
  ThumbsUp,
  ThumbsDown,
} from "lucide-react";

interface DocMeta {
  slug: string;
  title: string;
  scope?: string;
  category?: string;
  group: string;
  groupOrder?: number;
  order: number;
}

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

  // Extract current slug from pathname: e.g. /help/plugins/dns/ddns or /admin/help/plugins/dns/ddns -> "ddns"
  const currentSlug = useMemo(() => {
    const normPath = location.pathname.startsWith("/admin/help")
      ? location.pathname.replace(/^\/admin\/help/, "/help")
      : location.pathname;
    const parts = normPath.split("/").filter(Boolean);
    if (parts.length >= 4) return parts[3]; // /help/scope/category/slug
    if (parts.length > 1) return parts[parts.length - 1];
    return "";
  }, [location.pathname]);

  const [docs, setDocs] = useState<DocMeta[]>([]);
  const [_loadingDocs, setLoadingDocs] = useState(true);

  const [content, setContent] = useState<DocContent | null>(null);
  const [loadingContent, setLoadingContent] = useState(false);
  const [error, setError] = useState("");

  const [copiedLink, setCopiedLink] = useState(false);

  const contentRef = useRef<HTMLDivElement>(null);

  // Helper to build doc URL
  const getDocUrl = (d: DocMeta) => {
    const scope = (
      d.scope ||
      (d.group === "Core" || d.group === "Platform"
        ? "platform"
        : d.group === "Portal"
        ? "portal"
        : "plugins")
    ).toLowerCase();
    const category = (d.category || d.group || "general").toLowerCase();
    const prefix = location.pathname.startsWith("/admin/help")
      ? "/admin/help"
      : "/help";
    return `${prefix}/${scope}/${category}/${d.slug}`;
  };

  // Fetch doc index & handle default redirect
  useEffect(() => {
    api
      .helpIndex(lang)
      .then((res) => {
        const list: DocMeta[] = Array.isArray(res) ? res : (res as any)?.body || [];
        const sortedList = [...list].sort((a, b) => {
          const groupOrderA = a.groupOrder ?? 999;
          const groupOrderB = b.groupOrder ?? 999;
          if (groupOrderA !== groupOrderB) return groupOrderA - groupOrderB;

          const orderA = a.order ?? 999;
          const orderB = b.order ?? 999;
          if (orderA !== orderB) return orderA - orderB;

          return (a.title || "").localeCompare(b.title || "");
        });

        setDocs(sortedList);
        if (!currentSlug && sortedList.length > 0) {
          navigate(getDocUrl(sortedList[0]), { replace: true });
        } else if (currentSlug && sortedList.length > 0) {
          const matched = sortedList.find((d) => d.slug === currentSlug);
          if (matched) {
            const canonicalUrl = getDocUrl(matched);
            if (location.pathname !== canonicalUrl) {
              navigate(canonicalUrl, { replace: true });
            }
          } else {
            // Slug not found in active doc index -> redirect to default first doc
            navigate(getDocUrl(sortedList[0]), { replace: true });
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

    // Style blockquotes as GFM callouts / alert cards
    const blockquotes = contentRef.current.querySelectorAll("blockquote");
    blockquotes.forEach((bq) => {
      if (bq.dataset.enhanced) return;
      bq.dataset.enhanced = "true";
      const text = bq.textContent || "";

      let type: "tip" | "warning" | "important" | "note" | null = null;
      let label = "";

      if (text.includes("[!TIP]") || text.includes("TIP") || text.includes("提示")) {
        type = "tip";
        label = `💡 ${t("help.callout_tip", "提示")}`;
      } else if (
        text.includes("[!WARNING]") ||
        text.includes("[!CAUTION]") ||
        text.includes("WARNING") ||
        text.includes("警告") ||
        text.includes("CAUTION")
      ) {
        type = "warning";
        label = `⚠️ ${t("help.callout_warning", "警告")}`;
      } else if (
        text.includes("[!IMPORTANT]") ||
        text.includes("IMPORTANT") ||
        text.includes("注意")
      ) {
        type = "important";
        label = `🚨 ${t("help.callout_important", "注意")}`;
      } else if (text.includes("[!NOTE]") || text.includes("NOTE") || text.includes("说明")) {
        type = "note";
        label = `ℹ️ ${t("help.callout_note", "说明")}`;
      }

      if (type) {
        bq.classList.add("callout", `callout-${type}`);
        const html = bq.innerHTML.replace(/\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]/gi, "");
        bq.innerHTML = `<div class="callout-title">${label}</div><div>${html}</div>`;
      }
    });
  }, [content?.html]);

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

  const currentDocMeta = useMemo(() => {
    return docs.find((d) => d.slug === currentSlug);
  }, [docs, currentSlug]);

  const handleCopyLink = () => {
    navigator.clipboard.writeText(window.location.href);
    setCopiedLink(true);
    setTimeout(() => setCopiedLink(false), 2000);
  };

  const isPlatformCore =
    currentDocMeta?.group === "Core" || currentDocMeta?.group === "Platform";
  const groupLabel = currentDocMeta
    ? isPlatformCore
      ? t("help.platform_title", "平台能力")
      : `${t("help.plugin_title", "插件能力")} / ${t(
          `help.group.${currentDocMeta.group}`,
          currentDocMeta.group,
        )}`
    : "";

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
              <span
                className={`px-2.5 py-0.5 rounded-full text-[11px] font-semibold border ${
                  isPlatformCore
                    ? "bg-primary/10 text-primary border-primary/20"
                    : "bg-purple-500/10 text-purple-600 dark:text-purple-400 border-purple-500/20"
                }`}
              >
                {groupLabel}
              </span>
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
                      <span
                        className={`text-xs font-bold px-3 py-1 rounded-full border shadow-2xs flex items-center gap-1.5 ${
                          isPlatformCore
                            ? "bg-primary/10 text-primary border-primary/20"
                            : "bg-purple-500/10 text-purple-600 dark:text-purple-400 border-purple-500/20"
                        }`}
                      >
                        <span className="w-1.5 h-1.5 rounded-full bg-current inline-block" />
                        {groupLabel}
                      </span>

                      <div className="flex items-center gap-3 text-xs text-muted-foreground/80 font-medium">
                        <div className="flex items-center gap-1.5">
                          <Clock className="w-3.5 h-3.5 text-primary" />
                          <span>3 {t("help.read_time", "分钟阅读")}</span>
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

                  {/* Feedback Rating Section */}
                  <div className="mt-12 pt-6 border-t border-border/40 flex flex-wrap items-center justify-between gap-4 text-xs text-muted-foreground">
                    <div className="flex items-center gap-2">
                      <Sparkles className="w-4 h-4 text-primary shrink-0" />
                      <span className="font-medium">
                        {t("help.feedback_title", "这份文档对您有帮助吗？")}
                      </span>
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() =>
                          toast.success(
                            t("help.feedback_thanks", "感谢您的反馈！"),
                          )
                        }
                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-border/80 bg-background hover:bg-surface-hover hover:text-foreground transition-all shadow-2xs font-medium"
                      >
                        <ThumbsUp className="w-3.5 h-3.5 text-muted-foreground" />
                        <span>{t("help.helpful", "有帮助")}</span>
                      </button>
                      <button
                        onClick={() =>
                          toast.success(
                            t(
                              "help.feedback_thanks",
                              "感谢您的反馈，我们会持续优化！",
                            ),
                          )
                        }
                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-border/80 bg-background hover:bg-surface-hover hover:text-foreground transition-all shadow-2xs font-medium"
                      >
                        <ThumbsDown className="w-3.5 h-3.5 text-muted-foreground" />
                        <span>{t("help.unhelpful", "需改进")}</span>
                      </button>
                    </div>
                  </div>
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
