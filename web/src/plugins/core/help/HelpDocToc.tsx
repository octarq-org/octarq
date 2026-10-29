import { useEffect, useState } from "react";
import { ListFilter } from "lucide-react";
import { useTranslation } from "../../../i18n";

export interface TocItem {
  id: string;
  text: string;
  level: number;
}

export function HelpDocToc({ items }: { items: TocItem[] }) {
  const { t } = useTranslation();
  const [activeId, setActiveId] = useState<string | undefined>(items[0]?.id);

  // Track the heading the reader is inside, so the rail always answers "where
  // am I in this page" instead of being a static list of links.
  useEffect(() => {
    if (items.length === 0) return;
    // The doc arrives after this component mounts, so the state seeded at
    // render time is undefined. Seed it from the first heading once the list
    // is known, otherwise the rail shows no current section at page top.
    setActiveId((current) =>
      current && items.some((item) => item.id === current) ? current : items[0].id,
    );
    const headings = items
      .map((item) => document.getElementById(item.id))
      .filter((el): el is HTMLElement => el !== null);
    if (headings.length === 0) return;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setActiveId(visible[0].target.id);
      },
      { rootMargin: "-88px 0px -65% 0px" },
    );
    headings.forEach((h) => observer.observe(h));
    return () => observer.disconnect();
  }, [items]);

  if (items.length === 0) return null;

  return (
    <div className="hidden h-full w-64 shrink-0 flex-col overflow-hidden border-l border-border bg-background p-6 lg:flex">
      <div className="mb-4 flex shrink-0 items-center gap-2 text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground">
        <ListFilter className="h-3.5 w-3.5 text-accent-fg" />
        <span>{t("help.on_this_page", "On this page")}</span>
      </div>
      <nav className="scrollbar-thin flex-1 space-y-0.5 overflow-y-auto text-xs">
        {items.map((item) => {
          const active = item.id === activeId;
          return (
            <a
              key={item.id}
              href={`#${item.id}`}
              aria-current={active ? "true" : undefined}
              onClick={(e) => {
                e.preventDefault();
                const prefersReduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
                document
                  .getElementById(item.id)
                  ?.scrollIntoView({ behavior: prefersReduced ? "auto" : "smooth" });
              }}
              className={`block truncate rounded-md border-l-2 px-2 py-1.5 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-border ${
                item.level === 3 ? "pl-4 text-[11px]" : "font-medium"
              } ${
                active
                  ? "border-primary bg-accent-soft text-accent-fg"
                  : "border-transparent text-muted-foreground hover:bg-surface-hover hover:text-foreground"
              }`}
            >
              {item.text}
            </a>
          );
        })}
      </nav>
    </div>
  );
}
