import { ReactNode } from "react";
import {
  ColumnDef,
  SortingState,
  VisibilityState,
  Table,
} from "@tanstack/react-table";
import { z } from "zod";
import { TableDensity } from "../../ui";

export type ProTableValueType = "text" | "select" | "digit" | "date";

export interface ValueEnumItem {
  text: ReactNode;
  status?: string;
  disabled?: boolean;
}

export type ValueEnumType = Record<string, ValueEnumItem | ReactNode>;

export type ProColumn<TData = any, TValue = any> = Omit<ColumnDef<TData, TValue>, "header"> & {
  // Key / Field identification
  dataIndex?: keyof TData | string;
  key?: string;

  // Header / Title display
  title?: ReactNode;
  header?: ColumnDef<TData, TValue>["header"];
  tooltip?: ReactNode;

  // Search form configuration
  hideInSearch?: boolean;
  valueType?: ProTableValueType;
  valueEnum?: ValueEnumType;
  initialValue?: any;
  fieldProps?: Record<string, any>;

  // Table display configuration
  hideInTable?: boolean;
  sorter?: boolean | ((a: TData, b: TData) => number);
  width?: number | string;
  minWidth?: number | string;
  maxWidth?: number | string;
  ellipsis?: boolean;
  copyable?: boolean;

  // Custom render
  render?: (
    dom: ReactNode,
    record: TData,
    index: number,
    action?: ProTableActionRef
  ) => ReactNode;
};

export interface ProTablePaginationConfig {
  current?: number;
  pageSize?: number;
  defaultCurrent?: number;
  defaultPageSize?: number;
  pageSizeOptions?: number[];
  showSizeChanger?: boolean;
  showTotal?: boolean | ((total: number, range: [number, number]) => ReactNode);
  hideOnSinglePage?: boolean;
}

export interface ProTableSearchConfig {
  collapsed?: boolean;
  defaultCollapsed?: boolean;
  defaultCollapsedCount?: number;
  searchText?: ReactNode;
  resetText?: ReactNode;
  collapseText?: ReactNode;
  expandText?: ReactNode;
  filterOption?: boolean;
  span?: number;
}

export interface ProTableRequestParams {
  page: number;
  pageSize: number;
  sorter?: { field: string; order: "asc" | "desc" } | null;
  filters?: Record<string, any>;
  [key: string]: any;
}

export interface ProTableRequestResult<TData> {
  data: TData[];
  total?: number;
  success?: boolean;
  [key: string]: any;
}

export type ProTableRequest<TData, TParams = Record<string, any>> = (
  params: ProTableRequestParams & TParams,
  sort: Record<string, "asc" | "desc">,
  filter: Record<string, any>
) => Promise<ProTableRequestResult<TData> | TData[]>;

export interface ProTableActionRef {
  reload: (resetPageIndex?: boolean) => Promise<any> | void;
  reset: () => void;
  setPage: (page: number) => void;
  setPageSize: (pageSize: number) => void;
  clearFilters: () => void;
  getState: () => {
    page: number;
    pageSize: number;
    sorter: { field: string; order: "asc" | "desc" } | null;
    filters: Record<string, any>;
    searchParams: Record<string, any>;
  };
}

export interface UseProTableOptions<TData, TParams extends Record<string, any> = Record<string, any>> {
  columns: ProColumn<TData>[];
  request?: ProTableRequest<TData, TParams>;
  dataSource?: TData[];
  rowKey?: keyof TData | ((record: TData) => string | number);
  params?: TParams;
  pagination?: boolean | ProTablePaginationConfig;
  search?: boolean | ProTableSearchConfig;
  queryKeyPrefix?: string;
  schema?: z.ZodType<TData>;
  defaultDataFallback?: TData;
  defaultDensity?: TableDensity;
  polling?: number;
  actionRef?: React.MutableRefObject<ProTableActionRef | undefined> | ((ref: ProTableActionRef) => void);
  onLoadingChange?: (loading: boolean) => void;
}

export interface UseProTableReturn<TData, TParams extends Record<string, any> = Record<string, any>> {
  table: Table<TData>;
  data: TData[];
  total: number;
  page: number;
  pageSize: number;
  pageCount: number;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  sorting: SortingState;
  setSorting: (sorting: SortingState | ((old: SortingState) => SortingState)) => void;
  sorter: { field: string; order: "asc" | "desc" } | null;
  searchParams: Record<string, any>;
  setSearchParams: (params: Record<string, any> | ((old: Record<string, any>) => Record<string, any>)) => void;
  resetSearch: () => void;
  columnVisibility: VisibilityState;
  setColumnVisibility: (visibility: VisibilityState | ((old: VisibilityState) => VisibilityState)) => void;
  resetColumnVisibility: () => void;
  density: TableDensity;
  setDensity: (density: TableDensity) => void;
  isLoading: boolean;
  isFetching: boolean;
  isError: boolean;
  error: unknown;
  reload: (resetPageIndex?: boolean) => Promise<any> | void;
  reset: () => void;
  actionRef: ProTableActionRef;
}

export interface ProTableProps<TData, TParams extends Record<string, any> = Record<string, any>>
  extends UseProTableOptions<TData, TParams> {
  headerTitle?: ReactNode;
  tooltip?: ReactNode;
  toolBarRender?: (action?: ProTableActionRef) => ReactNode[];
  tableExtraRender?: (table: Table<TData>) => ReactNode;
  className?: string;
  tableClassName?: string;
  cardClassName?: string;
  emptyText?: ReactNode;
  emptyReason?: ReactNode;
  errorText?: ReactNode;
  showToolbar?: boolean;
  ariaLabel?: string;
}
