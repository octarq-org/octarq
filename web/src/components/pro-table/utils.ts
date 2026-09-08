import { ReactNode } from "react";
import { ColumnDef } from "@tanstack/react-table";
import { z } from "zod";
import { ProColumn, ProTableActionRef } from "./types";

/**
 * Remove empty strings, null, undefined, and whitespace-only values from search params.
 * Empty fields become undefined, preventing dirty parameters from overriding default behavior.
 */
export function cleanParams<T extends Record<string, any>>(params: T): Record<string, any> {
  const result: Record<string, any> = {};
  for (const [key, value] of Object.entries(params || {})) {
    if (value === undefined || value === null) {
      continue;
    }
    if (typeof value === "string") {
      const trimmed = value.trim();
      if (trimmed === "") {
        continue;
      }
      result[key] = trimmed;
      continue;
    }
    result[key] = value;
  }
  return result;
}

/**
 * Validates data against a Zod schema, returning the fallback on validation failure.
 * Ensures network boundary type safety without throwing exceptions or bare `as T` casting.
 */
export function parseWithFallback<T>(
  schema: z.ZodType<T>,
  data: unknown,
  fallback: T,
  onError?: (err: z.ZodError) => void
): T {
  const parsed = schema.safeParse(data);
  if (parsed.success) {
    return parsed.data;
  }
  if (onError) {
    onError(parsed.error);
  } else if (process.env.NODE_ENV !== "production") {
    console.warn("[ProTable Zod Schema Error]:", parsed.error.issues);
  }
  return fallback;
}

/**
 * Validates an array of items against a Zod schema. Invalid items are either discarded
 * or replaced with the optional fallback item.
 */
export function parseArrayWithFallback<T>(
  itemSchema: z.ZodType<T>,
  data: unknown,
  fallbackItem?: T
): T[] {
  if (!Array.isArray(data)) return [];
  const valid: T[] = [];
  for (const item of data) {
    const res = itemSchema.safeParse(item);
    if (res.success) {
      valid.push(res.data);
    } else if (fallbackItem !== undefined) {
      valid.push(fallbackItem);
    }
  }
  return valid;
}

/**
 * Normalizes ProColumn definitions into TanStack Table ColumnDef.
 */
export function normalizeColumns<TData>(
  columns: ProColumn<TData>[],
  actionRef?: ProTableActionRef
): ColumnDef<TData, any>[] {
  return columns
    .filter((col) => !col.hideInTable)
    .map((col, index) => {
      const colId = col.key || (col.dataIndex ? String(col.dataIndex) : `col_${index}`);
      const accessorKey = col.dataIndex ? String(col.dataIndex) : undefined;

      const columnDef: ColumnDef<TData, any> = {
        id: colId,
        ...(accessorKey ? { accessorKey } : {}),
        header: (headerProps) => {
          if (typeof col.header === "function") {
            return col.header(headerProps);
          }
          if (col.header !== undefined) {
            return col.header;
          }
          if (col.title !== undefined) {
            return col.title;
          }
          return colId;
        },
        enableSorting: col.sorter !== undefined ? !!col.sorter : true,
        ...(typeof col.width === "number" ? { size: col.width } : {}),
        ...(typeof col.minWidth === "number" ? { minSize: col.minWidth } : {}),
        ...(typeof col.maxWidth === "number" ? { maxSize: col.maxWidth } : {}),
      };

      if (col.render) {
        columnDef.cell = ({ getValue, row }) => {
          const dom = getValue() as ReactNode;
          return col.render!(dom, row.original, row.index, actionRef);
        };
      } else if (col.valueEnum) {
        columnDef.cell = ({ getValue }) => {
          const val = String(getValue() ?? "");
          const enumItem = col.valueEnum![val];
          if (!enumItem) return val;
          if (typeof enumItem === "object" && enumItem !== null && "text" in enumItem) {
            return (enumItem as { text: ReactNode }).text;
          }
          return enumItem as ReactNode;
        };
      }

      return columnDef;
    });
}
