package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestParseItauXLS(t *testing.T) {
	files := []string{
		"../Extrato Conta Corrente-120320261437.xls",
		"../Extrato Conta Corrente-120320261438.xls",
		"../Extrato Conta Corrente-120320261439.xls",
	}

	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			t.Fatalf("abs path: %v", err)
		}
		t.Run(filepath.Base(f), func(t *testing.T) {
			result, err := ParseFile(abs)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if result.Error != "" {
				t.Fatalf("ParseResult error: %s", result.Error)
			}
			if result.Bank != "Itau" {
				t.Errorf("expected bank Itau, got %s", result.Bank)
			}
			if len(result.Transactions) == 0 {
				t.Fatal("no transactions parsed")
			}

			t.Logf("Parsed %d transactions", len(result.Transactions))

			// Check first and last transaction
			first := result.Transactions[0]
			last := result.Transactions[len(result.Transactions)-1]
			t.Logf("First: %s %s credit=%v debit=%v account=%s",
				first.Date, first.Description, ptrStr(first.Credit), ptrStr(first.Debit), first.Account)
			t.Logf("Last:  %s %s credit=%v debit=%v account=%s",
				last.Date, last.Description, ptrStr(last.Credit), ptrStr(last.Debit), last.Account)

			// Verify no SALDO rows leaked through
			for _, tx := range result.Transactions {
				if tx.Description == "SALDO ANTERIOR" {
					t.Error("SALDO ANTERIOR should be filtered")
				}
				if len(tx.Description) > 15 && tx.Description[:15] == "SALDO TOTAL DIS" {
					t.Error("SALDO TOTAL DISPONÍVEL DIA should be filtered")
				}
			}

			// Verify account format
			if first.Account == "Itaú" {
				t.Error("account should be parsed from header, not default")
			}

			// Count credits and debits
			var credits, debits int
			var totalCredit, totalDebit float64
			for _, tx := range result.Transactions {
				if tx.Credit != nil {
					credits++
					totalCredit += *tx.Credit
				}
				if tx.Debit != nil {
					debits++
					totalDebit += *tx.Debit
				}
			}
			t.Logf("Credits: %d (R$ %.2f), Debits: %d (R$ %.2f)", credits, totalCredit, debits, totalDebit)
		})
	}
}

func ptrStr(p *float64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%.2f", *p)
}
