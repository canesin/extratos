package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExportXLSX_Basic(t *testing.T) {
	txns := sampleTransactions()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xlsx")

	if err := ExportXLSX(txns, path, ""); err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer f.Close()

	sheet := "Extratos"
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("get rows: %v", err)
	}

	// Header + 3 data rows + blank + summary = at least 4 rows
	if len(rows) < 4 {
		t.Errorf("expected at least 4 rows (header + 3 data), got %d", len(rows))
	}

	if rows[0][0] != "Data" {
		t.Errorf("header[0]: expected 'Data', got %q", rows[0][0])
	}
	if rows[0][1] != "Descrição" {
		t.Errorf("header[1]: expected 'Descrição', got %q", rows[0][1])
	}
	if rows[1][0] != "2026-01-05" {
		t.Errorf("row[1] date: expected '2026-01-05', got %q", rows[1][0])
	}
}

func TestExportXLSX_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.xlsx")

	if err := ExportXLSX([]Transaction{}, path, ""); err != nil {
		t.Fatalf("ExportXLSX empty: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	rows, _ := f.GetRows("Extratos")
	if len(rows) < 1 {
		t.Error("expected at least header row")
	}

	// Verify no SUM formulas with inverted ranges
	sumRow := 3 // header(1) + blank(2) + summary(3)
	creditCell, _ := excelize.CoordinatesToCellName(4, sumRow)
	formula, _ := f.GetCellFormula("Extratos", creditCell)
	if formula != "" {
		t.Errorf("empty export should have no sum formulas, got %q", formula)
	}
}

func TestExportXLSX_WithFilter(t *testing.T) {
	txns := sampleTransactions()[:1]
	dir := t.TempDir()
	path := filepath.Join(dir, "filtered.xlsx")

	if err := ExportXLSX(txns, path, "Fernanda, Correa"); err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	rows, _ := f.GetRows("Extratos")

	// Should have: header + 1 data + blank + filter label + summary = 5 rows
	if len(rows) < 4 {
		t.Errorf("expected at least 4 rows, got %d", len(rows))
	}

	// Find the filter label row
	found := false
	for _, row := range rows {
		if len(row) > 0 && row[0] == "Filtro:" {
			found = true
			if len(row) > 1 && row[1] != "Fernanda, Correa" {
				t.Errorf("filter value: expected 'Fernanda, Correa', got %q", row[1])
			}
			break
		}
	}
	if !found {
		t.Error("filter label not found in exported file")
	}
}

func TestExportXLSX_SumFormulas(t *testing.T) {
	txns := sampleTransactions()
	dir := t.TempDir()
	path := filepath.Join(dir, "sums.xlsx")

	if err := ExportXLSX(txns, path, ""); err != nil {
		t.Fatalf("ExportXLSX: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// Summary row: header(1) + 3 data + blank row + summary = row 6
	sumRow := len(txns) + 3
	creditCell, _ := excelize.CoordinatesToCellName(4, sumRow)
	formula, err := f.GetCellFormula("Extratos", creditCell)
	if err != nil {
		t.Fatalf("get formula: %v", err)
	}
	if formula != "SUM(D2:D4)" {
		t.Errorf("credit sum formula: expected 'SUM(D2:D4)', got %q", formula)
	}
}
