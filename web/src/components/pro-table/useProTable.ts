import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  useReactTable,
  getCoreRowModel,
  SortingState,
  VisibilityState,
} from "@tanstack/react-table";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  UseProTableOptions,
  UseProTableReturn,
  ProTableActionRef,
} from "./types";
import { cleanParams, normalizeColumns, parseArrayWithFallback } from "./utils";
import { TableDensity } from "../../ui";

export function useProTable<
  TData,
  TParams extends Record<string, any> = Record<string, any>
>(options: UseProTableOptions<TData, TParams>): UseProTableReturn<TData, TParams> {
  const {
    columns,
    request,
    dataSource,
    params,
    pagination = true,
    queryKeyPrefix = "proTable",
    schema,
    defaultDataFallback,
    defaultDensity = "comfortable",
    polling,
    actionRef: externalActionRef,
    onLoadingChange,
  } = options;

  // ─── Pagination State ────────────────────────────────────────────────────────
  const paginationConfig = typeof pagination === "object" ? pagination : {};
  const [page, setPage] = useState<number>(
    paginationConfig.current ?? paginationConfig.defaultCurrent ?? 1
  );
  const [pageSize, setPageSize] = useState<number>(
    paginationConfig.pageSize ?? paginationConfig.defaultPageSize ?? 10
  );

  // ─── Sorting State ──────────────────────────────────────────────────────────
  const [sorting, setSorting] = useState<SortingState>([]);
  const sorter = useMemo(() => {
    if (sorting.length === 0) return null;
    const first = sorting[0];
    return {
      field: first.id,
      order: (first.desc ? "desc" : "asc") as "asc" | "desc",
    };
  }, [sorting]);

  // ─── Search / Filter State ──────────────────────────────────────────────────
  const initialSearchParams = useMemo(() => {
    const init: Record<string, any> = {};
    for (const col of columns) {
      if (!col.hideInSearch && col.initialValue !== undefined) {
        const key = col.key || (col.dataIndex ? String(col.dataIndex) : undefined);
        if (key) {
          init[key] = col.initialValue;
        }
      }
    }
    return init;
  }, [columns]);

  const [searchParams, setSearchParams] = useState<Record<string, any>>(initialSearchParams);

  const resetSearch = useCallback(() => {
    setSearchParams(initialSearchParams);
    setPage(1);
  }, [initialSearchParams]);

  // ─── Column Visibility State ────────────────────────────────────────────────
  const initialVisibility = useMemo(() => {
    const vis: VisibilityState = {};
    for (const col of columns) {
      const colId = col.key || (col.dataIndex ? String(col.dataIndex) : undefined);
      if (colId && col.hideInTable) {
        vis[colId] = false;
      }
    }
    return vis;
  }, [columns]);

  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(initialVisibility);

  const resetColumnVisibility = useCallback(() => {
    setColumnVisibility(initialVisibility);
  }, [initialVisibility]);

  // ─── Density State ──────────────────────────────────────────────────────────
  const [density, setDensity] = useState<TableDensity>(defaultDensity);

  // ─── TanStack Query Integration ─────────────────────────────────────────────
  const cleanedSearch = useMemo(() => cleanParams(searchParams), [searchParams]);
  const cleanedExtraParams = useMemo(() => cleanParams(params || {}), [params]);

  const queryKey = useMemo(
    () => [
      queryKeyPrefix,
      page,
      pageSize,
      sorter,
      cleanedSearch,
      cleanedExtraParams,
    ],
    [queryKeyPrefix, page, pageSize, sorter, cleanedSearch, cleanedExtraParams]
  );

  const queryFn = useCallback(async () => {
    if (!request) {
      return { data: dataSource || [], total: (dataSource || []).length };
    }

    const sortMap: Record<string, "asc" | "desc"> = {};
    if (sorter) {
      sortMap[sorter.field] = sorter.order;
    }

    const requestParams = {
      page,
      pageSize,
      sorter,
      filters: cleanedSearch,
      ...cleanedSearch,
      ...cleanedExtraParams,
    } as any;

    const res = await request(requestParams, sortMap, cleanedSearch);

    let rawData: TData[] = [];
    let totalCount = 0;

    if (Array.isArray(res)) {
      rawData = res;
      totalCount = res.length;
    } else if (res && typeof res === "object") {
      rawData = res.data || [];
      totalCount = res.total ?? rawData.length;
    }

    // Boundary type safety via Zod schema + parseWithFallback
    const validData = schema
      ? parseArrayWithFallback(schema, rawData, defaultDataFallback)
      : rawData;

    return {
      data: validData,
      total: totalCount,
    };
  }, [
    request,
    dataSource,
    page,
    pageSize,
    sorter,
    cleanedSearch,
    cleanedExtraParams,
    schema,
    defaultDataFallback,
  ]);

  const {
    data: queryResponse,
    isLoading: isQueryLoading,
    isFetching,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey,
    queryFn,
    enabled: !!request || dataSource !== undefined,
    refetchInterval: polling,
    staleTime: 1000 * 30,
  });

  const isLoading = request ? isQueryLoading : false;

  useEffect(() => {
    onLoadingChange?.(isLoading || isFetching);
  }, [isLoading, isFetching, onLoadingChange]);

  const data = useMemo(() => {
    if (dataSource && !request) {
      // Client-side pagination if datasource provided
      if (pagination === false) return dataSource;
      const start = (page - 1) * pageSize;
      return dataSource.slice(start, start + pageSize);
    }
    return queryResponse?.data || [];
  }, [dataSource, request, pagination, page, pageSize, queryResponse?.data]);

  const total = useMemo(() => {
    if (dataSource && !request) return dataSource.length;
    return queryResponse?.total ?? data.length;
  }, [dataSource, request, queryResponse?.total, data.length]);

  const pageCount = useMemo(() => {
    if (pagination === false) return 1;
    return Math.max(1, Math.ceil(total / pageSize));
  }, [pagination, total, pageSize]);

  // ─── ActionRef Handle ───────────────────────────────────────────────────────
  const reload = useCallback(
    async (resetPageIndex?: boolean) => {
      if (resetPageIndex) {
        setPage(1);
      }
      return refetch();
    },
    [refetch]
  );

  const reset = useCallback(() => {
    resetSearch();
    setSorting([]);
    refetch();
  }, [resetSearch, refetch]);

  const actionRefObj: ProTableActionRef = useMemo(
    () => ({
      reload,
      reset,
      setPage,
      setPageSize,
      clearFilters: () => setSearchParams({}),
      getState: () => ({
        page,
        pageSize,
        sorter,
        filters: cleanedSearch,
        searchParams,
      }),
    }),
    [reload, reset, page, pageSize, sorter, cleanedSearch, searchParams]
  );

  useEffect(() => {
    if (externalActionRef) {
      if (typeof externalActionRef === "function") {
        externalActionRef(actionRefObj);
      } else {
        externalActionRef.current = actionRefObj;
      }
    }
  }, [externalActionRef, actionRefObj]);

  // ─── TanStack Table Instance ────────────────────────────────────────────────
  const normalizedColumns = useMemo(
    () => normalizeColumns(columns, actionRefObj),
    [columns, actionRefObj]
  );

  const table = useReactTable({
    data,
    columns: normalizedColumns,
    pageCount,
    state: {
      sorting,
      pagination: {
        pageIndex: page - 1,
        pageSize,
      },
      columnVisibility,
    },
    onSortingChange: setSorting,
    onColumnVisibilityChange: setColumnVisibility,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
  });

  return {
    table,
    data,
    total,
    page,
    pageSize,
    pageCount,
    setPage,
    setPageSize,
    sorting,
    setSorting,
    sorter,
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
    reset,
    actionRef: actionRefObj,
  };
}
