import { ArrowLeft, ArrowRight } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { HelpDocMeta } from "../../../api";

export interface HelpDocPaginationProps {
  prevDoc: HelpDocMeta | null;
  nextDoc: HelpDocMeta | null;
  onNavigateDoc: (doc: HelpDocMeta) => void;
}

export function HelpDocPagination({
  prevDoc,
  nextDoc,
  onNavigateDoc,
}: HelpDocPaginationProps) {
  const { t } = useTranslation();

  if (!prevDoc && !nextDoc) return null;

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2">
      {prevDoc ? (
        <button
          onClick={() => onNavigateDoc(prevDoc)}
          className="p-4 rounded-2xl border border-border/80 bg-card hover:bg-surface-hover text-left transition-colors group flex items-start gap-3 shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <ArrowLeft
            className="w-4 h-4 text-muted-foreground group-hover:text-primary group-hover:-translate-x-1 transition-transform mt-0.5 shrink-0"
            aria-hidden="true"
          />
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
          onClick={() => onNavigateDoc(nextDoc)}
          className="p-4 rounded-2xl border border-border/80 bg-card hover:bg-surface-hover text-right transition-colors group flex items-start justify-end gap-3 shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <div>
            <div className="text-[10px] text-muted-foreground uppercase font-semibold">
              {t("help.next_doc", "Next")}
            </div>
            <div className="text-xs font-bold text-foreground group-hover:text-primary transition-colors">
              {nextDoc.title}
            </div>
          </div>
          <ArrowRight
            className="w-4 h-4 text-muted-foreground group-hover:text-primary group-hover:translate-x-1 transition-transform mt-0.5 shrink-0"
            aria-hidden="true"
          />
        </button>
      ) : (
        <div />
      )}
    </div>
  );
}
