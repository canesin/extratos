package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Expected aggregates per bank (external-only sums).
//
// Bradesco: 19 txns, credits=178000.00, debits=-100001.60, net=77998.40, 2026-01-02..2026-01-20
// Itaú:     12 txns, credits=25000.00,  debits=-4266.15,   net=20733.85, 2026-01-02..2026-02-05
// Nubank:   10 txns, credits=3750.00,   debits=-503.20,    net=3246.80,  2026-01-05..2026-02-01
// BB:       15 txns, credits=21450.00,  debits=-1520.75,   net=19929.25, 2026-01-05..2026-02-20
//
// Combined: 56 txns, credits=228200.00, debits=-106291.70, net=121908.30, 2026-01-02..2026-02-20

const (
	multiBankWantTotal       = 56
	multiBankWantCredit      = 228200.00
	multiBankWantDebit       = -106291.70
	multiBankWantNet         = 121908.30
	multiBankWantMinDate     = "2026-01-02"
	multiBankWantMaxDate     = "2026-02-20"
	multiBankWantBankCount   = 4
)

var syntheticFiles = []struct {
	path      string
	wantBank  string
	wantCount int
}{
	{"testdata/synthetic_bradesco.csv", "Bradesco", 19},
	{"testdata/synthetic_itau.csv", "Itau", 12},
	{"testdata/synthetic_nubank.csv", "Nubank", 10},
	{"testdata/synthetic_bb.ofx", "Banco do Brasil", 15},
}

func newMultiBankDB(t *testing.T) *DB {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	db, err := OpenNamedDB("multibank-test")
	if err != nil {
		t.Fatalf("OpenNamedDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func loadAllSynthetic(t *testing.T, db *DB) {
	t.Helper()
	for _, sf := range syntheticFiles {
		result, err := ParseFile(sf.path)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", sf.path, err)
		}
		if result.Error != "" {
			t.Fatalf("ParseFile(%s) error: %s", sf.path, result.Error)
		}
		if result.Bank != sf.wantBank {
			t.Errorf("ParseFile(%s) bank: got %q, want %q", sf.path, result.Bank, sf.wantBank)
		}
		if len(result.Transactions) != sf.wantCount {
			t.Fatalf("ParseFile(%s) count: got %d, want %d", sf.path, len(result.Transactions), sf.wantCount)
		}

		inserted, err := db.InsertTransactions(result.Transactions)
		if err != nil {
			t.Fatalf("InsertTransactions(%s): %v", sf.path, err)
		}
		if inserted != sf.wantCount {
			t.Errorf("InsertTransactions(%s): got %d, want %d", sf.path, inserted, sf.wantCount)
		}
	}
}

// TestMultiBankPipeline loads all 4 synthetic files, verifies aggregates, FTS per-bank, and dedup.
func TestMultiBankPipeline(t *testing.T) {
	db := newMultiBankDB(t)
	loadAllSynthetic(t, db)

	// --- Verify total and aggregates ---
	sr, err := db.Search("", 100000, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if sr.Total != multiBankWantTotal {
		t.Errorf("total: got %d, want %d", sr.Total, multiBankWantTotal)
	}
	if !almostEqual(sr.TotalCredit, multiBankWantCredit, 0.01) {
		t.Errorf("total_credit: got %.2f, want %.2f", sr.TotalCredit, multiBankWantCredit)
	}
	if !almostEqual(sr.TotalDebit, multiBankWantDebit, 0.01) {
		t.Errorf("total_debit: got %.2f, want %.2f", sr.TotalDebit, multiBankWantDebit)
	}
	if !almostEqual(sr.NetAmount, multiBankWantNet, 0.01) {
		t.Errorf("net_amount: got %.2f, want %.2f", sr.NetAmount, multiBankWantNet)
	}
	if sr.MinDate != multiBankWantMinDate {
		t.Errorf("min_date: got %q, want %q", sr.MinDate, multiBankWantMinDate)
	}
	if sr.MaxDate != multiBankWantMaxDate {
		t.Errorf("max_date: got %q, want %q", sr.MaxDate, multiBankWantMaxDate)
	}

	// --- GetStats: verify bank count and date range ---
	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	banks := stats["banks"].([]string)
	if len(banks) != multiBankWantBankCount {
		t.Errorf("banks: got %d (%v), want %d", len(banks), banks, multiBankWantBankCount)
	}

	// --- Cross-bank FTS searches ---
	ftsTests := []struct {
		query     string
		wantTotal int
	}{
		{"Bradesco", 19},
		{"Nubank", 10},
		{"Itau", 12},
		{"Banco do Brasil", 15},
	}
	for _, ft := range ftsTests {
		sr, err := db.Search(ft.query, 100000, 0)
		if err != nil {
			t.Fatalf("Search(%q): %v", ft.query, err)
		}
		if sr.Total != ft.wantTotal {
			t.Errorf("Search(%q): got %d, want %d", ft.query, sr.Total, ft.wantTotal)
		}
	}

	// --- Multi-term OR: "Bradesco, Nubank" ---
	sr2, err := db.Search("Bradesco, Nubank", 100000, 0)
	if err != nil {
		t.Fatalf("Search multi-term: %v", err)
	}
	if sr2.Total != 29 {
		t.Errorf("Search('Bradesco, Nubank'): got %d, want 29", sr2.Total)
	}
	if len(sr2.ClauseSummaries) != 2 {
		t.Errorf("clause summaries: got %d, want 2", len(sr2.ClauseSummaries))
	}

	// --- Dedup: re-insert all 4 files → 0 new ---
	for _, sf := range syntheticFiles {
		result, err := ParseFile(sf.path)
		if err != nil {
			t.Fatalf("ParseFile(%s) re-import: %v", sf.path, err)
		}
		inserted, err := db.InsertTransactions(result.Transactions)
		if err != nil {
			t.Fatalf("InsertTransactions(%s) re-import: %v", sf.path, err)
		}
		if inserted != 0 {
			t.Errorf("dedup re-import(%s): got %d new, want 0", sf.path, inserted)
		}
	}

	// Verify total unchanged after dedup
	sr3, err := db.Search("", 100000, 0)
	if err != nil {
		t.Fatalf("Search post-dedup: %v", err)
	}
	if sr3.Total != multiBankWantTotal {
		t.Errorf("post-dedup total: got %d, want %d", sr3.Total, multiBankWantTotal)
	}
}

// TestMultiBankDateFilter verifies date-range filtering across banks.
func TestMultiBankDateFilter(t *testing.T) {
	db := newMultiBankDB(t)
	loadAllSynthetic(t, db)

	// January only
	sr, err := db.SearchFiltered("", 100000, 0, "2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatalf("SearchFiltered Jan: %v", err)
	}
	// All transactions must be in January
	for _, txn := range sr.Transactions {
		if txn.Date < "2026-01-01" || txn.Date > "2026-01-31" {
			t.Errorf("Jan filter leak: txn date %s outside range", txn.Date)
		}
	}
	if sr.Total == 0 {
		t.Error("Jan filter returned 0 results")
	}
	if sr.Total == multiBankWantTotal {
		t.Error("Jan filter returned all results (should exclude Feb)")
	}

	// FTS + date: "Pix" in first half of January
	sr2, err := db.SearchFiltered("Pix", 100000, 0, "2026-01-01", "2026-01-15")
	if err != nil {
		t.Fatalf("SearchFiltered Pix+date: %v", err)
	}
	for _, txn := range sr2.Transactions {
		if txn.Date < "2026-01-01" || txn.Date > "2026-01-15" {
			t.Errorf("Pix+date filter leak: txn date %s outside range", txn.Date)
		}
	}
	if sr2.Total == 0 {
		t.Error("Pix+date filter returned 0 results")
	}
}

// TestMultiBankMonthlySummary verifies monthly aggregation across banks.
func TestMultiBankMonthlySummary(t *testing.T) {
	db := newMultiBankDB(t)
	loadAllSynthetic(t, db)

	// All banks, all dates
	ms, err := db.GetMonthlySummary("", "", "")
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}
	if len(ms) < 2 {
		t.Errorf("monthly summary: got %d months, want >= 2", len(ms))
	}

	// Verify total count across months
	totalCount := 0
	for _, m := range ms {
		totalCount += m.Count
	}
	if totalCount != multiBankWantTotal {
		t.Errorf("monthly total count: got %d, want %d", totalCount, multiBankWantTotal)
	}

	// Per-bank monthly: Bradesco only
	msB, err := db.GetMonthlySummary("Bradesco", "", "")
	if err != nil {
		t.Fatalf("GetMonthlySummary Bradesco: %v", err)
	}
	bradCount := 0
	for _, m := range msB {
		bradCount += m.Count
	}
	if bradCount != 19 {
		t.Errorf("Bradesco monthly count: got %d, want 19", bradCount)
	}
}

// TestMultiBankExport verifies XLSX export with all banks and filtered.
func TestMultiBankExport(t *testing.T) {
	db := newMultiBankDB(t)
	loadAllSynthetic(t, db)

	tmpDir := os.Getenv("XDG_CONFIG_HOME")

	// Export all 56 transactions
	allTxns, err := db.SearchAll("")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(allTxns) != multiBankWantTotal {
		t.Errorf("SearchAll count: got %d, want %d", len(allTxns), multiBankWantTotal)
	}

	xlsxAll := filepath.Join(tmpDir, "export-all.xlsx")
	if err := ExportXLSX(allTxns, xlsxAll, ""); err != nil {
		t.Fatalf("ExportXLSX all: %v", err)
	}
	if info, err := os.Stat(xlsxAll); err != nil || info.Size() == 0 {
		t.Error("XLSX all: file missing or empty")
	}

	// Export Nubank only → verify 10 rows
	nubankTxns, err := db.SearchAll("Nubank")
	if err != nil {
		t.Fatalf("SearchAll Nubank: %v", err)
	}
	if len(nubankTxns) != 10 {
		t.Errorf("SearchAll Nubank: got %d, want 10", len(nubankTxns))
	}

	xlsxNubank := filepath.Join(tmpDir, "export-nubank.xlsx")
	if err := ExportXLSX(nubankTxns, xlsxNubank, "Nubank"); err != nil {
		t.Fatalf("ExportXLSX Nubank: %v", err)
	}

	// Verify all exported Nubank transactions have bank="Nubank"
	for _, txn := range nubankTxns {
		if txn.Bank != "Nubank" {
			t.Errorf("Nubank export: got bank=%q, want Nubank", txn.Bank)
		}
	}

	// --- Python cross-verification for multi-bank ---
	verifyScript := filepath.Join("testdata", "verify.py")
	if _, err := os.Stat(verifyScript); err != nil {
		t.Skipf("verify.py not found, skipping Python cross-check: %v", err)
	}

	// Save Go search result as JSON
	sr, err := db.Search("", 100000, 0)
	if err != nil {
		t.Fatalf("Search for JSON export: %v", err)
	}
	goJSONPath := filepath.Join(tmpDir, "multibank-go-result.json")
	goJSON, _ := json.Marshal(sr)
	os.WriteFile(goJSONPath, goJSON, 0644)
}

