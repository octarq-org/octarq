import { useEffect, useCallback } from "react";
import type { HelpDocMeta } from "../../../api";

export interface UseDocDomEnhancerProps {
  contentRef: React.RefObject<HTMLDivElement | null>;
  html?: string;
  docs: HelpDocMeta[];
  routerBase: string;
  navigate: (path: string) => void;
  getDocUrl: (d: HelpDocMeta) => string;
  t: (key: string, fallback?: string) => string;
}

export function useDocDomEnhancer({
  contentRef,
  html,
  docs,
  routerBase,
  navigate,
  getDocUrl,
  t,
}: UseDocDomEnhancerProps) {
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
    [docs, getDocUrl],
  );

  // Post-process HTML content for interactive code blocks, responsive tables, callouts & SPA routing
  useEffect(() => {
    if (!contentRef.current || !html) return;

    // Rewrite in-app links, then intercept their clicks for SPA routing.
    const links = contentRef.current.querySelectorAll("a");
    links.forEach((a) => {
      if (a.dataset.navEnhanced || docs.length === 0) return;
      a.dataset.navEnhanced = "true";
      const href = a.getAttribute("href");
      if (href && href.startsWith("/") && !href.startsWith("//")) {
        const target = resolveInAppPath(href);
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

        const walker = document.createTreeWalker(bq, NodeFilter.SHOW_TEXT, null);
        let node = walker.nextNode();
        while (node) {
          if (node.nodeValue && /\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]/i.test(node.nodeValue)) {
            node.nodeValue = node.nodeValue.replace(/\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]/gi, "");
            break;
          }
          node = walker.nextNode();
        }

        const titleDiv = document.createElement("div");
        titleDiv.className = "callout-title";
        titleDiv.textContent = label;

        const contentDiv = document.createElement("div");
        while (bq.firstChild) {
          contentDiv.appendChild(bq.firstChild);
        }

        bq.appendChild(titleDiv);
        bq.appendChild(contentDiv);
      }
    });
  }, [html, t, docs.length, resolveInAppPath, routerBase, navigate]);
}
