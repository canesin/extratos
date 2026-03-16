package main

import (
	"os"
	"testing"
)

func TestParseNubank(t *testing.T) {
	raw, err := os.ReadFile("testdata/synthetic_nubank.csv")
	if err != nil {
		t.Fatalf("read test file: %v", err)
	}

	txns := ParseNubank(raw, "synthetic_nubank.csv")

	if len(txns) != 10 {
		t.Fatalf("expected 10 transactions, got %d", len(txns))
	}

	// All transactions should have bank = "Nubank"
	for i, tx := range txns {
		if tx.Bank != "Nubank" {
			t.Errorf("txn[%d] bank: expected 'Nubank', got %q", i, tx.Bank)
		}
		if tx.Account != "Nubank" {
			t.Errorf("txn[%d] account: expected 'Nubank', got %q", i, tx.Account)
		}
		if tx.SourceFile != "synthetic_nubank.csv" {
			t.Errorf("txn[%d] source_file: expected 'synthetic_nubank.csv', got %q", i, tx.SourceFile)
		}
	}

	// First transaction: iFood on 2026-01-05, -45.90
	first := txns[0]
	if first.Date != "2026-01-05" {
		t.Errorf("first date: expected '2026-01-05', got %q", first.Date)
	}
	if first.Description != "iFood" {
		t.Errorf("first desc: expected 'iFood', got %q", first.Description)
	}
	if first.Debit == nil || *first.Debit != -45.90 {
		t.Errorf("first debit: expected -45.90, got %v", first.Debit)
	}
	if first.Doc != "abc123def456" {
		t.Errorf("first doc: expected 'abc123def456', got %q", first.Doc)
	}

	// Second transaction: PIX received, +3500.00
	second := txns[1]
	if second.Date != "2026-01-08" {
		t.Errorf("second date: expected '2026-01-08', got %q", second.Date)
	}
	if second.Credit == nil || *second.Credit != 3500.00 {
		t.Errorf("second credit: expected 3500.00, got %v", second.Credit)
	}

	// Verify description with special characters survived
	if second.Description != "PIX recebido - João Silva" {
		t.Errorf("second desc: expected 'PIX recebido - João Silva', got %q", second.Description)
	}

	// Count credits and debits
	var credits, debits int
	for _, tx := range txns {
		if tx.Credit != nil {
			credits++
		}
		if tx.Debit != nil {
			debits++
		}
	}
	if credits != 3 {
		t.Errorf("expected 3 credits, got %d", credits)
	}
	if debits != 7 {
		t.Errorf("expected 7 debits, got %d", debits)
	}

	// Verify internal transfer detection works for Nubank patterns
	for _, tx := range txns {
		if tx.Description == "Aplicação RDB" || tx.Description == "Resgate RDB" {
			if !IsInternalTransfer(tx.Description) {
				t.Errorf("expected %q to be detected as internal transfer", tx.Description)
			}
		}
	}

	// Last transaction: Amazon on 2026-02-01
	last := txns[len(txns)-1]
	if last.Date != "2026-02-01" {
		t.Errorf("last date: expected '2026-02-01', got %q", last.Date)
	}
	if last.Description != "Amazon" {
		t.Errorf("last desc: expected 'Amazon', got %q", last.Description)
	}
}

func TestDetectFormat_Nubank(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "standard header",
			input: "Data,Valor,Identificador,Descrição\n2026-01-01,-10.00,abc,Test\n",
		},
		{
			name:  "header without accent",
			input: "Data,Valor,Identificador,Descricao\n2026-01-01,-10.00,abc,Test\n",
		},
		{
			name:  "uppercase header",
			input: "DATA,VALOR,IDENTIFICADOR,DESCRIÇÃO\n2026-01-01,-10.00,abc,Test\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if f := DetectFormat([]byte(tt.input)); f != FormatNubank {
				t.Errorf("expected FormatNubank, got %s", f)
			}
		})
	}
}

func TestParseFile_Nubank(t *testing.T) {
	result, err := ParseFile("testdata/synthetic_nubank.csv")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("parse error: %s", result.Error)
	}
	if result.Bank != "Nubank" {
		t.Errorf("expected bank 'Nubank', got %s", result.Bank)
	}
	if len(result.Transactions) != 10 {
		t.Errorf("expected 10 transactions, got %d", len(result.Transactions))
	}
}
