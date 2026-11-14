// Turning declared fields into form controls and back. Pure, so the conversion
// rules can be tested without a browser: the backend coerces by declared type,
// and a value the form sends in the wrong shape becomes a 422 the operator
// cannot act on.
import type { EntityField } from "./types";

/** Why a field's input could not be turned into a value. The page translates it. */
export type FieldError = "number" | "date" | "json";

export type FieldParse = { ok: true; value: unknown } | { ok: false; reason: FieldError };

export type FormValue = string | boolean;

/** The value a form starts with: what the record holds, in the control's shape. */
export function toFormValue(field: EntityField, value: unknown): FormValue {
  if (field.widget === "checkbox") return value === true;
  if (value === null || value === undefined) return "";
  if (field.widget === "datetime") return isoToLocalInput(String(value));
  if (field.widget === "json") return prettyJSON(value);
  return String(value);
}

/** RFC 3339 -> the "YYYY-MM-DDTHH:mm" a datetime-local input reads, in local time. */
export function isoToLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function prettyJSON(value: unknown): string {
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}

/**
 * One form control's value -> the JSON the API expects for that field.
 *
 * An empty optional control yields null so a cleared field is a clear, not a
 * silent no-op. The caller decides whether to send it: on create an omitted
 * field takes the declaration's default, on update it keeps its current value.
 */
export function parseFieldValue(field: EntityField, raw: FormValue): FieldParse {
  if (field.widget === "checkbox") {
    return { ok: true, value: raw === true };
  }
  const text = typeof raw === "string" ? raw : "";
  if (text.trim() === "") {
    return { ok: true, value: null };
  }
  switch (field.type) {
    case "integer": {
      const n = Number(text);
      if (!Number.isInteger(n)) return { ok: false, reason: "number" };
      return { ok: true, value: n };
    }
    case "number": {
      const n = Number(text);
      if (!Number.isFinite(n)) return { ok: false, reason: "number" };
      return { ok: true, value: n };
    }
    case "timestamp": {
      const d = new Date(text);
      if (Number.isNaN(d.getTime())) return { ok: false, reason: "date" };
      return { ok: true, value: d.toISOString() };
    }
    case "json": {
      try {
        return { ok: true, value: JSON.parse(text) };
      } catch {
        return { ok: false, reason: "json" };
      }
    }
    default:
      return { ok: true, value: text };
  }
}

/** An RFC 3339 value as the table shows it, in the reader's locale. */
export function formatTimestamp(value: unknown, locale: string): string {
  if (value === null || value === undefined || value === "") return "";
  const d = new Date(String(value));
  return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleString(locale);
}

/** A stored value as the table shows it. */
export function formatFieldValue(
  field: EntityField,
  value: unknown,
  labels: { yes: string; no: string; locale: string },
): string {
  if (value === null || value === undefined) return "";
  switch (field.type) {
    case "boolean":
      return value === true ? labels.yes : labels.no;
    case "timestamp":
      return formatTimestamp(value, labels.locale);
    case "json":
      return typeof value === "string" ? value : JSON.stringify(value);
    default:
      return String(value);
  }
}

/** The fields the table shows, in declaration order: the declared list columns,
 *  or every field when the declaration marks none. */
export function tableFields(fields: EntityField[]): EntityField[] {
  const declared = fields.filter((f) => f.listColumn);
  return declared.length > 0 ? declared : fields;
}
