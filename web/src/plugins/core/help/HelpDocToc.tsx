import { ListFilter } from "lucide-react";
import { useTranslation } from "../../../i18n";

export interface TocItem {
  id: string;
  text: string;
  level: number;
}

export function HelpDocToc({ items }: { items: TocItem[] }) {
  const { t } = useTranslation();

  if (items.length === 0) return null;

  return (
    <div className="w-64 border-l border-border/60 bg-background p-6 hidden lg:flex flex-col h-full overflow-hidden shrink-0">
      <div className="flex items-center gap-2 text-xs font-bold text-foreground mb-4 shrink-0">
        <ListFilter className="w-4 h-4 text-primary" />
        <span>{t("help.on_this_page", "On this page")}</span>
      </div>
      <nav className="flex-1 overflow-y-auto scrollbar-thin space-y-1.5 text-xs border-l-2 border-border/40 ml-1 pl-2 pr-1">
        {items.map((item) => (
          <a
            key={item.id}
            href={`#${item.id}`}
            onClick={(e) => {
              e.preventDefault();
              const prefersReduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
              document
                .getElementById(item.id)
                ?.scrollIntoView({ behavior: prefersReduced ? "auto" : "smooth" });
            }}
            className={`block py-1 px-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-surface-hover transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary truncate ${
              item.level === 3 ? "pl-4 text-[11px]" : "font-medium"
            }`}
          >
            {item.text}
          </a>
        ))}
      </nav>
    </div>
  );
}
