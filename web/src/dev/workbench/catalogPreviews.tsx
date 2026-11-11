import { useState } from "react";
import {
  Alert,
  AreaChart,
  Badge,
  BarList,
  Button,
  Dialog,
  Field,
  Modal,
  Select,
  SetupChecklistProvider,
  SetupStep,
  Switch,
  Table,
  TableDensityProvider,
  TabBar,
  TablePagination,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useSetupChecklist,
  type TableDensity,
} from "@octarq/plugin-sdk";

export function ButtonVariants() {
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

export function SelectRow() {
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

export function SwitchRow() {
  const [on, setOn] = useState(true);
  return (
    <Field label="Enable alerts">
      <Switch checked={on} onCheckedChange={setOn} />
    </Field>
  );
}

export function BadgeRows() {
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

export function AlertRows() {
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

export function ModalRow() {
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

export function DialogRow() {
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

export function TablePaginationPreview() {
  const [page, setPage] = useState(2);
  const [pageSize, setPageSize] = useState(10);
  return (
    <TablePagination
      page={page}
      pageSize={pageSize}
      total={48}
      pageCount={Math.ceil(48 / pageSize)}
      onPageChange={setPage}
      onPageSizeChange={(size) => {
        setPageSize(size);
        setPage(1);
      }}
    />
  );
}

export function TablePreview() {
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

export function TabBarPreview() {
  const [value, setValue] = useState("overview");
  return (
    <TabBar
      value={value}
      onValueChange={setValue}
      items={[
        { value: "overview", label: "Overview" },
        { value: "activity", label: "Activity", count: 3 },
        { value: "settings", label: "Settings" },
      ]}
    />
  );
}

export function AreaChartPreview() {
  const sampleSeries = [
    { key: "Mon", count: 12 },
    { key: "Tue", count: 24 },
    { key: "Wed", count: 18 },
    { key: "Thu", count: 36 },
    { key: "Fri", count: 48 },
    { key: "Sat", count: 30 },
    { key: "Sun", count: 42 },
  ];
  return (
    <div className="w-full max-w-xl space-y-2">
      <div className="rounded-xl border border-border p-4 bg-well">
        <h4 className="text-xs font-medium text-muted-foreground mb-2">Traffic (Last 7 Days)</h4>
        <AreaChart series={sampleSeries} height={140} />
      </div>
    </div>
  );
}

export function BarListPreview() {
  const sampleRows = [
    { key: "google.com", count: 1280 },
    { key: "github.com", count: 840 },
    { key: "twitter.com", count: 520 },
    { key: "linkedin.com", count: 210 },
    { key: "", count: 95 },
  ];
  return (
    <div className="w-full max-w-md space-y-2">
      <div className="rounded-xl border border-border p-4 bg-well">
        <h4 className="text-xs font-medium text-muted-foreground mb-3">Top Referrers</h4>
        <BarList rows={sampleRows} />
      </div>
    </div>
  );
}

export function SetupStepPreview() {
  const [completed, setCompleted] = useState(false);
  return (
    <div className="w-full max-w-md space-y-3">
      <SetupStep
        title="Create your first link"
        description="Set up a target destination URL and custom slug."
        completed={completed}
        onClick={() => setCompleted(!completed)}
      />
      <SetupStep
        title="Configure custom domain"
        description="Point your custom domain CNAME record to the cluster."
        completed={true}
        onClick={() => {}}
      />
    </div>
  );
}

function ChecklistSummary() {
  const { totalCount, completedCount, progressPercent, allCompleted } = useSetupChecklist();
  return (
    <div className="mb-3 flex items-center justify-between text-xs text-muted-foreground">
      <span>
        {completedCount} of {totalCount} completed ({progressPercent}%)
      </span>
      {allCompleted && <span className="text-success-fg font-medium">All steps complete!</span>}
    </div>
  );
}

export function SetupChecklistProviderPreview() {
  const [step1, setStep1] = useState(true);
  const [step2, setStep2] = useState(false);
  const [step3, setStep3] = useState(false);
  return (
    <SetupChecklistProvider>
      <div className="w-full max-w-md space-y-3">
        <ChecklistSummary />
        <SetupStep
          title="Verify domain ownership"
          description="Add a TXT record to prove domain ownership."
          completed={step1}
          onClick={() => setStep1(!step1)}
        />
        <SetupStep
          title="Configure DNS records"
          description="Direct inbound mail and traffic to Octarq."
          completed={step2}
          onClick={() => setStep2(!step2)}
        />
        <SetupStep
          title="Create first mailbox"
          description="Set up default email address and forwarders."
          completed={step3}
          onClick={() => setStep3(!step3)}
        />
      </div>
    </SetupChecklistProvider>
  );
}

