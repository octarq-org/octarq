import type { EntityField } from "./types";

export type FieldError = "number" | "date" | "json";

export type FieldParse = { ok: true; value: unknown } | { ok: false; reason: FieldError };

export type FormValue = string | boolean;

export function toFormValue(field: EntityField, value: unknown): FormValue {
  if (field.widget === "checkbox") return value === true;
  if (value === null || value === undefined) return "";
  if (field.widget === "datetime") return isoToLocalInput(String(value));
  if (field.widget === "json") return prettyJSON(value);
  return String(value);
}

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

export function formatTimestamp(value: unknown, locale: string): string {
  if (value === null || value === undefined || value === "") return "";
  const d = new Date(String(value));
  return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleString(locale);
}

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

export function tableFields(fields: EntityField[]): EntityField[] {
  const declared = fields.filter((f) => f.listColumn);
  return declared.length > 0 ? declared : fields;
}
