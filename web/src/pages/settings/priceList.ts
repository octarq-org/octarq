import type { ModelPriceDTO } from "../../api/types";

export const PRICE_PAGE_SIZE = 20;

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
