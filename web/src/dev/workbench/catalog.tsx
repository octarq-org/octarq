import { type ReactNode } from "react";
import {
  Code,
  Empty,
  Field,
  FormError,
  GlassCard,
  Guide,
  Input,
  PageHeader,
  Panel,
  ProPill,
  RouteFallback,
  ScreenWrap,
  Skeleton,
  StatCard,
  TableEmpty,
  TableError,
  TableSkeleton,
  Tabs,
  Textarea,
  Tooltip,
  Button,
} from "@octarq/plugin-sdk";
import {
  AlertRows,
  BadgeRows,
  ButtonVariants,
  DialogRow,
  ModalRow,
  SelectRow,
  SwitchRow,
  TablePaginationPreview,
  TablePreview,
} from "./catalogPreviews";

// The workbench catalog: one entry per SDK UI component, each rendering the REAL
// component imported from the SDK — never a copy, never a mock. That is the
// point of the workbench: a preview that re-implements the component drifts from
// it and then lies about what ships.
//
// `covers` names the SDK exports the entry is responsible for. Most are 1:1;
// Table folds in its row/cell parts, which have no standalone meaning. The
// coverage test (catalog.coverage.test.ts) fails if an exported component has no
// entry, so adding a component to the SDK without previewing it breaks the build.
//
// `Component` is rendered as <entry.Component />, not called as a function, so
// each entry gets its own hook/error boundary — a preview that uses useState
// cannot corrupt the workbench's own hook order when you switch entries.
export type CatalogGroup = "layout" | "action" | "form" | "data" | "feedback" | "overlay";

export interface CatalogEntry {
  name: string;
  group: CatalogGroup;
  covers: string[];
  copy: string[];
  Component: () => ReactNode;
}

export const CATALOG_GROUPS: CatalogGroup[] = [
  "layout",
  "action",
  "form",
  "data",
  "feedback",
  "overlay",
];

export const CATALOG: CatalogEntry[] = [
  {
    name: "GlassCard",
    group: "layout",
    covers: ["GlassCard"],
    copy: ["Panel heading", "Body copy inside a card."],
    Component: () => (
      <GlassCard className="p-4">
        <h3 className="text-sm font-semibold">Panel heading</h3>
        <p className="mt-1 text-sm text-muted-foreground">Body copy inside a card.</p>
      </GlassCard>
    ),
  },
  {
    name: "Panel",
    group: "layout",
    covers: ["Panel"],
    copy: ["Section title", "Supporting line."],
    Component: () => (
      <Panel title="Section title">
        <p className="text-sm text-muted-foreground">Supporting line.</p>
      </Panel>
    ),
  },
  {
    name: "ScreenWrap",
    group: "layout",
    covers: ["ScreenWrap"],
    copy: [],
    Component: () => (
      <ScreenWrap>
        <div className="rounded-lg border border-dashed border-border p-6 text-sm text-muted-foreground">
          Page content is inset by ScreenWrap.
        </div>
      </ScreenWrap>
    ),
  },
  {
    name: "PageHeader",
    group: "layout",
    covers: ["PageHeader"],
    copy: ["Page title", "One line describing the page."],
    Component: () => <PageHeader title="Page title" description="One line describing the page." />,
  },
  {
    name: "StatCard",
    group: "layout",
    covers: ["StatCard"],
    copy: ["Clicks", "1,284"],
    Component: () => <StatCard label="Clicks" value="1,284" delta="+12%" positive />,
  },
  {
    name: "Empty",
    group: "layout",
    covers: ["Empty"],
    copy: ["Nothing here yet"],
    Component: () => <Empty>Nothing here yet</Empty>,
  },

  {
    name: "Button",
    group: "action",
    covers: ["Button"],
    copy: ["Save changes", "Disabled"],
    Component: () => <ButtonVariants />,
  },
  {
    name: "ProPill",
    group: "action",
    covers: ["ProPill"],
    copy: ["Pro"],
    Component: () => <ProPill />,
  },

  {
    name: "Field",
    group: "form",
    covers: ["Field"],
    copy: ["Display name", "Shown to other members."],
    Component: () => (
      <Field label="Display name" hint="Shown to other members.">
        <Input defaultValue="jungley" />
      </Field>
    ),
  },
  {
    name: "Input",
    group: "form",
    covers: ["Input"],
    copy: ["example.com"],
    Component: () => <Input placeholder="example.com" />,
  },
  {
    name: "Textarea",
    group: "form",
    covers: ["Textarea"],
    copy: ["Add a note"],
    Component: () => <Textarea placeholder="Add a note" />,
  },
  {
    name: "Select",
    group: "form",
    covers: ["Select"],
    copy: ["Daily", "Weekly"],
    Component: () => <SelectRow />,
  },
  {
    name: "Switch",
    group: "form",
    covers: ["Switch"],
    copy: ["Enable alerts"],
    Component: () => <SwitchRow />,
  },

  {
    name: "Table",
    group: "data",
    covers: ["Table", "THead", "TBody", "TR", "TH", "TD", "TableDensityProvider"],
    copy: ["Link", "Clicks", "/launch", "412", "Comfortable", "Compact"],
    Component: () => <TablePreview />,
  },
  {
    name: "TableSkeleton",
    group: "data",
    covers: ["TableSkeleton"],
    copy: [],
    Component: () => <TableSkeleton columnsCount={3} rowsCount={3} />,
  },
  {
    name: "TableEmpty",
    group: "data",
    covers: ["TableEmpty"],
    copy: ["No data yet", "No matching data found for the current filters"],
    Component: () => <TableEmpty />,
  },
  {
    name: "TablePagination",
    group: "data",
    covers: ["TablePagination"],
    copy: ["Total {{total}} items", "Page {{page}} of {{totalPages}}", "{{size}} / page", "Previous", "Next"],
    Component: () => <TablePaginationPreview />,
  },
  {
    name: "Tabs",
    group: "data",
    covers: ["Tabs"],
    copy: ["Overview", "Settings", "Overview content."],
    Component: () => (
      <Tabs
        items={[
          { value: "overview", label: "Overview", content: <p className="text-sm">Overview content.</p> },
          { value: "settings", label: "Settings", content: <p className="text-sm">Settings content.</p> },
        ]}
      />
    ),
  },
  {
    name: "Badge",
    group: "data",
    covers: ["Badge"],
    copy: ["Active"],
    Component: () => <BadgeRows />,
  },
  {
    name: "Skeleton",
    group: "data",
    covers: ["Skeleton"],
    copy: [],
    Component: () => (
      <div className="w-48 space-y-2">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    ),
  },

  {
    name: "Alert",
    group: "feedback",
    covers: ["Alert"],
    copy: ["Your changes were saved."],
    Component: () => <AlertRows />,
  },
  {
    name: "FormError",
    group: "feedback",
    covers: ["FormError"],
    copy: ["Something went wrong on the server.", "HTTP 500", "request abc-123"],
    Component: () => (
      <FormError
        err={{ message: "Something went wrong on the server.", status: 500, requestId: "abc-123" }}
      />
    ),
  },
  {
    name: "RouteFallback",
    group: "feedback",
    covers: ["RouteFallback"],
    copy: [],
    Component: () => <RouteFallback />,
  },
  {
    name: "TableError",
    group: "feedback",
    covers: ["TableError"],
    copy: ["Failed to load data", "Retry"],
    Component: () => (
      <TableError error={new Error("Request failed with status 500")} onRetry={() => {}} />
    ),
  },
  {
    name: "Tooltip",
    group: "feedback",
    covers: ["Tooltip"],
    copy: ["Copies the value"],
    Component: () => (
      <Tooltip content="Copies the value">
        <Button variant="outline">Hover me</Button>
      </Tooltip>
    ),
  },

  {
    name: "Modal",
    group: "overlay",
    covers: ["Modal"],
    copy: ["Rename link", "Modal body content."],
    Component: () => <ModalRow />,
  },
  {
    name: "Dialog",
    group: "overlay",
    covers: ["Dialog"],
    copy: ["Confirm removal"],
    Component: () => <DialogRow />,
  },

  {
    name: "Code",
    group: "data",
    covers: ["Code"],
    copy: ["v=spf1 include:octarq.dev -all"],
    Component: () => <Code>v=spf1 include:octarq.dev -all</Code>,
  },
  {
    name: "Guide",
    group: "feedback",
    covers: ["Guide"],
    copy: ["How DNS verification works", "Add the TXT record, then re-check."],
    Component: () => (
      <Guide title="How DNS verification works" open>
        <p>Add the TXT record, then re-check.</p>
      </Guide>
    ),
  },
];
