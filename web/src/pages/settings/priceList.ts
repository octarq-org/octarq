import type { ModelPriceDTO } from "../../api/types";

export const PRICE_PAGE_SIZE = 20;

export function modelId(row: Pick<ModelPriceDTO, "model" | "providerId">): string {
  return row.providerId ? `${row.providerId}/${row.model}` : row.model;
}

export function priceRowKey(row: Pick<ModelPriceDTO, "model" | "providerId">): string {
  return `${row.providerId}/${row.model}`;
}

export function formatPrice(value: number | null | undefined): string {
  if (value == null) return "—";
  return value
    .toFixed(6)
    .replace(/0+$/, "")
    .replace(/\.$/, "");
}

export function filterModelPrices(
  prices: ModelPriceDTO[],
  query: string,
  provider: string,
): ModelPriceDTO[] {
  const q = query.trim().toLowerCase();
  return prices.filter((row) => {
    if (provider !== "all" && row.providerId !== provider) return false;
    if (!q) return true;
    return (
      row.model.toLowerCase().includes(q) ||
      row.displayName.toLowerCase().includes(q) ||
      row.providerId.toLowerCase().includes(q)
    );
  });
}

export function paginatePrices<T>(
  rows: T[],
  page: number,
  pageSize: number,
): { page: number; pageCount: number; rows: T[] } {
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const safePage = Math.min(Math.max(1, page), pageCount);
  return {
    page: safePage,
    pageCount,
    rows: rows.slice((safePage - 1) * pageSize, safePage * pageSize),
  };
}
