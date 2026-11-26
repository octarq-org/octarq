import { useMemo } from "react";
import { BookOpen, ChevronRight } from "lucide-react";
import type { HelpCategory, HelpDocMeta } from "../../../api";
import { useTranslation } from "../../../i18n";
import HelpSearch from "./HelpSearch";

export interface HelpBrowserProps {
  docs: HelpDocMeta[];
  categories: HelpCategory[];
  onSelect: (slug: string, anchor: string | null) => void;
  autoFocus?: boolean;
}

function parseDocUrl(url: string): { slug: string; anchor: string | null } | null {
  const [pathAndQuery, hash] = url.split("#");
  const path = pathAndQuery.split("?")[0].replace(/\/+$/, "");
  const match = path.match(/^\/help\/(?:[^/]+\/)?([^/]+)$/);
  if (!match) return null;
  return { slug: match[1], anchor: hash ? decodeURIComponent(hash) : null };
}

export default function HelpBrowser({ docs, categories, onSelect, autoFocus }: HelpBrowserProps) {
  const { t, lang } = useTranslation();

  const groups = useMemo(() => {
    const ordered = categories.slice().sort((a, b) => a.order - b.order);
    const known = new Set(ordered.map((category) => category.key));
    const groups = ordered
      .map((category) => ({
        category,
        docs: docs
          .filter((doc) => (doc.category || "").toLowerCase() === category.key)
          .sort((a, b) => (a.order ?? 0) - (b.order ?? 0) || a.title.localeCompare(b.title)),
      }))
      .filter((group) => group.docs.length > 0);
    const uncategorized = docs
      .filter((doc) => !known.has((doc.category || "").toLowerCase()))
      .sort((a, b) => (a.order ?? 0) - (b.order ?? 0) || a.title.localeCompare(b.title));
    if (uncategorized.length > 0) {
      groups.push({
        category: { key: "_other", order: 999, icon: "book-open", labels: {} },
        docs: uncategorized,
      });
    }
    return groups;
  }, [categories, docs]);

  const label = (category: HelpCategory) =>
    category.labels?.[lang] || category.labels?.en || category.key;

  const handleSelect = (url: string) => {
    const target = parseDocUrl(url);
    if (target) onSelect(target.slug, target.anchor);
  };

  return (
    <div className="space-y-6" data-testid="help-browser">
      <div>
        <h2 className="mb-2 text-sm font-semibold text-foreground">
          {t("help.browse_title", "Browse documentation")}
        </h2>
        <HelpSearch categories={categories} autoFocus={autoFocus} onSelect={handleSelect} />
      </div>

      {docs.length === 0 ? (
        <div className="flex items-start gap-3 rounded-lg border border-border p-4">
          <BookOpen className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium text-foreground">
              {t("help.hub_empty_title", "No documentation is installed")}
            </p>
            <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
              {t("help.hub_empty_body", "This edition has no help docs yet.")}
            </p>
          </div>
        </div>
      ) : (
        groups.map((group) => (
          <section key={group.category.key}>
            <h3 className="mb-1 text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground">
              {label(group.category)}
            </h3>
            <ul className="divide-y divide-border border-y border-border">
              {group.docs.map((doc) => (
                <li key={doc.slug}>
                  <button
                    type="button"
                    onClick={() => onSelect(doc.slug, null)}
                    data-testid={`help-browser-doc-${doc.slug}`}
                    className="flex w-full items-start gap-3 py-3 text-left transition-colors duration-100 hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium text-foreground">{doc.title}</span>
                      {doc.description ? (
                        <span className="mt-0.5 block text-xs leading-relaxed text-muted-foreground">
                          {doc.description}
                        </span>
                      ) : null}
                    </span>
                    <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          </section>
        ))
      )}
    </div>
  );
}
