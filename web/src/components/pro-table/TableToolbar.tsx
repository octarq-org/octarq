import { ReactNode, useState } from "react";
import {
  ChevronDown,
  ChevronUp,
  Columns3,
  RotateCcw,
  Search,
  SlidersHorizontal,
} from "lucide-react";
import { Table, VisibilityState } from "@tanstack/react-table";
import { Button, TableDensity } from "../../ui";
import { useTranslation } from "../../i18n";
import {
  ProColumn,
  ProTableActionRef,
  ProTableSearchConfig,
} from "./types";

export interface TableToolbarProps<TData> {
  columns: ProColumn<TData>[];
  table: Table<TData>;
  headerTitle?: ReactNode;
  tooltip?: ReactNode;
  toolBarRender?: (action?: ProTableActionRef) => ReactNode[];
  searchConfig?: boolean | ProTableSearchConfig;
  searchParams: Record<string, any>;
  onSearch: (params: Record<string, any>) => void;
  onReset: () => void;
  onReload: () => void;
  isFetching?: boolean;
  density: TableDensity;
  onDensityChange: (density: TableDensity) => void;
  columnVisibility: VisibilityState;
  onColumnVisibilityChange: (colId: string, visible: boolean) => void;
  onResetColumnVisibility: () => void;
  actionRef?: ProTableActionRef;
}

export function TableToolbar<TData>({
  columns,
  table,
  headerTitle,
  tooltip,
  toolBarRender,
  searchConfig = true,
  searchParams,
  onSearch,
  onReset,
  onReload,
  isFetching = false,
  density,
  onDensityChange,
  columnVisibility,
  onColumnVisibilityChange,
  onResetColumnVisibility,
  actionRef,
}: TableToolbarProps<TData>) {
  const { t } = useTranslation();

  // ─── Search Form State ──────────────────────────────────────────────────────
  const searchCfg = typeof searchConfig === "object" ? searchConfig : {};
  const defaultCollapsedCount = searchCfg.defaultCollapsedCount ?? 2;
  const [collapsed, setCollapsed] = useState<boolean>(
    searchCfg.defaultCollapsed ?? false
  );
  const [formState, setFormState] = useState<Record<string, any>>(searchParams);

  // Filterable columns
  const filterColumns = columns.filter((c) => {
    if (c.hideInSearch) return false;
    const key = c.key || (c.dataIndex ? String(c.dataIndex) : undefined);
    return !!key;
  });

  const hasSearch = searchConfig !== false && filterColumns.length > 0;
  const showExpandButton = filterColumns.length > defaultCollapsedCount;
  const visibleFilterColumns =
    hasSearch && collapsed
      ? filterColumns.slice(0, defaultCollapsedCount)
      : filterColumns;

  // ─── Column Visibility Dropdown State ───────────────────────────────────────
  const [showColumnDropdown, setShowColumnDropdown] = useState(false);
  const [showDensityDropdown, setShowDensityDropdown] = useState(false);

  const handleFieldChange = (key: string, value: any) => {
    setFormState((prev) => ({
      ...prev,
      [key]: value,
    }));
  };

  const handleSubmit = (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    onSearch(formState);
  };

  const handleReset = () => {
    const emptyState: Record<string, any> = {};
    filterColumns.forEach((col) => {
      const key = col.key || (col.dataIndex ? String(col.dataIndex) : undefined);
      if (key) {
        emptyState[key] = col.initialValue ?? "";
      }
    });
    setFormState(emptyState);
    onReset();
  };

  return (
    <div className="space-y-3 p-4">
      {/* Search / Filter Section */}
      {hasSearch && (
        <form
          onSubmit={handleSubmit}
          className="rounded-xl border border-foreground/[0.06] bg-foreground/[0.02] p-3 text-xs"
        >
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {visibleFilterColumns.map((col) => {
              const fieldKey =
                col.key || (col.dataIndex ? String(col.dataIndex) : "");
              const label =
                typeof col.title === "string"
                  ? col.title
                  : typeof col.header === "string"
                    ? col.header
                    : fieldKey;
              const valueType = col.valueType || (col.valueEnum ? "select" : "text");
              const currentValue = formState[fieldKey] ?? "";

              return (
                <div key={fieldKey} className="flex flex-col gap-1">
                  <label
                    htmlFor={`pro-search-${fieldKey}`}
                    className="font-medium text-foreground/75"
                  >
                    {label}
                  </label>

                  {valueType === "select" && col.valueEnum ? (
                    <select
                      id={`pro-search-${fieldKey}`}
                      value={currentValue}
                      onChange={(e) => handleFieldChange(fieldKey, e.target.value)}
                      className="rounded-lg border border-border bg-card/60 px-2.5 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                    >
                      <option value="">{t("proTable.all")}</option>
                      {Object.entries(col.valueEnum).map(([enumKey, enumVal]) => {
                        const enumText =
                          typeof enumVal === "object" &&
                          enumVal !== null &&
                          "text" in enumVal
                            ? (enumVal as any).text
                            : String(enumVal);
                        return (
                          <option key={enumKey} value={enumKey}>
                            {enumText}
                          </option>
                        );
                      })}
                    </select>
                  ) : (
                    <input
                      id={`pro-search-${fieldKey}`}
                      type={
                        valueType === "digit"
                          ? "number"
                          : valueType === "date"
                            ? "date"
                            : "text"
                      }
                      value={currentValue}
                      onChange={(e) => handleFieldChange(fieldKey, e.target.value)}
                      placeholder={col.fieldProps?.placeholder || t("proTable.inputPlaceholder", { name: label })}
                      className="rounded-lg border border-border bg-card/60 px-2.5 py-1.5 text-xs text-foreground placeholder:text-foreground/40 focus:outline-none focus:ring-1 focus:ring-primary"
                    />
                  )}
                </div>
              );
            })}
          </div>

          <div className="mt-3 flex items-center justify-between border-t border-foreground/[0.04] pt-2.5">
            <div>
              {showExpandButton && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setCollapsed(!collapsed)}
                  className="gap-1 text-xs text-foreground/60 hover:text-foreground"
                >
                  {collapsed ? (
                    <>
                      <span>{t("proTable.expand")}</span>
                      <ChevronDown className="h-3 w-3" />
                    </>
                  ) : (
                    <>
                      <span>{t("proTable.collapse")}</span>
                      <ChevronUp className="h-3 w-3" />
                    </>
                  )}
                </Button>
              )}
            </div>

            <div className="flex items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={handleReset}
              >
                {searchCfg.resetText ?? t("proTable.reset")}
              </Button>
              <Button
                type="submit"
                size="sm"
                variant="primary"
                className="gap-1.5"
              >
                <Search className="h-3 w-3" />
                <span>{searchCfg.searchText ?? t("proTable.search")}</span>
              </Button>
            </div>
          </div>
        </form>
      )}

      {/* Main Toolbar Action Bar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        {/* Left Slot: Title & Tooltip */}
        <div className="flex items-center gap-2">
          {headerTitle && (
            <h3 className="text-base font-semibold text-foreground">
              {headerTitle}
            </h3>
          )}
          {tooltip && (
            <span
              className="text-xs text-foreground/50"
              title={typeof tooltip === "string" ? tooltip : undefined}
            >
              {tooltip}
            </span>
          )}
        </div>

        {/* Right Slot: Actions, Refresh, Density, Columns */}
        <div className="flex items-center gap-2">
          {toolBarRender && (
            <div className="flex items-center gap-2">
              {toolBarRender(actionRef)}
            </div>
          )}

          {/* Reload / Refresh Button */}
          <Button
            size="sm"
            variant="outline"
            onClick={onReload}
            aria-label={t("proTable.reload")}
            className="h-8 w-8 p-0"
            title={t("proTable.reload")}
          >
            <RotateCcw
              className={`h-3.5 w-3.5 ${isFetching ? "animate-spin" : ""}`}
            />
          </Button>

          {/* Table Density Selector */}
          <div className="relative">
            <Button
              size="sm"
              variant="outline"
              onClick={() => setShowDensityDropdown((prev) => !prev)}
              aria-label={t("proTable.density")}
              className="h-8 w-8 p-0"
              title={t("proTable.density")}
            >
              <SlidersHorizontal className="h-3.5 w-3.5" />
            </Button>

            {showDensityDropdown && (
              <div className="absolute right-0 top-9 z-50 w-32 rounded-xl border border-border bg-card/95 p-1 shadow-lg backdrop-blur-md">
                <button
                  type="button"
                  onClick={() => {
                    onDensityChange("comfortable");
                    setShowDensityDropdown(false);
                  }}
                  className={`flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-left text-xs transition-colors ${
                    density === "comfortable"
                      ? "bg-primary text-primary-foreground font-medium"
                      : "text-foreground hover:bg-surface-hover"
                  }`}
                >
                  <span>{t("proTable.densityComfortable")}</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    onDensityChange("compact");
                    setShowDensityDropdown(false);
                  }}
                  className={`mt-1 flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-left text-xs transition-colors ${
                    density === "compact"
                      ? "bg-primary text-primary-foreground font-medium"
                      : "text-foreground hover:bg-surface-hover"
                  }`}
                >
                  <span>{t("proTable.densityCompact")}</span>
                </button>
              </div>
            )}
          </div>

          {/* Column Visibility Selector */}
          <div className="relative">
            <Button
              size="sm"
              variant="outline"
              onClick={() => setShowColumnDropdown((prev) => !prev)}
              aria-label={t("proTable.columns")}
              className="h-8 w-8 p-0"
              title={t("proTable.columns")}
            >
              <Columns3 className="h-3.5 w-3.5" />
            </Button>

            {showColumnDropdown && (
              <div className="absolute right-0 top-9 z-50 w-48 rounded-xl border border-border bg-card/95 p-2 shadow-lg backdrop-blur-md">
                <div className="mb-2 flex items-center justify-between border-b border-foreground/[0.06] pb-1.5 px-1">
                  <span className="text-xs font-semibold text-foreground/80">
                    {t("proTable.columns")}
                  </span>
                  <button
                    type="button"
                    onClick={onResetColumnVisibility}
                    className="text-[11px] text-primary hover:underline cursor-pointer"
                  >
                    {t("proTable.resetColumns")}
                  </button>
                </div>
                <div className="max-h-56 space-y-1 overflow-y-auto pr-1">
                  {columns
                    .filter((c) => !c.hideInTable)
                    .map((col) => {
                      const colId =
                        col.key || (col.dataIndex ? String(col.dataIndex) : "");
                      const title =
                        typeof col.title === "string"
                          ? col.title
                          : typeof col.header === "string"
                            ? col.header
                            : colId;
                      const isVisible = columnVisibility[colId] !== false;

                      return (
                        <label
                          key={colId}
                          className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-xs text-foreground/80 hover:bg-surface-hover"
                        >
                          <input
                            type="checkbox"
                            checked={isVisible}
                            onChange={(e) =>
                              onColumnVisibilityChange(colId, e.target.checked)
                            }
                            className="rounded border-border text-primary focus:ring-primary"
                          />
                          <span className="truncate">{title}</span>
                        </label>
                      );
                    })}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
