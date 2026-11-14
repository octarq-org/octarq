import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import {
  Button,
  Field,
  Input,
  Modal,
  PageHeader,
  ScreenWrap,
  Switch,
  TableError,
  TableSkeleton,
  Textarea,
  confirmDialog,
  toast,
} from "@octarq/plugin-sdk";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { req } from "../../api";
import { useTranslation } from "../../i18n";
import { ProTable, type ProColumn, type ProTableActionRef } from "../../components/pro-table";
import {
  formatFieldValue,
  formatTimestamp,
  parseFieldValue,
  tableFields,
  toFormValue,
  type FormValue,
} from "./fields";
import type { Catalog, CatalogEntity, DataList, DataRow, EntityField } from "./types";

// DataPage renders one declared entity: the rows, and a form built from the
// declaration. It is the same page for every entity — the declaration decides
// the columns, the widgets and which actions appear.
export default function DataPage() {
  const { slug = "" } = useParams();
  const { t, lang } = useTranslation();
  const actionRef = useRef<ProTableActionRef | undefined>(undefined);
  const [editing, setEditing] = useState<DataRow | null>(null);
  const [creating, setCreating] = useState(false);

  const catalog = useQuery({
    queryKey: ["data-catalog"],
    queryFn: () => req<Catalog>("GET", "/api/data"),
  });
  const entity = catalog.data?.entities.find((e) => e.slug === slug);

  if (catalog.isPending) return <TableSkeleton rowsCount={6} />;
  if (catalog.isError) {
    return (
      <TableError error={catalog.error} onRetry={() => void catalog.refetch()} />
    );
  }
  if (!entity) {
    return (
      <ScreenWrap>
        <PageHeader title={t("data.notFoundTitle")} description={t("data.notFoundDesc")} />
      </ScreenWrap>
    );
  }

  // A nested function does not keep the narrowing above, so the entity is
  // captured once here.
  const current = entity;
  const canCreate = entity.operations.includes("create");
  const canUpdate = entity.operations.includes("update");
  const canDelete = entity.operations.includes("delete");

  const columns = buildColumns(entity, {
    yes: t("data.yes"),
    no: t("data.no"),
    locale: lang,
    editLabel: t("data.edit"),
    deleteLabel: t("data.delete"),
    createdAtLabel: t("data.createdAt"),
    actionsLabel: t("data.actions"),
    canUpdate,
    canDelete,
    onEdit: (row) => setEditing(row),
    onDelete: (row) => void removeRow(row),
  });

  async function removeRow(row: DataRow) {
    const ok = await confirmDialog({
      title: t("data.deleteTitle"),
      message: t("data.deleteConfirm"),
      confirmLabel: t("data.delete"),
      danger: true,
    });
    if (!ok) return;
    try {
      await req("DELETE", `/api/data/${current.slug}/${encodeURIComponent(row.id)}`);
      toast.success(t("data.deleted"));
      void actionRef.current?.reload();
    } catch (err) {
      toast.error(errMessage(err, t("data.deleteFailed")));
    }
  }

  return (
    <ScreenWrap>
      <PageHeader
        title={entity.pluralLabel}
        eyebrow={entity.label}
        description={t("data.pageDesc")}
      />
      <ProTable<DataRow>
        columns={columns}
        rowKey="id"
        search={false}
        queryKeyPrefix={`data-${entity.slug}`}
        actionRef={actionRef}
        request={async (params, sortMap) => {
          const query = new URLSearchParams({
            limit: String(params.pageSize),
            offset: String((params.page - 1) * params.pageSize),
          });
          const sortField = Object.keys(sortMap)[0];
          if (sortField) {
            query.set("sort", `${sortMap[sortField] === "desc" ? "-" : ""}${sortField}`);
          }
          const res = await req<DataList>(
            "GET",
            `/api/data/${entity.slug}?${query.toString()}`,
          );
          return { data: res.items, total: res.total };
        }}
        emptyText={t("data.empty")}
        emptyReason={t("data.emptyReason")}
        toolBarRender={() =>
          canCreate
            ? [
                <Button key="new" onClick={() => setCreating(true)}>
                  <Plus className="h-4 w-4" aria-hidden />
                  {t("data.new")}
                </Button>,
              ]
            : []
        }
      />
      {(creating || editing) && (
        <EntityForm
          entity={entity}
          record={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={() => {
            setCreating(false);
            setEditing(null);
            void actionRef.current?.reload();
          }}
        />
      )}
    </ScreenWrap>
  );
}

interface ColumnLabels {
  yes: string;
  no: string;
  locale: string;
  editLabel: string;
  deleteLabel: string;
  createdAtLabel: string;
  actionsLabel: string;
  canUpdate: boolean;
  canDelete: boolean;
  onEdit: (row: DataRow) => void;
  onDelete: (row: DataRow) => void;
}

function buildColumns(entity: CatalogEntity, labels: ColumnLabels): ProColumn<DataRow>[] {
  const cols: ProColumn<DataRow>[] = tableFields(entity.fields).map((field) => ({
    title: field.label,
    dataIndex: field.name,
    key: field.name,
    hideInSearch: true,
    sorter: true,
    render: (_, row) => (
      <span className="whitespace-nowrap text-sm">
        {formatFieldValue(field, row[field.name], labels)}
      </span>
    ),
  }));
  cols.push({
    title: labels.createdAtLabel,
    dataIndex: "created_at",
    key: "created_at",
    hideInSearch: true,
    sorter: true,
    render: (_, row) => (
      <span className="whitespace-nowrap font-mono tnum text-xs text-muted-foreground">
        {formatTimestamp(row["created_at"], labels.locale)}
      </span>
    ),
  });
  if (labels.canUpdate || labels.canDelete) {
    cols.push({
      title: labels.actionsLabel,
      key: "actions",
      hideInSearch: true,
      render: (_, row) => (
        <div className="flex items-center gap-1">
          {labels.canUpdate && (
            <Button
              variant="ghost"
              size="sm"
              aria-label={labels.editLabel}
              onClick={() => labels.onEdit(row)}
            >
              <Pencil className="h-4 w-4" aria-hidden />
            </Button>
          )}
          {labels.canDelete && (
            <Button
              variant="ghost"
              size="sm"
              aria-label={labels.deleteLabel}
              onClick={() => labels.onDelete(row)}
            >
              <Trash2 className="h-4 w-4" aria-hidden />
            </Button>
          )}
        </div>
      ),
    });
  }
  return cols;
}

interface EntityFormProps {
  entity: CatalogEntity;
  record: DataRow | null;
  onClose: () => void;
  onSaved: () => void;
}

// EntityForm is the create and edit dialog. The record being edited decides
// which: null creates.
function EntityForm({ entity, record, onClose, onSaved }: EntityFormProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const creating = record === null;
  const [values, setValues] = useState<Record<string, FormValue>>(() => {
    const init: Record<string, FormValue> = {};
    for (const field of entity.fields) {
      init[field.name] = toFormValue(field, record?.[field.name]);
    }
    return init;
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {};
      const nextErrors: Record<string, string> = {};
      for (const field of entity.fields) {
        const raw = values[field.name] ?? "";
        const parsed = parseFieldValue(field, raw);
        if (!parsed.ok) {
          nextErrors[field.name] =
            parsed.reason === "number"
              ? t("data.errNumber")
              : parsed.reason === "date"
                ? t("data.errDate")
                : t("data.errJson");
          continue;
        }
        if (parsed.value === null) {
          if (field.required) {
            nextErrors[field.name] = t("data.errRequired");
            continue;
          }
          // A cleared field is a clear on update; on create, omitting it lets
          // the declaration's default apply.
          if (!creating) body[field.name] = null;
          continue;
        }
        body[field.name] = parsed.value;
      }
      setErrors(nextErrors);
      if (Object.keys(nextErrors).length > 0) return;
      if (creating) {
        await req("POST", `/api/data/${entity.slug}`, body);
      } else {
        await req("PATCH", `/api/data/${entity.slug}/${encodeURIComponent(record.id)}`, body);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["data-catalog"] });
      toast.success(creating ? t("data.created") : t("data.updated"));
      onSaved();
    },
    onError: (err) => toast.error(errMessage(err, t("data.saveFailed"))),
  });

  return (
    <Modal
      title={creating ? t("data.new") : t("data.edit")}
      onClose={onClose}
      wide
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        {entity.fields.map((field) => (
          <Field
            key={field.name}
            label={field.required ? `${field.label} *` : field.label}
            hint={errors[field.name]}
          >
            <FieldControl
              field={field}
              value={values[field.name] ?? ""}
              onChange={(v) => setValues((prev) => ({ ...prev, [field.name]: v }))}
            />
          </Field>
        ))}
        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={save.isPending}>
            {save.isPending ? t("data.saving") : t("data.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function FieldControl({
  field,
  value,
  onChange,
}: {
  field: EntityField;
  value: FormValue;
  onChange: (value: FormValue) => void;
}) {
  const text = typeof value === "string" ? value : "";
  switch (field.widget) {
    case "checkbox":
      return <Switch checked={value === true} onCheckedChange={(v) => onChange(v)} />;
    case "textarea":
      return <Textarea value={text} onChange={(e) => onChange(e.target.value)} />;
    case "json":
      return (
        <Textarea
          value={text}
          spellCheck={false}
          className="font-mono text-xs"
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case "number":
      return (
        <Input
          type="number"
          value={text}
          step={field.type === "integer" ? 1 : "any"}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    case "datetime":
      return (
        <Input
          type="datetime-local"
          value={text}
          onChange={(e) => onChange(e.target.value)}
        />
      );
    default:
      return <Input value={text} onChange={(e) => onChange(e.target.value)} />;
  }
}

function errMessage(err: unknown, fallback: string): string {
  if (err && typeof err === "object" && "message" in err) {
    const message = (err as { message?: unknown }).message;
    if (typeof message === "string" && message !== "") return message;
  }
  return fallback;
}
