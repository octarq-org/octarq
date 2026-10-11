import { useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Dialog,
  Field,
  Modal,
  Select,
  Switch,
  Table,
  TableDensityProvider,
  TablePagination,
  TBody,
  TD,
  TH,
  THead,
  TR,
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
