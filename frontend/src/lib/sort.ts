export type SortKey =
  | "date"
  | "description"
  | "doc"
  | "credit"
  | "debit"
  | "balance"
  | "account"
  | "bank";

export type SortDir = "asc" | "desc";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function sortTransactions<T extends { [K in SortKey]?: any }>(
  txns: T[],
  key: SortKey | null,
  dir: SortDir
): T[] {
  if (!key) return txns;
  const sorted = [...txns];
  const mul = dir === "asc" ? 1 : -1;
  sorted.sort((a, b) => {
    const av = a[key];
    const bv = b[key];
    if (av == null && bv == null) return 0;
    if (av == null) return 1;
    if (bv == null) return -1;
    if (typeof av === "number" && typeof bv === "number")
      return (av - bv) * mul;
    return String(av).localeCompare(String(bv), "pt-BR") * mul;
  });
  return sorted;
}
