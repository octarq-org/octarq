// Shapes the ontology plugin serves at /api/data. The declaration lives in the
// backend (octarq.entities.json); this file is only the wire contract.

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
  /**
   * The license tier the page needs to open, in the self-hosted vocabulary.
   * Absent means free. It is the list operation's tier: that is the request
   * this page makes first, so it is the one a 402 comes from.
   */
  tier?: "pro" | "elite";
  fields: EntityField[];
  /** The operations this caller may perform. The backend already filtered. */
  operations: EntityOperation[];
}

export interface Catalog {
  entities: CatalogEntity[];
}

/** One stored record. Columns come from the declaration, so values are dynamic. */
export type DataRow = Record<string, unknown> & { id: string };

export interface DataList {
  items: DataRow[];
  total: number;
  limit: number;
  offset: number;
}

/** The keys the platform owns; a form never renders them. */
export const RESERVED_COLUMNS = ["id", "org_id", "created_at", "updated_at"];
