import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "../../ui";
import { useTranslation } from "../../i18n";
import { ProTablePaginationConfig } from "./types";

export interface TablePaginationProps {
  page: number;
  pageSize: number;
  total: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  config?: boolean | ProTablePaginationConfig;
}

export function TablePagination({
  page,
  pageSize,
  total,
  pageCount,
  onPageChange,
  onPageSizeChange,
  config = true,
}: TablePaginationProps) {
  const { t } = useTranslation();

  if (config === false) {
    return null;
  }

  const paginationConfig = typeof config === "object" ? config : {};
  const pageSizeOptions = paginationConfig.pageSizeOptions ?? [10, 20, 50, 100];
  const hideOnSinglePage = paginationConfig.hideOnSinglePage ?? false;

  if (hideOnSinglePage && total <= pageSize) {
    return null;
  }

  const canPrev = page > 1;
  const canNext = page < pageCount;

  // Generate visible page numbers
  const pages: number[] = [];
  const maxButtons = 5;
  let start = Math.max(1, page - Math.floor(maxButtons / 2));
  let end = Math.min(pageCount, start + maxButtons - 1);
  if (end - start + 1 < maxButtons) {
    start = Math.max(1, end - maxButtons + 1);
  }
  for (let i = start; i <= end; i++) {
    pages.push(i);
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-4 border-t border-foreground/[0.06] px-4 py-3 text-xs text-foreground/70">
      {/* Total items and current page info */}
      <div className="flex items-center gap-3">
        <span className="font-medium">
          {t("proTable.totalItems", { total })}
        </span>
        <span className="text-foreground/40">|</span>
        <span>
          {t("proTable.pageOf", { page, totalPages: Math.max(1, pageCount) })}
        </span>
      </div>

      {/* Controls: Page size selector and navigation buttons */}
      <div className="flex items-center gap-2">
        <select
          value={pageSize}
          onChange={(e) => {
            const newSize = Number(e.target.value);
            onPageSizeChange(newSize);
            onPageChange(1);
          }}
          aria-label={t("proTable.perPage", { size: pageSize })}
          className="rounded-lg border border-border bg-card/60 px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
        >
          {pageSizeOptions.map((opt) => (
            <option key={opt} value={opt}>
              {t("proTable.perPage", { size: opt })}
            </option>
          ))}
        </select>

        <div className="flex items-center gap-1">
          <Button
            size="sm"
            variant="outline"
            disabled={!canPrev}
            onClick={() => onPageChange(page - 1)}
            aria-label={t("proTable.prevPage")}
            className="h-7 w-7 p-0"
          >
            <ChevronLeft className="h-3.5 w-3.5" />
          </Button>

          {pages.map((p) => (
            <Button
              key={p}
              size="sm"
              variant={p === page ? "primary" : "outline"}
              onClick={() => onPageChange(p)}
              className={`h-7 min-w-[1.75rem] px-1.5 text-xs ${
                p === page ? "font-semibold" : ""
              }`}
            >
              {p}
            </Button>
          ))}

          <Button
            size="sm"
            variant="outline"
            disabled={!canNext}
            onClick={() => onPageChange(page + 1)}
            aria-label={t("proTable.nextPage")}
            className="h-7 w-7 p-0"
          >
            <ChevronRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </div>
  );
}
