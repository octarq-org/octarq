import { forwardRef } from "react";
import { cn } from "../cn";
import { useTranslation } from "../../i18n";
import { Button } from "./button";

// Pagination bar for a table. Moved here from the host app's pro-table; it
// depends on nothing tanstack — only the fields it actually reads are part of
// the config type, so it carries none of that grid's weight.
export interface TablePaginationConfig {
  pageSizeOptions?: number[];
  hideOnSinglePage?: boolean;
}

export interface TablePaginationProps {
  page: number;
  pageSize: number;
  total: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  config?: boolean | TablePaginationConfig;
  className?: string;
}

const MAX_PAGE_BUTTONS = 5;

export const TablePagination = forwardRef<HTMLDivElement, TablePaginationProps>(
  (
    { page, pageSize, total, pageCount, onPageChange, onPageSizeChange, config = true, className },
    ref,
  ) => {
  const { t } = useTranslation();

  if (config === false) return null;

  const paginationConfig = typeof config === "object" ? config : {};
  const pageSizeOptions = paginationConfig.pageSizeOptions ?? [10, 20, 50, 100];
  const hideOnSinglePage = paginationConfig.hideOnSinglePage ?? false;

  if (hideOnSinglePage && total <= pageSize) return null;

  const canPrev = page > 1;
  const canNext = page < pageCount;

  // A sliding window of page numbers, kept within bounds at both ends.
  const pages: number[] = [];
  let start = Math.max(1, page - Math.floor(MAX_PAGE_BUTTONS / 2));
  const end = Math.min(pageCount, start + MAX_PAGE_BUTTONS - 1);
  if (end - start + 1 < MAX_PAGE_BUTTONS) start = Math.max(1, end - MAX_PAGE_BUTTONS + 1);
  for (let i = start; i <= end; i++) pages.push(i);

  return (
    <div
      ref={ref}
      className={cn(
        "flex flex-wrap items-center justify-between gap-4 border-t border-foreground/[0.06] px-4 py-3 text-xs text-foreground/70",
        className,
      )}
    >
      <div className="flex items-center gap-3">
        <span className="font-medium">{t("proTable.totalItems", { total })}</span>
        <span className="text-foreground/40">|</span>
        <span>{t("proTable.pageOf", { page, totalPages: Math.max(1, pageCount) })}</span>
      </div>

      <div className="flex items-center gap-2">
        <select
          value={pageSize}
          onChange={(e) => {
            onPageSizeChange(Number(e.target.value));
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
            <ChevronLeftGlyph className="h-3.5 w-3.5" />
          </Button>

          {pages.map((p) => (
            <Button
              key={p}
              size="sm"
              variant={p === page ? "primary" : "outline"}
              onClick={() => onPageChange(p)}
              className={`h-7 min-w-[1.75rem] px-1.5 text-xs ${p === page ? "font-semibold" : ""}`}
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
            <ChevronRightGlyph className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </div>
    );
  },
);
TablePagination.displayName = "TablePagination";

// Inline so the package stays free of an icon-library dependency.
function ChevronLeftGlyph({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}

function ChevronRightGlyph({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}
