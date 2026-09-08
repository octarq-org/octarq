import { ReactNode, useMemo } from "react";
import {
  flexRender,
  Header,
} from "@tanstack/react-table";
import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import {
  GlassCard,
  Table,
  THead,
  TBody,
  TR,
  TH,
  TD,
  TableDensityProvider,
} from "../../ui";
import { ProTableProps } from "./types";
import { useProTable } from "./useProTable";
import { TableToolbar } from "./TableToolbar";
import { TablePagination } from "./TablePagination";
import { TableSkeleton } from "./TableSkeleton";
import { TableEmpty } from "./TableEmpty";
import { TableError } from "./TableError";

// Safe fallback QueryClient for standalone tests or components rendered outside a provider
const fallbackQueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      staleTime: 1000 * 30,
    },
  },
});

function ProTableInner<
  TData,
  TParams extends Record<string, any> = Record<string, any>
>(props: ProTableProps<TData, TParams>) {
  const {
    columns,
    headerTitle,
    tooltip,
    toolBarRender,
    tableExtraRender,
    className = "",
    tableClassName = "",
    cardClassName = "",
    emptyText,
    emptyReason,
    errorText,
    showToolbar = true,
    rowKey = "id",
    pagination = true,
    search = true,
    ariaLabel,
  } = props;

  const {
    table,
    data,
    total,
    page,
    pageSize,
    pageCount,
    setPage,
    setPageSize,
    searchParams,
    setSearchParams,
    resetSearch,
    columnVisibility,
    setColumnVisibility,
    resetColumnVisibility,
    density,
    setDensity,
    isLoading,
    isFetching,
    isError,
    error,
    reload,
    actionRef,
  } = useProTable(props);

  const visibleColumnsCount = useMemo(
    () => table.getVisibleLeafColumns().length,
    [table]
  );

  const renderSortIndicator = (header: Header<TData, unknown>) => {
    if (!header.column.getCanSort()) return null;
    const sorted = header.column.getIsSorted();
    if (sorted === "asc") {
      return <ArrowUp className="inline-block h-3.5 w-3.5 text-primary" />;
    }
    if (sorted === "desc") {
      return <ArrowDown className="inline-block h-3.5 w-3.5 text-primary" />;
    }
    return (
      <ArrowUpDown className="inline-block h-3.5 w-3.5 text-foreground/30 opacity-0 group-hover:opacity-100 transition-opacity" />
    );
  };

  return (
    <GlassCard className={`overflow-hidden p-0 ${cardClassName} ${className}`.trim()}>
      {/* Toolbar / Search form */}
      {showToolbar && (
        <TableToolbar
          columns={columns}
          table={table}
          headerTitle={headerTitle}
          tooltip={tooltip}
          toolBarRender={toolBarRender}
          searchConfig={search}
          searchParams={searchParams}
          onSearch={(params) => {
            setSearchParams(params);
            setPage(1);
          }}
          onReset={resetSearch}
          onReload={() => reload(false)}
          isFetching={isFetching}
          density={density}
          onDensityChange={setDensity}
          columnVisibility={columnVisibility}
          onColumnVisibilityChange={(colId, visible) => {
            setColumnVisibility((prev) => ({
              ...prev,
              [colId]: visible,
            }));
          }}
          onResetColumnVisibility={resetColumnVisibility}
          actionRef={actionRef}
        />
      )}

      {/* Extra render slot */}
      {tableExtraRender && (
        <div className="border-t border-foreground/[0.06] px-4 py-2">
          {tableExtraRender(table)}
        </div>
      )}

      {/* Table Content with Density Provider */}
      <TableDensityProvider density={density} onDensityChange={setDensity}>
        {isLoading ? (
          <TableSkeleton columnsCount={visibleColumnsCount} ariaLabel={ariaLabel} />
        ) : isError ? (
          <TableError
            error={error}
            errorText={errorText}
            onRetry={() => reload(false)}
          />
        ) : data.length === 0 ? (
          <TableEmpty emptyText={emptyText} emptyReason={emptyReason} />
        ) : (
          <Table className={tableClassName}>
            <THead className="border-b border-foreground/[0.06] bg-foreground/[0.02]">
              {table.getHeaderGroups().map((headerGroup) => (
                <TR key={headerGroup.id}>
                  {headerGroup.headers.map((header) => {
                    const canSort = header.column.getCanSort();
                    return (
                      <TH
                        key={header.id}
                        style={{
                          width: header.getSize() !== 150 ? header.getSize() : undefined,
                        }}
                        className={canSort ? "group cursor-pointer select-none" : ""}
                        onClick={
                          canSort
                            ? header.column.getToggleSortingHandler()
                            : undefined
                        }
                      >
                        <div className="flex items-center gap-1.5">
                          <span>
                            {header.isPlaceholder
                              ? null
                              : flexRender(
                                  header.column.columnDef.header,
                                  header.getContext()
                                )}
                          </span>
                          {renderSortIndicator(header)}
                        </div>
                      </TH>
                    );
                  })}
                </TR>
              ))}
            </THead>
            <TBody>
              {table.getRowModel().rows.map((row) => {
                const key =
                  typeof rowKey === "function"
                    ? rowKey(row.original)
                    : (row.original as any)?.[rowKey] ?? row.id;

                return (
                  <TR key={key}>
                    {row.getVisibleCells().map((cell) => (
                      <TD
                        key={cell.id}
                        style={{
                          width: cell.column.getSize() !== 150 ? cell.column.getSize() : undefined,
                        }}
                      >
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext()
                        )}
                      </TD>
                    ))}
                  </TR>
                );
              })}
            </TBody>
          </Table>
        )}

        {/* Pagination Section */}
        {!isLoading && !isError && data.length > 0 && pagination !== false && (
          <TablePagination
            page={page}
            pageSize={pageSize}
            total={total}
            pageCount={pageCount}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
            config={pagination}
          />
        )}
      </TableDensityProvider>
    </GlassCard>
  );
}

export function ProTable<
  TData,
  TParams extends Record<string, any> = Record<string, any>
>(props: ProTableProps<TData, TParams>) {
  let hasClient = false;
  try {
    hasClient = !!useQueryClient();
  } catch {
    hasClient = false;
  }

  if (!hasClient) {
    return (
      <QueryClientProvider client={fallbackQueryClient}>
        <ProTableInner {...props} />
      </QueryClientProvider>
    );
  }

  return <ProTableInner {...props} />;
}
