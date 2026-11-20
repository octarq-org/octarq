
export type FieldType =
  | "string"
  | "text"
  | "integer"
  | "number"
  | "boolean"
  | "timestamp"
  | "uuid"
  | "json";

export type Widget = "text" | "textarea" | "number" | "checkbox" | "datetime" | "json";

export interface EntityField {
  name: string;
  type: FieldType;
  label: string;
  widget: Widget;
  required: boolean;
  unique: boolean;
  listColumn: boolean;
}

export type EntityOperation = "list" | "get" | "create" | "update" | "delete";

export interface CatalogEntity {
  slug: string;
  label: string;
  pluralLabel: string;
  icon: string;
  tier?: "pro" | "elite";
  fields: EntityField[];
  operations: EntityOperation[];
}

export interface Catalog {
  entities: CatalogEntity[];
}

export type DataRow = Record<string, unknown> & { id: string };

export interface DataList {
  items: DataRow[];
  total: number;
  limit: number;
  offset: number;
}

export const RESERVED_COLUMNS = ["id", "org_id", "created_at", "updated_at"];
