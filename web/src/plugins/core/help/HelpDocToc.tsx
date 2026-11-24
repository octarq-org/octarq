import { useEffect, useState } from "react";
import { ListFilter } from "lucide-react";
import { Dialog } from "@octarq/plugin-sdk";
import { useTranslation } from "../../../i18n";

export interface TocItem {
  id: string;
  text: string;
  level: number;
}

function scrollToAnchor(id: string) {
  const prefersReduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  document.getElementById(id)?.scrollIntoView({
    behavior: prefersReduced ? "auto" : "smooth",
  });
}

function TocLinks({
  items,
  activeId,
  onSelect,
}: {
  items: TocItem[];
  activeId?: string;
  onSelect: (id: string) => void;
}) {
  return (
    <nav className="scrollbar-thin flex-1 space-y-0.5 overflow-y-auto text-xs">
      {items.map((item) => {
        const active = item.id === activeId;
        return (
          <a
            key={item.id}
            href={`#${item.id}`}
            aria-current={active ? "true" : undefined}
            onClick={(event) => {
              event.preventDefault();
              onSelect(item.id);
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
  );
}

export function HelpDocToc({ items }: { items: TocItem[] }) {
  const { t } = useTranslation();
  const [activeId, setActiveId] = useState<string | undefined>(items[0]?.id);

  useEffect(() => {
    if (items.length === 0) return;
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
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setActiveId(visible[0].target.id);
      },
      { rootMargin: "-88px 0px -65% 0px" },
    );
    headings.forEach((heading) => observer.observe(heading));
    return () => observer.disconnect();
  }, [items]);

  if (items.length === 0) return null;

  return (
    <div className="hidden h-full w-64 shrink-0 flex-col overflow-hidden border-l border-border bg-background p-6 lg:flex">
      <div className="mb-4 flex shrink-0 items-center gap-2 text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground">
        <ListFilter className="h-3.5 w-3.5 text-accent-fg" />
        <span>{t("help.on_this_page", "On this page")}</span>
      </div>
      <TocLinks items={items} activeId={activeId} onSelect={scrollToAnchor} />
    </div>
  );
}

export function HelpDocTocMobile({ items }: { items: TocItem[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  if (items.length === 0) return null;

  const title = t("help.on_this_page", "On this page");

  const handleSelect = (id: string) => {
    setOpen(false);
    scrollToAnchor(id);
  };

  return (
    <div className="sticky top-0 z-20 -mx-4 mb-6 flex bg-background px-4 py-2 lg:hidden">
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
        className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-border"
      >
        <ListFilter className="h-3.5 w-3.5 text-accent-fg" />
        <span>{title}</span>
      </button>
      <Dialog open={open} onOpenChange={setOpen} title={title}>
        <div className="flex max-h-[60vh] flex-col">
          <TocLinks items={items} onSelect={handleSelect} />
        </div>
      </Dialog>
    </div>
  );
}
