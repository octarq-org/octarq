import { useState, type ReactNode } from "react";
import {
  Alert,
  Badge,
  Button,
  Code,
  Dialog,
  Empty,
  Field,
  FormError,
  GlassCard,
  Guide,
  Input,
  Modal,
  PageHeader,
  Panel,
  ProPill,
  RouteFallback,
  ScreenWrap,
  Select,
  Skeleton,
  StatCard,
  Switch,
  Table,
  TableDensityProvider,
  TBody,
  TD,
  TH,
  THead,
  TR,
  Tabs,
  Textarea,
  Toggle,
  Tooltip,
  type TableDensity,
} from "@octarq/plugin-sdk";

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
    name: "Toggle",
    group: "form",
    covers: ["Toggle"],
    copy: ["Enable alerts"],
    Component: () => <ToggleRow />,
  },
  {
    name: "Switch",
    group: "form",
    covers: ["Switch"],
    copy: [],
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

// Previews that need local state. Each is its own component so switching catalog
// entries cannot disturb the workbench's hook order.

function ButtonVariants() {
  return (
    <div className="flex flex-wrap items-center gap-3">
      {(["primary", "secondary", "subtle", "ghost", "outline", "danger"] as const).map((variant) => (
        <Button key={variant} variant={variant}>
          Save changes
        </Button>
      ))}
      <Button size="sm">Sm</Button>
      <Button size="md">Md</Button>
      <Button size="lg">Lg</Button>
      <Button disabled>Disabled</Button>
    </div>
  );
}

function SelectRow() {
  const [value, setValue] = useState("daily");
  return (
    <Select
      value={value}
      onValueChange={setValue}
      options={[
        { value: "daily", label: "Daily" },
        { value: "weekly", label: "Weekly" },
      ]}
    />
  );
}

function ToggleRow() {
  const [on, setOn] = useState(true);
  return (
    <Field label="Enable alerts">
      <Toggle on={on} onChange={setOn} />
    </Field>
  );
}

function SwitchRow() {
  const [checked, setChecked] = useState(false);
  return <Switch checked={checked} onCheckedChange={setChecked} />;
}

function BadgeRows() {
  return (
    <div className="flex flex-wrap gap-2">
      {(["default", "info", "success", "warning", "danger", "secondary", "outline"] as const).map(
        (tone) => (
          <Badge key={tone} tone={tone}>
            Active
          </Badge>
        ),
      )}
    </div>
  );
}

function AlertRows() {
  return (
    <div className="space-y-3">
      {(["info", "success", "warning", "danger"] as const).map((variant) => (
        <Alert key={variant} variant={variant}>
          Your changes were saved.
        </Alert>
      ))}
    </div>
  );
}

function ModalRow() {
  // Modal has no `open` prop: mounting it IS opening it (it wraps Dialog with
  // open). So the preview mounts it only while the button has been clicked.
  const [open, setOpen] = useState(true);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open modal</Button>
      {open && (
        <Modal title="Rename link" onClose={() => setOpen(false)}>
          <p className="text-sm text-muted-foreground">Modal body content.</p>
        </Modal>
      )}
    </>
  );
}

function DialogRow() {
  const [open, setOpen] = useState(true);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open dialog</Button>
      <Dialog open={open} onOpenChange={setOpen} title="Confirm removal">
        <p className="text-sm text-muted-foreground">This cannot be undone.</p>
      </Dialog>
    </>
  );
}

function TablePreview() {
  const [density, setDensity] = useState<TableDensity>("comfortable");
  return (
    <div className="space-y-3">
      <div className="flex gap-2">
        {(["comfortable", "compact"] as const).map((d) => (
          <Button
            key={d}
            size="sm"
            variant={density === d ? "primary" : "ghost"}
            onClick={() => setDensity(d)}
          >
            {d === "comfortable" ? "Comfortable" : "Compact"}
          </Button>
        ))}
      </div>
      <TableDensityProvider density={density} onDensityChange={setDensity}>
        <Table>
          <THead>
            <TR>
              <TH>Link</TH>
              <TH>Clicks</TH>
            </TR>
          </THead>
          <TBody>
            <TR>
              <TD>/launch</TD>
              <TD>412</TD>
            </TR>
            <TR>
              <TD>/pricing</TD>
              <TD>88</TD>
            </TR>
          </TBody>
        </Table>
      </TableDensityProvider>
    </div>
  );
}
