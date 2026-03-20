import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Mock the Wails bindings before importing App
vi.mock("../bindings/extratos-app", () => {
  const SearchResult = class {
    transactions: never[] = [];
    total = 0;
    total_credit = 0;
    total_debit = 0;
    net_amount = 0;
    min_date = "";
    max_date = "";
    clause_summaries: never[] = [];
  };
  return {
    AppService: {
      ListDatabases: vi.fn().mockResolvedValue([]),
      OpenDatabase: vi.fn().mockResolvedValue(null),
      CreateDatabase: vi.fn().mockResolvedValue(null),
      DeleteDatabase: vi.fn().mockResolvedValue(null),
      RenameDatabase: vi.fn().mockResolvedValue(null),
      GetCurrentDB: vi.fn().mockResolvedValue(""),
      GetDBError: vi.fn().mockResolvedValue(""),
      PreviewImport: vi.fn().mockResolvedValue(null),
      ConfirmImport: vi.fn().mockResolvedValue(null),
      CancelImport: vi.fn().mockResolvedValue(null),
      Search: vi.fn().mockResolvedValue(new SearchResult()),
      SearchFiltered: vi.fn().mockResolvedValue(new SearchResult()),
      GetMonthlySummary: vi.fn().mockResolvedValue([]),
      GetStats: vi.fn().mockResolvedValue(null),
      ExportResults: vi.fn().mockResolvedValue(null),
    },
    SearchResult,
    ImportPreview: class {},
    FilePreview: class {},
    DBInfo: class {},
    ClauseSummary: class {},
    MonthlySummary: class {},
  };
});

import App from "./App";

describe("App", () => {
  it("renders DB selection screen initially", async () => {
    render(<App />);

    expect(
      await screen.findByRole("heading", { name: "Extratos", level: 1 })
    ).toBeInTheDocument();
    expect(
      await screen.findByPlaceholderText(/Nome \(ex: Empresa X, Pessoal\)/)
    ).toBeInTheDocument();
  });
});
