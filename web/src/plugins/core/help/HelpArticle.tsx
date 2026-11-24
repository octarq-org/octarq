import {
  Fragment,
  type AnchorHTMLAttributes,
  type MouseEvent as ReactMouseEvent,
} from "react";
import { useHref, useNavigate } from "react-router-dom";
import DOMPurify from "dompurify";
import parse, {
  attributesToProps,
  domToReact,
  type DOMNode,
  type Element,
  type HTMLReactParserOptions,
  type Text,
} from "html-react-parser";
import type { Lang } from "../../../i18n";
import { Table } from "@octarq/plugin-sdk";
import { Callout, type CalloutType } from "./Callout";
import { CodeBlock } from "./CodeBlock";

const ALERT_MARKER = /^\s*\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]\s*/i;
const ALERT_MARKER_GLOBAL = /^(\s*)\[!(TIP|WARNING|CAUTION|IMPORTANT|NOTE)\]\s*/i;
const LANGUAGE_CLASS = /(?:^|\s)language-([\w-]+)/;

export interface HelpArticleProps {
  title: string;
  html: string;
  slug: string;
  lang: Lang;
  onNavigate?: (path: string) => void;
  resolveDocPath?: (slug: string) => string | null;
}

function nodeText(node: unknown): string {
  const current = node as { type?: string; data?: string; children?: unknown[] };
  if (current?.type === "text") return current.data ?? "";
  if (Array.isArray(current?.children)) return current.children.map(nodeText).join("");
  return "";
}

function firstElement(node: Element, name: string): Element | undefined {
  return node.children.find(
    (child): child is Element => child.type === "tag" && child.name === name,
  );
}

function alertTypeOf(blockquote: Element): CalloutType | null {
  const paragraph = firstElement(blockquote, "p");
  if (!paragraph) return null;
  const match = ALERT_MARKER.exec(nodeText(paragraph));
  if (!match) return null;
  const tag = match[1].toUpperCase();
  if (tag === "TIP") return "tip";
  if (tag === "WARNING" || tag === "CAUTION") return "warning";
  if (tag === "IMPORTANT") return "important";
  return "note";
}

function alertParagraph(paragraph: Element, options: HTMLReactParserOptions) {
  let stripped = false;
  return (
    <p>
      {paragraph.children.map((child, index) => {
        if (!stripped && child.type === "text") {
          const data = (child as Text).data;
          const next = data.replace(ALERT_MARKER_GLOBAL, "");
          if (next !== data) {
            stripped = true;
            return <Fragment key={index}>{next}</Fragment>;
          }
        }
        return <Fragment key={index}>{domToReact([child as unknown as DOMNode], options)}</Fragment>;
      })}
    </p>
  );
}

function scrollToAnchor(id: string) {
  const target =
    document.getElementById(id) ??
    document.getElementById(decodeURIComponent(id));
  if (!target) return;
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  target.scrollIntoView({ behavior: reduced ? "auto" : "smooth" });
}

export function HelpArticle({
  title,
  html,
  slug,
  lang,
  onNavigate,
  resolveDocPath,
}: HelpArticleProps) {
  const navigate = useNavigate();
  const routerBase = useHref("/").replace(/\/$/, "");

  const resolveDocHref = (href: string) => {
    if (!href.startsWith("/help/")) return href;
    const [path, hash] = href.split("#");
    const parts = path.split("/").filter(Boolean);
    const docSlug = parts[parts.length - 1] ?? "";
    const resolved = resolveDocPath?.(docSlug) ?? path;
    return hash ? `${resolved}#${hash}` : resolved;
  };

  const go = (path: string) => {
    if (onNavigate) onNavigate(path);
    else navigate(path);
  };

  const options: HTMLReactParserOptions = {
    replace: (node) => {
      if (node.type !== "tag") return;

      if (node.name === "pre") {
        const codeElement = firstElement(node, "code");
        const language = codeElement?.attribs.class?.match(LANGUAGE_CLASS)?.[1];
        return (
          <CodeBlock
            code={codeElement ? nodeText(codeElement) : nodeText(node)}
            language={language}
          />
        );
      }

      if (node.name === "blockquote") {
        const type = alertTypeOf(node);
        if (!type) return;
        const paragraph = firstElement(node, "p");
        return (
          <Callout type={type}>
            {node.children.map((child, index) => (
              <Fragment key={index}>
                {paragraph && child === paragraph
                  ? alertParagraph(paragraph, options)
                  : domToReact([child as unknown as DOMNode], options)}
              </Fragment>
            ))}
          </Callout>
        );
      }

      if (node.name === "table") {
        return (
          <div className="table-wrapper">
            <Table {...attributesToProps(node.attribs)}>
              {domToReact(node.children as unknown as DOMNode[], options)}
            </Table>
          </div>
        );
      }

      if (node.name === "a") {
        const props = attributesToProps(node.attribs) as AnchorHTMLAttributes<HTMLAnchorElement>;
        const rawHref = props.href ?? "";
        const internal = rawHref.startsWith("/") && !rawHref.startsWith("//");
        const hashOnly = rawHref.startsWith("#");
        const resolved = internal ? resolveDocHref(rawHref) : rawHref;

        const handleClick = (event: ReactMouseEvent<HTMLAnchorElement>) => {
          if (hashOnly) {
            event.preventDefault();
            scrollToAnchor(resolved.slice(1));
            return;
          }
          if (!internal) return;
          event.preventDefault();
          go(resolved);
        };

        return (
          <a {...props} href={internal ? `${routerBase}${resolved}` : resolved} onClick={handleClick}>
            {domToReact(node.children as unknown as DOMNode[], options)}
          </a>
        );
      }
    },
  };

  return (
    <article lang={lang} data-help-slug={slug} className="mx-auto w-full max-w-[72ch]">
      <header className="mb-8 border-b border-border pb-6">
        <h1 className="font-display text-[1.75rem] font-semibold leading-[1.15] tracking-[-0.03em] text-foreground sm:text-[2rem]">
          {title}
        </h1>
      </header>
      <div className="markdown-body">
        {parse(DOMPurify.sanitize(html), options)}
      </div>
    </article>
  );
}
