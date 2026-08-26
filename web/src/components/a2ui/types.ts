/**
 * Invariant #11: P3 pre-API volatility — A2UI kind and payload fields are internal contracts,
 * subject to evolution as AI tool specifications finalize (VOLATILE).
 */

export interface ChartSeries {
  name?: string;
  values: number[];
}

export interface ChartData {
  labels: string[];
  series: ChartSeries[];
}

export interface ChartWidget {
  kind: "chart";
  title?: string;
  data: ChartData;
}

export interface DiffWidget {
  kind: "diff";
  title?: string;
  before: string;
  after: string;
}

export interface ApprovalWidget {
  kind: "approval";
  title?: string;
  tool: string;
  args?: Record<string, unknown>;
}

export interface UnknownWidget {
  kind: string;
  title?: string;
  [key: string]: unknown;
}

export type A2UIWidget = ChartWidget | DiffWidget | ApprovalWidget | UnknownWidget;
