package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var validDBName = regexp.MustCompile(`^[a-zA-ZÀ-ÿ0-9 _\-]+$`)

type AppService struct {
	db      *DB
	dbError string
	dbName  string

	// Pending import state
	pendingFiles []FilePreview
}

// ServiceStartup is called by Wails v3 during app startup.
// Does NOT open a DB — the frontend must call OpenDatabase or CreateDatabase.
func (a *AppService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown is called by Wails v3 during app shutdown.
func (a *AppService) ServiceShutdown() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// --- Database selection ---

func (a *AppService) ListDatabases() []DBInfo {
	dbs, err := ListDatabases()
	if err != nil {
		return []DBInfo{}
	}
	// Sort by modification time descending (most recent first)
	sort.Slice(dbs, func(i, j int) bool {
		return dbs[i].ModifiedAt > dbs[j].ModifiedAt
	})
	return dbs
}

func (a *AppService) OpenDatabase(name string) string {
	if a.db != nil {
		a.db.Close()
		a.db = nil
	}
	a.pendingFiles = nil
	db, err := OpenNamedDB(name)
	if err != nil {
		a.dbError = fmt.Sprintf("Erro ao abrir banco de dados: %v", err)
		return a.dbError
	}
	a.db = db
	a.dbName = name
	a.dbError = ""
	return ""
}

func (a *AppService) CreateDatabase(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Nome não pode ser vazio"
	}
	if !validDBName.MatchString(name) {
		return "Nome inválido — use apenas letras, números, espaços, _ ou -"
	}
	// OpenNamedDB creates the file if it doesn't exist
	return a.OpenDatabase(name)
}

func (a *AppService) DeleteDatabase(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Nome não pode ser vazio"
	}
	// Close if it's the currently open database
	if a.dbName == name && a.db != nil {
		a.db.Close()
		a.db = nil
		a.dbName = ""
		a.pendingFiles = nil
	}
	dbDir, err := getDBDir()
	if err != nil {
		return fmt.Sprintf("Erro: %v", err)
	}
	dbPath := filepath.Join(dbDir, name+".db")
	if err := os.Remove(dbPath); err != nil {
		return fmt.Sprintf("Erro ao excluir: %v", err)
	}
	// Also remove WAL/SHM files if they exist
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	return ""
}

func (a *AppService) RenameDatabase(oldName, newName string) string {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return "Nome não pode ser vazio"
	}
	if !validDBName.MatchString(newName) {
		return "Nome inválido — use apenas letras, números, espaços, _ ou -"
	}
	if oldName == newName {
		return ""
	}
	dbDir, err := getDBDir()
	if err != nil {
		return fmt.Sprintf("Erro: %v", err)
	}
	oldPath := filepath.Join(dbDir, oldName+".db")
	newPath := filepath.Join(dbDir, newName+".db")
	// Check if target already exists
	if _, err := os.Stat(newPath); err == nil {
		return "Já existe um banco de dados com esse nome"
	}
	// Close if it's the currently open database
	wasOpen := a.dbName == oldName && a.db != nil
	if wasOpen {
		a.db.Close()
		a.db = nil
		a.dbName = ""
		a.pendingFiles = nil
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Sprintf("Erro ao renomear: %v", err)
	}
	// Also rename WAL/SHM files if they exist
	os.Rename(oldPath+"-wal", newPath+"-wal")
	os.Rename(oldPath+"-shm", newPath+"-shm")
	// Reopen if it was the current database
	if wasOpen {
		return a.OpenDatabase(newName)
	}
	return ""
}

func (a *AppService) GetCurrentDB() string {
	return a.dbName
}

func (a *AppService) GetDBError() string {
	return a.dbError
}

// --- Import preview workflow ---

func (a *AppService) PreviewImport() *ImportPreview {
	if a.db == nil {
		return &ImportPreview{Error: "Banco de dados não aberto"}
	}

	app := application.Get()
	paths, err := app.Dialog.OpenFile().
		SetTitle("Selecionar extratos").
		AddFilter("Extratos (CSV/TXT/XLS)", "*.csv;*.txt;*.xls;*.CSV;*.TXT;*.XLS").
		PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return nil
	}

	var files []FilePreview
	for _, path := range paths {
		result, err := ParseFile(path)
		if err != nil {
			files = append(files, FilePreview{
				Filename: filepath.Base(path),
				Error:    fmt.Sprintf("Erro ao ler: %v", err),
			})
			continue
		}
		if result.Error != "" {
			files = append(files, FilePreview{
				Filename: filepath.Base(path),
				Error:    result.Error,
			})
			continue
		}

		fp := buildFilePreview(result.Transactions, filepath.Base(path), result.Bank)
		files = append(files, fp)
	}

	a.pendingFiles = files
	return &ImportPreview{Files: files}
}

func (a *AppService) ConfirmImport() *ParseResult {
	if a.db == nil {
		return &ParseResult{Error: "Banco de dados não aberto"}
	}
	if len(a.pendingFiles) == 0 {
		return &ParseResult{Error: "Nenhum arquivo pendente para importar"}
	}

	totalInserted := 0
	totalSkipped := 0
	var lastBank string

	for _, fp := range a.pendingFiles {
		if fp.Error != "" {
			continue
		}
		inserted, err := a.db.InsertTransactions(fp.Transactions)
		if err != nil {
			a.pendingFiles = nil
			return &ParseResult{Error: fmt.Sprintf("Erro ao inserir: %v", err)}
		}
		totalInserted += inserted
		totalSkipped += len(fp.Transactions) - inserted
		lastBank = fp.Bank
	}

	a.pendingFiles = nil
	return &ParseResult{
		Bank:     lastBank,
		Inserted: totalInserted,
		Skipped:  totalSkipped,
	}
}

func (a *AppService) CancelImport() {
	a.pendingFiles = nil
}

// --- Search & stats ---

func (a *AppService) Search(query string, limit, offset int) *SearchResult {
	if a.db == nil {
		return &SearchResult{Transactions: []Transaction{}, Total: 0}
	}
	result, err := a.db.Search(query, limit, offset)
	if err != nil {
		return &SearchResult{Transactions: []Transaction{}, Total: 0}
	}
	return result
}

func (a *AppService) GetStats() map[string]any {
	if a.db == nil {
		return map[string]any{"error": a.dbError}
	}
	stats, err := a.db.GetStats()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return stats
}

func (a *AppService) ExportResults(query string) string {
	if a.db == nil {
		return a.dbError
	}
	txns, err := a.db.SearchAll(query)
	if err != nil {
		return fmt.Sprintf("Erro: %v", err)
	}
	if len(txns) == 0 {
		return "Nenhuma transação para exportar"
	}

	app := application.Get()
	home, _ := os.UserHomeDir()
	defaultName := fmt.Sprintf("extratos_%s.xlsx", time.Now().Format("2006-01-02"))
	savePath, err := app.Dialog.SaveFile().
		SetMessage("Salvar XLSX").
		SetFilename(defaultName).
		SetDirectory(home).
		AddFilter("Excel (*.xlsx)", "*.xlsx").
		PromptForSingleSelection()
	if err != nil || savePath == "" {
		return "Exportação cancelada"
	}

	if filepath.Ext(savePath) != ".xlsx" {
		savePath += ".xlsx"
	}

	if err := ExportXLSX(txns, savePath, query); err != nil {
		return fmt.Sprintf("Erro ao exportar: %v", err)
	}

	return fmt.Sprintf("Exportado %d transações para %s", len(txns), filepath.Base(savePath))
}

// --- helpers ---

func buildFilePreview(txns []Transaction, filename, bank string) FilePreview {
	fp := FilePreview{
		Filename:     filename,
		Bank:         bank,
		Transactions: txns,
		Count:        len(txns),
	}

	if len(txns) == 0 {
		return fp
	}

	// Compute aggregates
	var minDate, maxDate string
	accounts := make(map[string]bool)
	for _, t := range txns {
		if !IsInternalTransfer(t.Description) {
			if t.Credit != nil {
				fp.TotalCredit += *t.Credit
			}
			if t.Debit != nil {
				fp.TotalDebit += *t.Debit
			}
		}
		if minDate == "" || t.Date < minDate {
			minDate = t.Date
		}
		if maxDate == "" || t.Date > maxDate {
			maxDate = t.Date
		}
		if t.Account != "" {
			accounts[t.Account] = true
		}
	}

	fp.DateRange = minDate + " a " + maxDate

	// Pick first account as representative
	for acct := range accounts {
		fp.Account = acct
		break
	}

	return fp
}
