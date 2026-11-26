import { useEffect, useId, useState, type KeyboardEvent, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Search } from "lucide-react";
import { Button, Input } from "@octarq/plugin-sdk";
import { api } from "../../../api";
import type { HelpCategory, HelpSearchHit, HelpSearchSnippet } from "../../../api";
import { useTranslation } from "../../../i18n";

const DEBOUNCE_MS = 250;
const RESULT_LIMIT = 20;

type Status = "idle" | "loading" | "ready" | "error" | "unavailable";

interface HelpSearchProps {
  categories: HelpCategory[];
  autoFocus?: boolean;
  onSelect?: (url: string) => void;
}

function HighlightedSnippet({ snippet }: { snippet: HelpSearchSnippet }) {
  const text = snippet.text ?? "";
  const ranges = (snippet.offsets ?? [])
    .filter((range) => Array.isArray(range) && range.length === 2)
    .map(([start, end]) => [Math.max(0, start), Math.min(text.length, end)] as const)
    .filter(([start, end]) => end > start)
    .sort((a, b) => a[0] - b[0]);

  const parts: ReactNode[] = [];
  let cursor = 0;
  ranges.forEach(([start, end], index) => {
    const from = Math.max(cursor, start);
    if (from > cursor) parts.push(text.slice(cursor, from));
    if (end > from) {
      parts.push(
        <mark key={`mark-${index}`} className="rounded-sm bg-accent-soft px-0.5 text-foreground">
          {text.slice(from, end)}
        </mark>,
      );
      cursor = end;
    }
  });
  if (cursor < text.length) parts.push(text.slice(cursor));
  return <>{parts}</>;
}

export default function HelpSearch({ categories, autoFocus, onSelect }: HelpSearchProps) {
  const { t, lang } = useTranslation();
  const navigate = useNavigate();
  const listId = `help-search-list-${useId().replace(/:/g, "")}`;

  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<Status>("idle");
  const [hits, setHits] = useState<HelpSearchHit[]>([]);
  const [activeIndex, setActiveIndex] = useState(-1);
  const [retryNonce, setRetryNonce] = useState(0);

  useEffect(() => {
    const q = query.trim();
    setActiveIndex(-1);
    if (!q) {
      setHits([]);
      setStatus("idle");
      return;
    }

    const controller = new AbortController();
    let cancelled = false;
    setHits([]);
    setStatus("loading");

    const timer = window.setTimeout(() => {
      api
        .helpSearch(q, lang, RESULT_LIMIT, undefined, controller.signal)
        .then((response) => {
          if (cancelled) return;
          setHits(response.results ?? []);
          setStatus("ready");
        })
        .catch((err: unknown) => {
          if (cancelled || controller.signal.aborted) return;
          const code = (err as { status?: number }).status;
          setHits([]);
          setStatus(code === 404 || code === 501 ? "unavailable" : "error");
        });
    }, DEBOUNCE_MS);

    return () => {
      cancelled = true;
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [query, lang, retryNonce]);

  const open = query.trim().length > 0 && status !== "idle";
  const optionId = (index: number) => `${listId}-option-${index}`;
  const activeHit = activeIndex >= 0 ? hits[activeIndex] : undefined;

  const goTo = (hit: HelpSearchHit) => {
    if (onSelect) {
      onSelect(hit.url);
      return;
    }
    navigate(hit.url);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      if (query) event.preventDefault();
      setQuery("");
      setHits([]);
      setActiveIndex(-1);
      setStatus("idle");
      return;
    }
    if (event.key === "ArrowDown") {
      if (status !== "ready" || hits.length === 0) return;
      event.preventDefault();
      setActiveIndex((current) => (current + 1 < hits.length ? current + 1 : 0));
      return;
    }
    if (event.key === "ArrowUp") {
      if (status !== "ready" || hits.length === 0) return;
      event.preventDefault();
      setActiveIndex((current) => (current <= 0 ? hits.length - 1 : current - 1));
      return;
    }
    if (event.key === "Enter" && status === "ready") {
      const hit = activeHit ?? hits[0];
      if (!hit) return;
      event.preventDefault();
      goTo(hit);
    }
  };

  const categoryLabel = (key: string) => {
    const category = categories.find((item) => item.key === key);
    return category?.labels[lang] || category?.labels.en || key;
  };

  return (
    <div className="w-full">
      <div className="relative">
        <Search
          className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          type="search"
          role="combobox"
          aria-expanded={open}
          aria-controls={open ? listId : undefined}
          aria-autocomplete="list"
          aria-activedescendant={activeHit ? optionId(activeIndex) : undefined}
          aria-label={t("help.search_label", "Search documentation")}
          placeholder={t("help.search_placeholder", "Search the documentation")}
          value={query}
          autoFocus={autoFocus}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={onKeyDown}
          className="h-12 pl-10 text-[15px]"
        />
      </div>

      {open ? (
        <div className="mt-2 overflow-hidden rounded-lg border border-border bg-card">
          {status === "loading" ? (
            <p className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground" role="status">
              <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
              {t("help.search_loading", "Searching…")}
            </p>
          ) : status === "unavailable" ? (
            <p className="px-4 py-3 text-xs text-muted-foreground" role="status">
              {t("help.search_unavailable", "Search is not available in this build.")}
            </p>
          ) : status === "error" ? (
            <div className="flex items-center justify-between gap-3 px-4 py-3" role="status">
              <p className="text-xs text-muted-foreground">{t("help.search_error", "Search failed. Try again.")}</p>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() => setRetryNonce((value) => value + 1)}
              >
                {t("help.error_retry", "Retry")}
              </Button>
            </div>
          ) : hits.length === 0 ? (
            <p className="px-4 py-3 text-xs text-muted-foreground" role="status">
              {t("help.search_no_results", "No results for “{{query}}”", { query: query.trim() })}
            </p>
          ) : (
            <div id={listId} role="listbox" aria-label={t("help.search_results", "Search results")}>
              {hits.map((hit, index) => (
                <button
                  key={`${hit.url}-${hit.anchor}-${index}`}
                  type="button"
                  role="option"
                  id={optionId(index)}
                  aria-selected={index === activeIndex}
                  onMouseEnter={() => setActiveIndex(index)}
                  onClick={() => goTo(hit)}
                  className={`flex w-full flex-col gap-1 border-b border-border px-4 py-3 text-left transition-colors duration-100 last:border-b-0 hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${
                    index === activeIndex ? "bg-surface-hover" : "bg-card"
                  }`}
                >
                  <span className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <span className="text-sm font-medium text-foreground">{hit.title}</span>
                    <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                      {categoryLabel(hit.category)}
                    </span>
                  </span>
                  <span className="text-xs leading-relaxed text-muted-foreground">
                    <HighlightedSnippet snippet={hit.snippet} />
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}
