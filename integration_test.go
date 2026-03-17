package main

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Expected values for testdata/synthetic_bradesco.csv — computed by hand.
// Internal movements (Resgate Inv Fac, Rent.inv.facil, Apl.invest Fac) are
// excluded from credit/debit/net sums but still counted in total.
//
// All credits:      5000.00 + 1.50 + 150000.00 + 8000.00 + 2.35 + 3000.00 + 25000.00 = 191003.85
// Internal credits: 5000.00 + 1.50 + 8000.00 + 2.35 = 13003.85  (Resgate/Rent.inv)
// External credits: 150000.00 + 3000.00 + 25000.00 = 178000.00
//
// All debits:       -(3500+2500+10000+50000+1250.75+86750.75+3000+5000+250.85+3000+1500+20000) = -186752.35
// Internal debits:  -86750.75  (Apl.invest Fac)
// External debits:  -(3500+2500+10000+50000+1250.75+3000+5000+250.85+3000+1500+20000) = -100001.60
//
// External net:     178000.00 + (-100001.60) = 77998.40
// Count:            19 (all transactions counted, SALDO ANTERIOR + skip sections excluded)
// Dates:            2026-01-02 to 2026-01-20

const (
	wantCount       = 19
	wantTotalCredit = 178000.00
	wantTotalDebit  = -100001.60
	wantNetAmount   = 77998.40
	wantMinDate     = "2026-01-02"
	wantMaxDate     = "2026-01-20"
)

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

// TestSyntheticPipeline runs the full parse→insert→search→export pipeline
// on synthetic anonymized data and verifies aggregates.
func TestSyntheticPipeline(t *testing.T) {
	csvPath := filepath.Join("testdata", "synthetic_bradesco.csv")
	if _, err := os.Stat(csvPath); err != nil {
		t.Fatalf("missing test data: %v", err)
	}

	// --- Step 1: Parse CSV ---
	result, err := ParseFile(csvPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("ParseFile error: %s", result.Error)
	}
	if result.Bank != "Bradesco" {
		t.Errorf("bank: got %q, want Bradesco", result.Bank)
	}
	if len(result.Transactions) != wantCount {
		t.Fatalf("parsed count: got %d, want %d", len(result.Transactions), wantCount)
	}

	// Verify accounts
	accounts := map[string]bool{}
	for _, txn := range result.Transactions {
		accounts[txn.Account] = true
	}
	if !accounts["Ag 1234 / 56789-0"] || !accounts["Ag 5678 / 12345-6"] {
		t.Errorf("accounts: got %v, want both Ag 1234/56789-0 and Ag 5678/12345-6", accounts)
	}

	// --- Step 2: Insert into temp DB ---
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	db, err := OpenNamedDB("integration-test")
	if err != nil {
		t.Fatalf("OpenNamedDB: %v", err)
	}
	defer db.Close()

	inserted, err := db.InsertTransactions(result.Transactions)
	if err != nil {
		t.Fatalf("InsertTransactions: %v", err)
	}
	if inserted != wantCount {
		t.Errorf("inserted: got %d, want %d", inserted, wantCount)
	}

	// --- Step 3: Search all — verify aggregates ---
	sr, err := db.Search("", 100000, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if sr.Total != wantCount {
		t.Errorf("total: got %d, want %d", sr.Total, wantCount)
	}
	if !almostEqual(sr.TotalCredit, wantTotalCredit, 0.01) {
		t.Errorf("total_credit: got %.2f, want %.2f", sr.TotalCredit, wantTotalCredit)
	}
	if !almostEqual(sr.TotalDebit, wantTotalDebit, 0.01) {
		t.Errorf("total_debit: got %.2f, want %.2f", sr.TotalDebit, wantTotalDebit)
	}
	if !almostEqual(sr.NetAmount, wantNetAmount, 0.01) {
		t.Errorf("net_amount: got %.2f, want %.2f", sr.NetAmount, wantNetAmount)
	}
	if sr.MinDate != wantMinDate {
		t.Errorf("min_date: got %q, want %q", sr.MinDate, wantMinDate)
	}
	if sr.MaxDate != wantMaxDate {
		t.Errorf("max_date: got %q, want %q", sr.MaxDate, wantMaxDate)
	}

	// --- Step 4: Export XLSX ---
	xlsxPath := filepath.Join(tmpDir, "test-export.xlsx")
	allTxns, err := db.SearchAll("", "")
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(allTxns) != wantCount {
		t.Errorf("SearchAll count: got %d, want %d", len(allTxns), wantCount)
	}
	if err := ExportXLSX(allTxns, xlsxPath, ""); err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}

	// --- Step 5: Save Go search result as JSON for Python comparison ---
	goJSONPath := filepath.Join(tmpDir, "go-result.json")
	goJSON, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(goJSONPath, goJSON, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// --- Step 6: Run Python verification ---
	verifyScript := filepath.Join("testdata", "verify.py")
	if _, err := os.Stat(verifyScript); err != nil {
		t.Fatalf("missing verify.py: %v", err)
	}

	cmd := exec.Command("python3", verifyScript,
		"--csv", csvPath,
		"--go-json", goJSONPath,
		"--xlsx", xlsxPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Python verification failed: %v", err)
	}
}

// TestSyntheticDedup verifies that importing the same CSV twice doesn't duplicate records.
func TestSyntheticDedup(t *testing.T) {
	csvPath := filepath.Join("testdata", "synthetic_bradesco.csv")

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	db, err := OpenNamedDB("dedup-test")
	if err != nil {
		t.Fatalf("OpenNamedDB: %v", err)
	}
	defer db.Close()

	result, err := ParseFile(csvPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// First import
	inserted1, err := db.InsertTransactions(result.Transactions)
	if err != nil {
		t.Fatalf("InsertTransactions (1st): %v", err)
	}
	if inserted1 != wantCount {
		t.Errorf("first import: got %d, want %d", inserted1, wantCount)
	}

	// Second import — all should be skipped as duplicates
	inserted2, err := db.InsertTransactions(result.Transactions)
	if err != nil {
		t.Fatalf("InsertTransactions (2nd): %v", err)
	}
	if inserted2 != 0 {
		t.Errorf("second import should be 0 duplicates, got %d", inserted2)
	}

	// Verify total count unchanged
	sr, err := db.Search("", 100000, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if sr.Total != wantCount {
		t.Errorf("total after dedup: got %d, want %d", sr.Total, wantCount)
	}
}

// TestSyntheticSearch verifies FTS search on synthetic data with per-clause summaries.
func TestSyntheticSearch(t *testing.T) {
	csvPath := filepath.Join("testdata", "synthetic_bradesco.csv")

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	db, err := OpenNamedDB("search-test")
	if err != nil {
		t.Fatalf("OpenNamedDB: %v", err)
	}
	defer db.Close()

	result, err := ParseFile(csvPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	db.InsertTransactions(result.Transactions)

	// Search for "Ana Costa" — should match transactions with continuation "Ana Costa Lima"
	sr, err := db.Search("Ana Costa", 100, 0)
	if err != nil {
		t.Fatalf("Search Ana Costa: %v", err)
	}
	// Ana Costa appears in: txn 12 (debit -3000), txn 14 (debit -250.85 via Loja Gamma — no),
	// Actually: "Des: Ana Costa Lima 10/01", "Des: Pedro Lima Souza 10/01" (no),
	// "Des: Ana Costa Lima 20/01" (credit 3000 devolution), "Des: Ana Costa Lima 20/01" (debit -3000)
	// So 3 matches: descriptions containing "Ana Costa"
	if sr.Total < 1 {
		t.Errorf("search 'Ana Costa': got %d results, want >= 1", sr.Total)
	}

	// Multi-term OR search with per-clause summaries: "Beta Tecnologia, Delta Importadora"
	sr2, err := db.Search("Beta Tecnologia, Delta Importadora", 100, 0)
	if err != nil {
		t.Fatalf("Search multi-term: %v", err)
	}
	if sr2.Total < 2 {
		t.Errorf("search multi-term: got %d results, want >= 2", sr2.Total)
	}
	if len(sr2.ClauseSummaries) != 2 {
		t.Errorf("clause summaries: got %d, want 2", len(sr2.ClauseSummaries))
	}
}
