package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// schemaVersion is incremented whenever the DB schema or FTS indexing changes.
// Bumping this triggers an automatic FTS rebuild on next startup.
const schemaVersion = 2

// externalFilter is the SQL condition excluding internal banking movements
// (investment applications/redemptions) from aggregate sums.
// Covers Bradesco, Itaú, Banco do Brasil, Caixa, and Nubank patterns.
const externalFilter = `description NOT LIKE 'Resgate Inv%' ` +
	`AND description NOT LIKE 'Resg.autom%' ` +
	`AND description NOT LIKE 'Resg/vencto%' ` +
	`AND description NOT LIKE 'Rent.inv%' ` +
	`AND description NOT LIKE 'Rentab.invest%' ` +
	`AND description NOT LIKE 'Apl.invest%' ` +
	`AND description NOT LIKE 'Aplicacao Cdb%' ` +
	`AND description NOT LIKE 'Aplicacao Inve%' ` +
	`AND description NOT LIKE 'REND PAGO APLIC AUT%' ` +
	`AND description NOT LIKE 'RESGATE CDB%' ` +
	`AND description NOT LIKE 'APLICACAO CDB%' ` +
	// Banco do Brasil
	`AND description NOT LIKE 'APLICACAO POUPANCA%' ` +
	`AND description NOT LIKE 'RESGATE POUPANCA%' ` +
	`AND description NOT LIKE 'APLICACAO FUNDOS%' ` +
	`AND description NOT LIKE 'RESGATE FUNDOS%' ` +
	// Caixa
	`AND description NOT LIKE 'APL POUP%' ` +
	`AND description NOT LIKE 'RES POUP%' ` +
	// Nubank
	`AND description NOT LIKE 'Aplicação RDB%' ` +
	`AND description NOT LIKE 'Resgate RDB%'`

// aggCols is the SELECT columns for aggregate queries,
// counting all rows but summing only external (non-internal) transactions.
var aggCols = fmt.Sprintf(`COUNT(*),
	COALESCE(SUM(CASE WHEN %[1]s THEN credit END), 0),
	COALESCE(SUM(CASE WHEN %[1]s THEN debit END), 0),
	COALESCE(SUM(CASE WHEN %[1]s THEN amount END), 0),
	COALESCE(MIN(date),''),
	COALESCE(MAX(date),'')`, externalFilter)

// internalPrefixes lists description prefixes for internal banking movements.
// Covers Bradesco, Itaú, Banco do Brasil, Caixa, and Nubank patterns.
var internalPrefixes = []string{
	// Bradesco
	"Resgate Inv",
	"Resg.autom",
	"Resg/vencto",
	"Rent.inv",
	"Rentab.invest",
	"Apl.invest",
	"Aplicacao Cdb",
	"Aplicacao Inve",
	// Itaú
	"REND PAGO APLIC AUT",
	"RESGATE CDB",
	"APLICACAO CDB",
	// Banco do Brasil
	"APLICACAO POUPANCA",
	"RESGATE POUPANCA",
	"APLICACAO FUNDOS",
	"RESGATE FUNDOS",
	// Caixa
	"APL POUP",
	"RES POUP",
	// Nubank
	"Aplicação RDB",
	"Resgate RDB",
}

// IsInternalTransfer returns true if the description indicates an internal
// banking movement (investment application or redemption).
func IsInternalTransfer(desc string) bool {
	for _, p := range internalPrefixes {
		if strings.HasPrefix(desc, p) {
			return true
		}
	}
	return false
}

type DB struct {
	conn *sql.DB
}

// DBInfo describes a database file available for selection.
type DBInfo struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"size_bytes"`
	ModifiedAt string `json:"modified_at"`
}

// ClauseSummary holds aggregate stats for a single search clause.
type ClauseSummary struct {
	Clause      string  `json:"clause"`
	Total       int     `json:"total"`
	TotalCredit float64 `json:"total_credit"`
	TotalDebit  float64 `json:"total_debit"`
	NetAmount   float64 `json:"net_amount"`
	MinDate     string  `json:"min_date"`
	MaxDate     string  `json:"max_date"`
}

// FilePreview holds parsed data for a single file before import confirmation.
type FilePreview struct {
	Filename     string        `json:"filename"`
	Bank         string        `json:"bank"`
	Account      string        `json:"account"`
	DateRange    string        `json:"date_range"`
	Transactions []Transaction `json:"transactions"`
	TotalCredit  float64       `json:"total_credit"`
	TotalDebit   float64       `json:"total_debit"`
	Count        int           `json:"count"`
	Error        string        `json:"error,omitempty"`
}

// ImportPreview holds all file previews for a pending import.
type ImportPreview struct {
	Files []FilePreview `json:"files"`
	Error string        `json:"error,omitempty"`
}

// getDBDir returns the directory where all databases are stored.
func getDBDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	dbDir := filepath.Join(configDir, "extratos-app")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return "", fmt.Errorf("create db dir: %w", err)
	}
	return dbDir, nil
}

// ListDatabases scans the config directory for .db files.
func ListDatabases() ([]DBInfo, error) {
	dbDir, err := getDBDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dbDir)
	if err != nil {
		return nil, err
	}

	var dbs []DBInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".db")
		dbs = append(dbs, DBInfo{
			Name:       name,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().Format("2006-01-02 15:04"),
		})
	}
	return dbs, nil
}

// OpenNamedDB opens (or creates) a named database.
func OpenNamedDB(name string) (*DB, error) {
	dbDir, err := getDBDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, name+".db")

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// NewDB opens the default database (for tests / backward compat).
func NewDB() (*DB, error) {
	return OpenNamedDB("extratos")
}

func (db *DB) migrate() error {
	// Core table
	if _, err := db.conn.Exec(`CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date TEXT NOT NULL,
		description TEXT NOT NULL,
		doc TEXT,
		credit REAL,
		debit REAL,
		balance REAL,
		amount REAL,
		account TEXT,
		bank TEXT NOT NULL,
		source_file TEXT NOT NULL,
		import_hash TEXT NOT NULL,
		search_text TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	if _, err := db.conn.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_txn_hash ON transactions(import_hash)`); err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	// Add search_text column if missing (existing DBs from older schema)
	if !db.hasColumn("transactions", "search_text") {
		if _, err := db.conn.Exec(`ALTER TABLE transactions ADD COLUMN search_text TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add search_text column: %w", err)
		}
	}

	// Check schema version — rebuild FTS if schema changed
	var currentVersion int
	db.conn.QueryRow(`PRAGMA user_version`).Scan(&currentVersion)
	needsRebuild := currentVersion < schemaVersion

	// Populate search_text for any rows that have it empty
	db.populateSearchText()

	// Drop and recreate FTS + triggers (safe — derived data)
	db.conn.Exec(`DROP TRIGGER IF EXISTS txn_ai`)
	db.conn.Exec(`DROP TRIGGER IF EXISTS txn_ad`)
	db.conn.Exec(`DROP TRIGGER IF EXISTS txn_au`)
	if needsRebuild {
		db.conn.Exec(`DROP TABLE IF EXISTS transactions_fts`)
	}

	if _, err := db.conn.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS transactions_fts USING fts5(
		search_text,
		content='transactions',
		content_rowid='id',
		tokenize='trigram'
	)`); err != nil {
		return fmt.Errorf("create FTS: %w", err)
	}

	// Rebuild FTS index from existing data if schema changed
	if needsRebuild {
		if _, err := db.conn.Exec(`INSERT INTO transactions_fts(transactions_fts) VALUES('rebuild')`); err != nil {
			return fmt.Errorf("rebuild FTS: %w", err)
		}
		db.conn.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion))
	}

	// Triggers to keep FTS in sync
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS txn_ai AFTER INSERT ON transactions BEGIN
			INSERT INTO transactions_fts(rowid, search_text)
			VALUES (new.id, new.search_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS txn_ad AFTER DELETE ON transactions BEGIN
			INSERT INTO transactions_fts(transactions_fts, rowid, search_text)
			VALUES ('delete', old.id, old.search_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS txn_au AFTER UPDATE ON transactions BEGIN
			INSERT INTO transactions_fts(transactions_fts, rowid, search_text)
			VALUES ('delete', old.id, old.search_text);
			INSERT INTO transactions_fts(rowid, search_text)
			VALUES (new.id, new.search_text);
		END`,
	}
	for _, s := range triggers {
		if _, err := db.conn.Exec(s); err != nil {
			return fmt.Errorf("create trigger: %w", err)
		}
	}

	return nil
}

func (db *DB) hasColumn(table, column string) bool {
	rows, err := db.conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

func (db *DB) populateSearchText() {
	rows, err := db.conn.Query(`SELECT id, description, account, bank FROM transactions WHERE search_text = ''`)
	if err != nil || rows == nil {
		return
	}
	defer rows.Close()

	tx, err := db.conn.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE transactions SET search_text = ? WHERE id = ?`)
	if err != nil {
		return
	}
	defer stmt.Close()

	for rows.Next() {
		var id int64
		var desc, acct, bank string
		rows.Scan(&id, &desc, &acct, &bank)
		stmt.Exec(buildSearchText(desc, acct, bank), id)
	}
	tx.Commit()
}

type Transaction struct {
	ID          int64    `json:"id"`
	Date        string   `json:"date"`
	Description string   `json:"description"`
	Doc         string   `json:"doc"`
	Credit      *float64 `json:"credit"`
	Debit       *float64 `json:"debit"`
	Balance     *float64 `json:"balance"`
	Amount      *float64 `json:"amount"`
	Account     string   `json:"account"`
	Bank        string   `json:"bank"`
	SourceFile  string   `json:"source_file"`
}

type SearchResult struct {
	Transactions    []Transaction   `json:"transactions"`
	Total           int             `json:"total"`
	TotalCredit     float64         `json:"total_credit"`
	TotalDebit      float64         `json:"total_debit"`
	NetAmount       float64         `json:"net_amount"`
	MinDate         string          `json:"min_date"`
	MaxDate         string          `json:"max_date"`
	ClauseSummaries []ClauseSummary `json:"clause_summaries,omitempty"`
}

func (db *DB) InsertTransactions(txns []Transaction) (int, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO transactions
		(date, description, doc, credit, debit, balance, amount, account, bank, source_file, import_hash, search_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	inserted := 0
	for _, t := range txns {
		hash := fmt.Sprintf("%s|%s|%s|%v|%v|%s|%s",
			t.Date, t.Description, t.Doc, ptrVal(t.Credit), ptrVal(t.Debit), t.Account, t.Bank)
		searchText := buildSearchText(t.Description, t.Account, t.Bank)
		res, err := stmt.Exec(t.Date, t.Description, t.Doc, t.Credit, t.Debit, t.Balance, t.Amount, t.Account, t.Bank, t.SourceFile, hash, searchText)
		if err != nil {
			return inserted, err
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}
	return inserted, tx.Commit()
}

func ptrVal(p *float64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%.2f", *p)
}

// buildFTSQuery normalizes and builds an FTS5 query from user input.
// Supports comma-separated terms for OR matching: "Mayte, Correa" → "mayte" OR "correa"
func buildFTSQuery(query string) string {
	query = normalizeText(query)
	parts := strings.Split(query, ",")
	var terms []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.ReplaceAll(p, `"`, `""`)
		terms = append(terms, `"`+p+`"`)
	}
	if len(terms) == 0 {
		return ""
	}
	return strings.Join(terms, " OR ")
}

// splitQueryClauses returns individual normalized terms from a comma-separated query.
func splitQueryClauses(query string) []string {
	parts := strings.Split(query, ",")
	var clauses []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clauses = append(clauses, p)
		}
	}
	return clauses
}

// buildSingleFTSClause builds an FTS5 MATCH expression for a single term.
func buildSingleFTSClause(term string) string {
	normalized := normalizeText(term)
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, `"`, `""`)
	return `"` + normalized + `"`
}

func (db *DB) Search(query string, limit, offset int) (*SearchResult, error) {
	if limit <= 0 {
		limit = 100
	}

	result := &SearchResult{Transactions: []Transaction{}}

	if query == "" {
		// No filter — all transactions, but sums exclude internal banking movements
		db.conn.QueryRow(`SELECT ` + aggCols + ` FROM transactions`).
			Scan(&result.Total, &result.TotalCredit, &result.TotalDebit, &result.NetAmount, &result.MinDate, &result.MaxDate)

		rows, err := db.conn.Query(`SELECT id, date, description, doc, credit, debit, balance, amount, account, bank, source_file
			FROM transactions ORDER BY date DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var t Transaction
			if err := rows.Scan(&t.ID, &t.Date, &t.Description, &t.Doc, &t.Credit, &t.Debit, &t.Balance, &t.Amount, &t.Account, &t.Bank, &t.SourceFile); err != nil {
				return nil, err
			}
			result.Transactions = append(result.Transactions, t)
		}
		return result, rows.Err()
	}

	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return result, nil
	}

	// Aggregates over all matching rows, sums exclude internal banking movements
	db.conn.QueryRow(`SELECT `+aggCols+`
		FROM transactions WHERE id IN (SELECT rowid FROM transactions_fts WHERE transactions_fts MATCH ?)`, ftsQuery).
		Scan(&result.Total, &result.TotalCredit, &result.TotalDebit, &result.NetAmount, &result.MinDate, &result.MaxDate)

	// Paginated results
	rows, err := db.conn.Query(`SELECT t.id, t.date, t.description, t.doc, t.credit, t.debit, t.balance, t.amount, t.account, t.bank, t.source_file
		FROM transactions t
		INNER JOIN transactions_fts fts ON t.id = fts.rowid
		WHERE transactions_fts MATCH ?
		ORDER BY t.date DESC, t.id DESC
		LIMIT ? OFFSET ?`, ftsQuery, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Date, &t.Description, &t.Doc, &t.Credit, &t.Debit, &t.Balance, &t.Amount, &t.Account, &t.Bank, &t.SourceFile); err != nil {
			return nil, err
		}
		result.Transactions = append(result.Transactions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Per-clause summaries when there are multiple comma-separated terms
	clauses := splitQueryClauses(query)
	if len(clauses) > 1 {
		for _, clause := range clauses {
			fts := buildSingleFTSClause(clause)
			if fts == "" {
				continue
			}
			var cs ClauseSummary
			cs.Clause = clause
			db.conn.QueryRow(`SELECT `+aggCols+`
				FROM transactions WHERE id IN (SELECT rowid FROM transactions_fts WHERE transactions_fts MATCH ?)`, fts).
				Scan(&cs.Total, &cs.TotalCredit, &cs.TotalDebit, &cs.NetAmount, &cs.MinDate, &cs.MaxDate)
			result.ClauseSummaries = append(result.ClauseSummaries, cs)
		}
	}

	return result, nil
}

func (db *DB) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var total int
	db.conn.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&total)
	stats["total_transactions"] = total

	var banks []string
	rows, err := db.conn.Query(`SELECT DISTINCT bank FROM transactions ORDER BY bank`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var b string
			rows.Scan(&b)
			banks = append(banks, b)
		}
	}
	stats["banks"] = banks

	var accounts []string
	rows2, err := db.conn.Query(`SELECT DISTINCT account FROM transactions ORDER BY account`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var a string
			rows2.Scan(&a)
			accounts = append(accounts, a)
		}
	}
	stats["accounts"] = accounts

	var minDate, maxDate string
	db.conn.QueryRow(`SELECT COALESCE(MIN(date),''), COALESCE(MAX(date),'') FROM transactions`).Scan(&minDate, &maxDate)
	stats["min_date"] = minDate
	stats["max_date"] = maxDate

	return stats, nil
}

func (db *DB) SearchAll(query string) ([]Transaction, error) {
	var rows *sql.Rows
	var err error

	if query == "" {
		rows, err = db.conn.Query(`SELECT id, date, description, doc, credit, debit, balance, amount, account, bank, source_file
			FROM transactions ORDER BY date DESC, id DESC`)
	} else {
		ftsQuery := buildFTSQuery(query)
		if ftsQuery == "" {
			return nil, nil
		}
		rows, err = db.conn.Query(`SELECT t.id, t.date, t.description, t.doc, t.credit, t.debit, t.balance, t.amount, t.account, t.bank, t.source_file
			FROM transactions t
			INNER JOIN transactions_fts fts ON t.id = fts.rowid
			WHERE transactions_fts MATCH ?
			ORDER BY t.date DESC, t.id DESC`, ftsQuery)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.Date, &t.Description, &t.Doc, &t.Credit, &t.Debit, &t.Balance, &t.Amount, &t.Account, &t.Bank, &t.SourceFile); err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

func (db *DB) Close() error {
	return db.conn.Close()
}
