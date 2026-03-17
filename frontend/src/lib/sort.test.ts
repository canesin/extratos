import { describe, it, expect } from "vitest";
import { sortTransactions } from "./sort";

const txns = [
  { date: "2026-01-05", description: "PIX Alpha", doc: "", credit: 100, debit: null, balance: 200, account: "A", bank: "Bradesco" },
  { date: "2026-01-02", description: "Boleto Beta", doc: "123", credit: null, debit: -50, balance: 100, account: "B", bank: "Itau" },
  { date: "2026-01-10", description: "TED Gamma", doc: "", credit: 300, debit: null, balance: null, account: "A", bank: "Nubank" },
];

describe("sortTransactions", () => {
  it("returns original when key is null", () => {
    const result = sortTransactions(txns, null, "asc");
    expect(result).toBe(txns);
  });

  it("sorts by date ascending", () => {
    const result = sortTransactions(txns, "date", "asc");
    expect(result.map((t) => t.date)).toEqual([
      "2026-01-02",
      "2026-01-05",
      "2026-01-10",
    ]);
  });

  it("sorts by date descending", () => {
    const result = sortTransactions(txns, "date", "desc");
    expect(result.map((t) => t.date)).toEqual([
      "2026-01-10",
      "2026-01-05",
      "2026-01-02",
    ]);
  });

  it("sorts by credit with nulls last", () => {
    const result = sortTransactions(txns, "credit", "asc");
    expect(result.map((t) => t.credit)).toEqual([100, 300, null]);
  });

  it("sorts by debit with nulls last", () => {
    const result = sortTransactions(txns, "debit", "desc");
    expect(result.map((t) => t.debit)).toEqual([-50, null, null]);
  });

  it("sorts by description string (locale-aware)", () => {
    const result = sortTransactions(txns, "description", "asc");
    expect(result.map((t) => t.description)).toEqual([
      "Boleto Beta",
      "PIX Alpha",
      "TED Gamma",
    ]);
  });

  it("sorts by bank descending", () => {
    const result = sortTransactions(txns, "bank", "desc");
    expect(result.map((t) => t.bank)).toEqual(["Nubank", "Itau", "Bradesco"]);
  });

  it("does not mutate original array", () => {
    const original = [...txns];
    sortTransactions(txns, "date", "asc");
    expect(txns).toEqual(original);
  });

  it("handles all-null column", () => {
    const data = [
      { date: "2026-01-01", description: "A", doc: "", credit: null, debit: null, balance: null, account: "", bank: "" },
      { date: "2026-01-02", description: "B", doc: "", credit: null, debit: null, balance: null, account: "", bank: "" },
    ];
    const result = sortTransactions(data, "balance", "asc");
    expect(result).toHaveLength(2);
  });
});
