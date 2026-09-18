// The primitives sub-barrel: the pure, app-independent shared components.
export { GlassCard } from "./glass-card";
export { Panel } from "./panel";
export { Badge, badgeVariants, type BadgeTone, type BadgeShape, type BadgeProps } from "./badge";
export { Button, buttonVariants, type ButtonVariant } from "./button";
export { ProPill, TIER_LABEL } from "./pro-pill";
export { StatCard } from "./stat-card";
export { PageHeader } from "./page-header";
export { ScreenWrap } from "./screen-wrap";
export { Modal } from "./modal";
export { Field } from "./field";
export { Empty } from "./empty";
export { Toggle } from "./toggle";

// Form / data components
export { Input, fieldClass } from "./input";
export { Textarea } from "./textarea";
export { Select, type SelectOption } from "./select";
export { Tabs, type TabItem } from "./tabs";
export { Tooltip } from "./tooltip";
export { Table, THead, TBody, TR, TH, TD, TableDensityProvider, useTableDensity, useSetTableDensity, type TableDensity } from "./table";
export { Skeleton } from "./skeleton";
export { TableEmpty, type TableEmptyProps } from "./table-empty";
export { TableError, type TableErrorProps } from "./table-error";
export { TableSkeleton, type TableSkeletonProps } from "./table-skeleton";
export {
  TablePagination,
  type TablePaginationProps,
  type TablePaginationConfig,
} from "./table-pagination";

export { Alert, alertVariants, type AlertProps } from "./alert";
export { FormError, formErrorMessage, formErrorStatusKeys } from "./form-error";
export { RouteFallback } from "./route-fallback";

export { Code } from "./code";
export { Guide } from "./guide";
export { timeAgo } from "./time";
