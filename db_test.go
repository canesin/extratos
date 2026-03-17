package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB creates a DB in a temp directory for testing.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigration(t *testing.T) {
	db := newTestDB(t)

	// Verify tables exist
	var name string
	err := db.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='transactions'`).Scan(&name)
	if err != nil {
		t.Fatalf("transactions table not found: %v", err)
	}

	// Verify FTS5 virtual table
	err = db.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='transactions_fts'`).Scan(&name)
	if err != nil {
		t.Fatalf("transactions_fts table not found: %v", err)
	}

	// Verify triggers
	rows, err := db.conn.Query(`SELECT name FROM sqlite_master WHERE type='trigger' ORDER BY name`)
	if err != nil {
		t.Fatalf("query triggers: %v", err)
	}
	defer rows.Close()
	var triggers []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		triggers = append(triggers, n)
	}
	expected := []string{"txn_ad", "txn_ai", "txn_au"}
	if len(triggers) != len(expected) {
		t.Fatalf("expected %d triggers, got %d: %v", len(expected), len(triggers), triggers)
	}

	// Verify schema version
	var ver int
	db.conn.QueryRow(`PRAGMA user_version`).Scan(&ver)
	if ver != schemaVersion {
		t.Errorf("expected schema version %d, got %d", schemaVersion, ver)
	}
}

func TestMigrationIdempotent(t *testing.T) {
	db := newTestDB(t)
	// Run migration again — should not fail
	if err := db.migrate(); err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}
}

func TestMigrationRebuildsOnVersionBump(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	// Simulate old schema version
	db.conn.Exec(`PRAGMA user_version = 0`)

	// Re-migrate should rebuild FTS
	if err := db.migrate(); err != nil {
		t.Fatalf("migrate after version bump: %v", err)
	}

	// FTS should still work
	result, err := db.Search("Fernanda", 10, 0)
	if err != nil {
		t.Fatalf("search after rebuild: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 result after rebuild, got %d", result.Total)
	}
}

func ptr(v float64) *float64 { return &v }

func sampleTransactions() []Transaction {
	return []Transaction{
		{
			Date: "2026-01-05", Description: "Transfe Pix | Des: Fernanda Correa da si 05/01",
			Doc: "0321351", Credit: nil, Debit: ptr(-1500.00), Balance: ptr(1.00),
			Amount: ptr(-1500.00), Account: "Ag 3841 / 134175-8", Bank: "Bradesco", SourceFile: "Bradesco 2026.csv",
		},
		{
			Date: "2026-01-12", Description: "Transfe Pix | Des: Maytê Celina Amarante 10/01",
			Doc: "1233360", Credit: nil, Debit: ptr(-7315.00), Balance: ptr(1.00),
			Amount: ptr(-7315.00), Account: "Ag 3841 / 134175-8", Bank: "Bradesco", SourceFile: "Bradesco 2026.csv",
		},
		{
			Date: "2026-02-06", Description: "Transfe Pix | Rem: Brla Digital Ltda 06/02",
			Doc: "2159129", Credit: ptr(2103576.88), Debit: nil, Balance: ptr(2103577.88),
			Amount: ptr(2103576.88), Account: "Ag 3841 / 134175-8", Bank: "Bradesco", SourceFile: "Bradesco 2026.csv",
		},
	}
}

func TestInsertTransactions(t *testing.T) {
	db := newTestDB(t)
	txns := sampleTransactions()

	inserted, err := db.InsertTransactions(txns)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if inserted != 3 {
		t.Errorf("expected 3 inserted, got %d", inserted)
	}

	// Verify count
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 rows, got %d", count)
	}

	// Verify search_text is populated
	var st string
	db.conn.QueryRow(`SELECT search_text FROM transactions WHERE id = 1`).Scan(&st)
	if st == "" {
		t.Error("search_text is empty")
	}
	// Should be accent-stripped and lowercased
	if st != buildSearchText(txns[0].Description, txns[0].Account, txns[0].Bank) {
		t.Errorf("unexpected search_text: %q", st)
	}
}

func TestInsertDedup(t *testing.T) {
	db := newTestDB(t)
	txns := sampleTransactions()

	inserted1, _ := db.InsertTransactions(txns)
	inserted2, _ := db.InsertTransactions(txns)

	if inserted1 != 3 {
		t.Errorf("first insert: expected 3, got %d", inserted1)
	}
	if inserted2 != 0 {
		t.Errorf("second insert (dedup): expected 0, got %d", inserted2)
	}
}

func TestSearchEmpty(t *testing.T) {
	db := newTestDB(t)

	result, err := db.Search("", 10, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
	if len(result.Transactions) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(result.Transactions))
	}
}

func TestSearchAll(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	result, err := db.Search("", 10, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("expected 3 total, got %d", result.Total)
	}
	if len(result.Transactions) != 3 {
		t.Errorf("expected 3 transactions, got %d", len(result.Transactions))
	}
}

func TestSearchFTS(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	tests := []struct {
		query    string
		expected int
	}{
		{"Fernanda", 1},
		{"fernanda", 1},
		{"Maytê", 1},
		{"Mayte", 1}, // accent-insensitive!
		{"mayte", 1}, // case + accent insensitive
		{"Brla", 1},
		{"Transfe Pix", 3},
		{"nonexistent", 0},
		{"134175", 3},   // account number search
		{"Bradesco", 3}, // bank name
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result, err := db.Search(tt.query, 100, 0)
			if err != nil {
				t.Fatalf("search %q: %v", tt.query, err)
			}
			if result.Total != tt.expected {
				t.Errorf("search %q: expected %d results, got %d", tt.query, tt.expected, result.Total)
			}
		})
	}
}

func TestSearchMultiTerm(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	tests := []struct {
		query    string
		expected int
	}{
		{"Mayte, Correa", 2},         // OR: Maytê + Fernanda Correa
		{"Fernanda, Mayte", 2},       // same two people
		{"Fernanda, Mayte, Brla", 3}, // all three
		{"nonexistent, Brla", 1},     // one match
		{"nonexistent, nope", 0},     // no matches
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result, err := db.Search(tt.query, 100, 0)
			if err != nil {
				t.Fatalf("search %q: %v", tt.query, err)
			}
			if result.Total != tt.expected {
				t.Errorf("search %q: expected %d results, got %d", tt.query, tt.expected, result.Total)
			}
		})
	}
}

func TestSearchSummary(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	result, err := db.Search("", 10, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if result.TotalCredit != 2103576.88 {
		t.Errorf("total credit: expected 2103576.88, got %f", result.TotalCredit)
	}
	if result.TotalDebit != -8815.00 {
		t.Errorf("total debit: expected -8815.00, got %f", result.TotalDebit)
	}
	if result.MinDate != "2026-01-05" {
		t.Errorf("min date: expected 2026-01-05, got %s", result.MinDate)
	}
	if result.MaxDate != "2026-02-06" {
		t.Errorf("max date: expected 2026-02-06, got %s", result.MaxDate)
	}

	// Filtered summary
	r2, _ := db.Search("Fernanda", 10, 0)
	if r2.TotalDebit != -1500.00 {
		t.Errorf("filtered debit: expected -1500.00, got %f", r2.TotalDebit)
	}
}

func TestSearchPagination(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	// Page 1: limit 2
	r1, _ := db.Search("", 2, 0)
	if len(r1.Transactions) != 2 {
		t.Errorf("page 1: expected 2 transactions, got %d", len(r1.Transactions))
	}
	if r1.Total != 3 {
		t.Errorf("page 1: expected total 3, got %d", r1.Total)
	}

	// Page 2: limit 2, offset 2
	r2, _ := db.Search("", 2, 2)
	if len(r2.Transactions) != 1 {
		t.Errorf("page 2: expected 1 transaction, got %d", len(r2.Transactions))
	}
}

func TestSearchAll_DB(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	txns, err := db.SearchAll("Fernanda", "")
	if err != nil {
		t.Fatalf("searchAll: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("expected 1 result, got %d", len(txns))
	}

	all, _ := db.SearchAll("", "")
	if len(all) != 3 {
		t.Errorf("expected 3 results, got %d", len(all))
	}
}

func TestGetStats(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("getStats: %v", err)
	}

	if stats["total_transactions"] != 3 {
		t.Errorf("expected 3 total, got %v", stats["total_transactions"])
	}
	if stats["min_date"] != "2026-01-05" {
		t.Errorf("expected min_date 2026-01-05, got %v", stats["min_date"])
	}
	if stats["max_date"] != "2026-02-06" {
		t.Errorf("expected max_date 2026-02-06, got %v", stats["max_date"])
	}

	banks := stats["banks"].([]string)
	if len(banks) != 1 || banks[0] != "Bradesco" {
		t.Errorf("expected [Bradesco], got %v", banks)
	}
}

func TestNewDB_RealPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "extratos.db")

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	db := &DB{conn: conn}
	err = db.migrate()
	if err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	db.Close()

	// Re-open and verify
	conn2, _ := sql.Open("sqlite", dbPath)
	var count int
	conn2.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='transactions'`).Scan(&count)
	if count != 1 {
		t.Error("transactions table missing after re-open")
	}
	conn2.Close()

	// Verify db file exists on disk
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("db file not created")
	}
}

func TestSearchClauseSummaries(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	// Single term: no clause summaries
	r1, err := db.Search("Fernanda", 100, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(r1.ClauseSummaries) != 0 {
		t.Errorf("single term should have no clause summaries, got %d", len(r1.ClauseSummaries))
	}

	// Multi-term: should have per-clause summaries
	r2, err := db.Search("Fernanda, Mayte", 100, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r2.Total != 2 {
		t.Errorf("expected 2 total results, got %d", r2.Total)
	}
	if len(r2.ClauseSummaries) != 2 {
		t.Fatalf("expected 2 clause summaries, got %d", len(r2.ClauseSummaries))
	}

	// Verify each clause summary
	fernanda := r2.ClauseSummaries[0]
	if fernanda.Clause != "Fernanda" {
		t.Errorf("expected clause 'Fernanda', got %q", fernanda.Clause)
	}
	if fernanda.Total != 1 {
		t.Errorf("Fernanda: expected 1 result, got %d", fernanda.Total)
	}
	if fernanda.TotalDebit != -1500.00 {
		t.Errorf("Fernanda: expected debit -1500, got %f", fernanda.TotalDebit)
	}

	mayte := r2.ClauseSummaries[1]
	if mayte.Clause != "Mayte" {
		t.Errorf("expected clause 'Mayte', got %q", mayte.Clause)
	}
	if mayte.Total != 1 {
		t.Errorf("Mayte: expected 1 result, got %d", mayte.Total)
	}
	if mayte.TotalDebit != -7315.00 {
		t.Errorf("Mayte: expected debit -7315, got %f", mayte.TotalDebit)
	}

	// Three terms
	r3, _ := db.Search("Fernanda, Mayte, Brla", 100, 0)
	if len(r3.ClauseSummaries) != 3 {
		t.Fatalf("expected 3 clause summaries, got %d", len(r3.ClauseSummaries))
	}
	brla := r3.ClauseSummaries[2]
	if brla.Total != 1 {
		t.Errorf("Brla: expected 1 result, got %d", brla.Total)
	}
	if brla.TotalCredit != 2103576.88 {
		t.Errorf("Brla: expected credit 2103576.88, got %f", brla.TotalCredit)
	}
}

func TestListDatabases(t *testing.T) {
	// Save and restore env to use temp dir
	dir := t.TempDir()
	origConfigDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	t.Cleanup(func() { os.Setenv("XDG_CONFIG_HOME", origConfigDir) })

	// Initially empty
	dbs, err := ListDatabases()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dbs) != 0 {
		t.Errorf("expected 0 databases, got %d", len(dbs))
	}

	// Create a DB
	db, err := OpenNamedDB("test-empresa")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	db.Close()

	// Should now list it
	dbs, _ = ListDatabases()
	if len(dbs) != 1 {
		t.Fatalf("expected 1 database, got %d", len(dbs))
	}
	if dbs[0].Name != "test-empresa" {
		t.Errorf("expected name 'test-empresa', got %q", dbs[0].Name)
	}

	// Create another
	db2, _ := OpenNamedDB("pessoal")
	db2.Close()

	dbs, _ = ListDatabases()
	if len(dbs) != 2 {
		t.Errorf("expected 2 databases, got %d", len(dbs))
	}
}

func TestBuildFilePreview(t *testing.T) {
	txns := sampleTransactions()
	fp := buildFilePreview(txns, "test.csv", "Bradesco")

	if fp.Filename != "test.csv" {
		t.Errorf("filename: expected test.csv, got %q", fp.Filename)
	}
	if fp.Bank != "Bradesco" {
		t.Errorf("bank: expected Bradesco, got %q", fp.Bank)
	}
	if fp.Count != 3 {
		t.Errorf("count: expected 3, got %d", fp.Count)
	}
	if fp.TotalCredit != 2103576.88 {
		t.Errorf("total credit: expected 2103576.88, got %f", fp.TotalCredit)
	}
	if fp.TotalDebit != -8815.00 {
		t.Errorf("total debit: expected -8815.00, got %f", fp.TotalDebit)
	}
	if fp.DateRange != "2026-01-05 a 2026-02-06" {
		t.Errorf("date range: expected '2026-01-05 a 2026-02-06', got %q", fp.DateRange)
	}
	if fp.Account == "" {
		t.Error("account should not be empty")
	}

	// Empty transactions
	fp2 := buildFilePreview(nil, "empty.csv", "Itau")
	if fp2.Count != 0 {
		t.Errorf("empty: expected count 0, got %d", fp2.Count)
	}
}

func TestSearchFiltered_DateRange(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	// No date filter — should return all 3
	r1, err := db.SearchFiltered("", 100, 0, "", "", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r1.Total != 3 {
		t.Errorf("no filter: expected 3 total, got %d", r1.Total)
	}

	// dateFrom only — from 2026-01-12 onwards should include 2 transactions
	r2, err := db.SearchFiltered("", 100, 0, "2026-01-12", "", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r2.Total != 2 {
		t.Errorf("dateFrom 2026-01-12: expected 2 total, got %d", r2.Total)
	}

	// dateTo only — up to 2026-01-12 should include 2 transactions
	r3, err := db.SearchFiltered("", 100, 0, "", "2026-01-12", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r3.Total != 2 {
		t.Errorf("dateTo 2026-01-12: expected 2 total, got %d", r3.Total)
	}

	// Both dates — exactly Jan 2026
	r4, err := db.SearchFiltered("", 100, 0, "2026-01-01", "2026-01-31", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r4.Total != 2 {
		t.Errorf("Jan 2026: expected 2 total, got %d", r4.Total)
	}
	// Aggregates should only include the 2 January transactions
	if r4.TotalCredit != 0 {
		t.Errorf("Jan 2026 credit: expected 0, got %f", r4.TotalCredit)
	}
	if r4.TotalDebit != -8815.00 {
		t.Errorf("Jan 2026 debit: expected -8815, got %f", r4.TotalDebit)
	}

	// Date filter + FTS query
	r5, err := db.SearchFiltered("Fernanda", 100, 0, "2026-01-01", "2026-01-31", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r5.Total != 1 {
		t.Errorf("Fernanda in Jan: expected 1 total, got %d", r5.Total)
	}

	// Date filter that excludes all results
	r6, err := db.SearchFiltered("Fernanda", 100, 0, "2026-03-01", "2026-03-31", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r6.Total != 0 {
		t.Errorf("Fernanda in Mar: expected 0 total, got %d", r6.Total)
	}

	// Multi-term with date filter — clause summaries should also be filtered
	r7, err := db.SearchFiltered("Fernanda, Brla", 100, 0, "2026-01-01", "2026-01-31", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r7.Total != 1 {
		t.Errorf("Fernanda,Brla in Jan: expected 1 total, got %d", r7.Total)
	}
	if len(r7.ClauseSummaries) != 2 {
		t.Fatalf("expected 2 clause summaries, got %d", len(r7.ClauseSummaries))
	}
	// Fernanda clause should have 1 result
	if r7.ClauseSummaries[0].Total != 1 {
		t.Errorf("Fernanda clause in Jan: expected 1, got %d", r7.ClauseSummaries[0].Total)
	}
	// Brla clause should have 0 results (it's in Feb)
	if r7.ClauseSummaries[1].Total != 0 {
		t.Errorf("Brla clause in Jan: expected 0, got %d", r7.ClauseSummaries[1].Total)
	}
}

func TestGetMonthlySummary(t *testing.T) {
	db := newTestDB(t)
	db.InsertTransactions(sampleTransactions())

	// All months, no filter
	summaries, err := db.GetMonthlySummary("", "", "", "")
	if err != nil {
		t.Fatalf("monthly summary: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 months, got %d", len(summaries))
	}

	// Results ordered DESC by month
	if summaries[0].Month != "2026-02" {
		t.Errorf("first month: expected 2026-02, got %s", summaries[0].Month)
	}
	if summaries[1].Month != "2026-01" {
		t.Errorf("second month: expected 2026-01, got %s", summaries[1].Month)
	}

	// Feb: 1 transaction (Brla credit)
	if summaries[0].Count != 1 {
		t.Errorf("Feb count: expected 1, got %d", summaries[0].Count)
	}
	if summaries[0].TotalCredit != 2103576.88 {
		t.Errorf("Feb credit: expected 2103576.88, got %f", summaries[0].TotalCredit)
	}

	// Jan: 2 transactions (both debits)
	if summaries[1].Count != 2 {
		t.Errorf("Jan count: expected 2, got %d", summaries[1].Count)
	}
	if summaries[1].TotalDebit != -8815.00 {
		t.Errorf("Jan debit: expected -8815, got %f", summaries[1].TotalDebit)
	}

	// With FTS query
	summaries2, err := db.GetMonthlySummary("Fernanda", "", "", "")
	if err != nil {
		t.Fatalf("monthly summary with query: %v", err)
	}
	if len(summaries2) != 1 {
		t.Fatalf("expected 1 month for Fernanda, got %d", len(summaries2))
	}
	if summaries2[0].Month != "2026-01" {
		t.Errorf("Fernanda month: expected 2026-01, got %s", summaries2[0].Month)
	}

	// With date filter
	summaries3, err := db.GetMonthlySummary("", "2026-02-01", "2026-02-28", "")
	if err != nil {
		t.Fatalf("monthly summary with dates: %v", err)
	}
	if len(summaries3) != 1 {
		t.Fatalf("expected 1 month for Feb filter, got %d", len(summaries3))
	}
	if summaries3[0].Month != "2026-02" {
		t.Errorf("Feb filter month: expected 2026-02, got %s", summaries3[0].Month)
	}
}

func TestBuildFTSQuery(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Mayte", `"mayte"`},
		{"Maytê", `"mayte"`}, // accent stripped
		{"Mayte, Correa", `"mayte" OR "correa"`},
		{"  Mayte ,  Correa  ", `"mayte" OR "correa"`},
		{",,,", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := buildFTSQuery(tt.input)
			if got != tt.expected {
				t.Errorf("buildFTSQuery(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInsertInternalTransactions(t *testing.T) {
	db := newTestDB(t)
	txns := []Transaction{
		{
			Date: "2026-01-05", Description: "Resgate Inv Programado",
			Credit: ptr(1000.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
		{
			Date: "2026-01-06", Description: "Apl.invest Automatico",
			Debit: ptr(-500.00), Amount: ptr(-500.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
		{
			Date: "2026-01-07", Description: "Transfe Pix | Des: Fulano",
			Debit: ptr(-200.00), Amount: ptr(-200.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
	}
	db.InsertTransactions(txns)

	// Verify is_internal flags
	var isInt int
	db.conn.QueryRow(`SELECT is_internal FROM transactions WHERE description LIKE 'Resgate%'`).Scan(&isInt)
	if isInt != 1 {
		t.Errorf("Resgate should be internal, got %d", isInt)
	}
	db.conn.QueryRow(`SELECT is_internal FROM transactions WHERE description LIKE 'Apl.invest%'`).Scan(&isInt)
	if isInt != 1 {
		t.Errorf("Apl.invest should be internal, got %d", isInt)
	}
	db.conn.QueryRow(`SELECT is_internal FROM transactions WHERE description LIKE 'Transfe%'`).Scan(&isInt)
	if isInt != 0 {
		t.Errorf("Transfe Pix should NOT be internal, got %d", isInt)
	}

	// Verify IsInternal field on returned transaction
	result, _ := db.Search("", 100, 0)
	var foundInternal, foundExternal bool
	for _, tx := range result.Transactions {
		if tx.IsInternal {
			foundInternal = true
		} else {
			foundExternal = true
		}
	}
	if !foundInternal {
		t.Error("expected at least one internal transaction in results")
	}
	if !foundExternal {
		t.Error("expected at least one external transaction in results")
	}
}

func TestToggleInternal(t *testing.T) {
	db := newTestDB(t)
	txns := []Transaction{
		{
			Date: "2026-01-05", Description: "Transfe Pix | Normal",
			Debit: ptr(-100.00), Amount: ptr(-100.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
	}
	db.InsertTransactions(txns)

	// Should start as external
	var isInt int
	db.conn.QueryRow(`SELECT is_internal FROM transactions WHERE id = 1`).Scan(&isInt)
	if isInt != 0 {
		t.Errorf("should start as external, got %d", isInt)
	}

	// Toggle to internal
	if err := db.ToggleInternal(1); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	db.conn.QueryRow(`SELECT is_internal FROM transactions WHERE id = 1`).Scan(&isInt)
	if isInt != 1 {
		t.Errorf("should be internal after toggle, got %d", isInt)
	}

	// Toggle back
	if err := db.ToggleInternal(1); err != nil {
		t.Fatalf("toggle back: %v", err)
	}
	db.conn.QueryRow(`SELECT is_internal FROM transactions WHERE id = 1`).Scan(&isInt)
	if isInt != 0 {
		t.Errorf("should be external after second toggle, got %d", isInt)
	}
}

func TestSearchFilteredInternal(t *testing.T) {
	db := newTestDB(t)
	txns := []Transaction{
		{
			Date: "2026-01-05", Description: "Resgate Inv Programado",
			Credit: ptr(1000.00), Amount: ptr(1000.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
		{
			Date: "2026-01-06", Description: "Apl.invest Automatico",
			Debit: ptr(-500.00), Amount: ptr(-500.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
		{
			Date: "2026-01-07", Description: "Transfe Pix | Des: Fulano",
			Debit: ptr(-200.00), Amount: ptr(-200.00), Account: "123", Bank: "Bradesco", SourceFile: "test.csv",
		},
	}
	db.InsertTransactions(txns)

	// All transactions
	rAll, _ := db.SearchFiltered("", 100, 0, "", "", "")
	if rAll.Total != 3 {
		t.Errorf("all: expected 3, got %d", rAll.Total)
	}
	if rAll.InternalCount != 2 {
		t.Errorf("internal count: expected 2, got %d", rAll.InternalCount)
	}

	// External only
	rExt, _ := db.SearchFiltered("", 100, 0, "", "", "external")
	if rExt.Total != 1 {
		t.Errorf("external: expected 1, got %d", rExt.Total)
	}
	if rExt.Transactions[0].Description != "Transfe Pix | Des: Fulano" {
		t.Errorf("external: wrong transaction: %s", rExt.Transactions[0].Description)
	}

	// Internal only
	rInt, _ := db.SearchFiltered("", 100, 0, "", "", "internal")
	if rInt.Total != 2 {
		t.Errorf("internal: expected 2, got %d", rInt.Total)
	}
	for _, tx := range rInt.Transactions {
		if !tx.IsInternal {
			t.Errorf("internal filter returned non-internal: %s", tx.Description)
		}
	}

	// Aggregates: external filter should only sum external transactions
	if rExt.TotalDebit != -200.00 {
		t.Errorf("external debit: expected -200, got %f", rExt.TotalDebit)
	}
	if rExt.TotalCredit != 0 {
		t.Errorf("external credit: expected 0, got %f", rExt.TotalCredit)
	}

	// Aggregates: internal filter should sum internal transactions
	if rInt.TotalCredit != 1000.00 {
		t.Errorf("internal credit: expected 1000, got %f", rInt.TotalCredit)
	}
	if rInt.TotalDebit != -500.00 {
		t.Errorf("internal debit: expected -500, got %f", rInt.TotalDebit)
	}
	if rInt.NetAmount != 500.00 {
		t.Errorf("internal net: expected 500, got %f", rInt.NetAmount)
	}
}

func TestBackfillIsInternal(t *testing.T) {
	db := newTestDB(t)

	// Insert transactions bypassing IsInternalTransfer (simulating old schema)
	tx, _ := db.conn.Begin()
	stmt, _ := tx.Prepare(`INSERT INTO transactions
		(date, description, doc, credit, debit, balance, amount, account, bank, source_file, import_hash, search_text, is_internal)
		VALUES (?, ?, '', ?, ?, ?, ?, '123', 'Bradesco', 'test.csv', ?, '', 0)`)
	stmt.Exec("2026-01-05", "Resgate Inv Programado", 1000.0, nil, nil, 1000.0, "hash1")
	stmt.Exec("2026-01-06", "Apl.invest Auto", nil, -500.0, nil, -500.0, "hash2")
	stmt.Exec("2026-01-07", "Transfe Pix Normal", nil, -200.0, nil, -200.0, "hash3")
	stmt.Close()
	tx.Commit()

	// All should be is_internal=0
	var count int
	db.conn.QueryRow(`SELECT COUNT(*) FROM transactions WHERE is_internal = 1`).Scan(&count)
	if count != 0 {
		t.Errorf("before backfill: expected 0 internal, got %d", count)
	}

	// Run backfill
	db.backfillIsInternal()

	db.conn.QueryRow(`SELECT COUNT(*) FROM transactions WHERE is_internal = 1`).Scan(&count)
	if count != 2 {
		t.Errorf("after backfill: expected 2 internal, got %d", count)
	}

	db.conn.QueryRow(`SELECT COUNT(*) FROM transactions WHERE is_internal = 0`).Scan(&count)
	if count != 1 {
		t.Errorf("after backfill: expected 1 external, got %d", count)
	}
}
