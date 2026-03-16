package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseOFX(t *testing.T) {
	raw, err := os.ReadFile("testdata/synthetic_bb.ofx")
	if err != nil {
		t.Fatalf("read test file: %v", err)
	}

	txns := ParseOFX(raw, "synthetic_bb.ofx")

	if len(txns) != 15 {
		t.Fatalf("expected 15 transactions, got %d", len(txns))
	}

	// Verify bank name resolved from FID 001
	for i, tx := range txns {
		if tx.Bank != "Banco do Brasil" {
			t.Errorf("txn[%d] bank: expected 'Banco do Brasil', got %q", i, tx.Bank)
		}
		if tx.Account != "Ag 001 / 98765-4" {
			t.Errorf("txn[%d] account: expected 'Ag 001 / 98765-4', got %q", i, tx.Account)
		}
		if tx.SourceFile != "synthetic_bb.ofx" {
			t.Errorf("txn[%d] source_file: expected 'synthetic_bb.ofx', got %q", i, tx.SourceFile)
		}
	}

	// First transaction: CREDITO SALARIO on 2026-01-05, +8500.00
	first := txns[0]
	if first.Date != "2026-01-05" {
		t.Errorf("first date: expected '2026-01-05', got %q", first.Date)
	}
	if first.Description != "CREDITO SALARIO" {
		t.Errorf("first desc: expected 'CREDITO SALARIO', got %q", first.Description)
	}
	if first.Credit == nil || *first.Credit != 8500.00 {
		t.Errorf("first credit: expected 8500.00, got %v", first.Credit)
	}
	if first.Debit != nil {
		t.Errorf("first debit: expected nil, got %v", *first.Debit)
	}

	// Second transaction: PIX ENVIADO, debit -350.00
	second := txns[1]
	if second.Date != "2026-01-07" {
		t.Errorf("second date: expected '2026-01-07', got %q", second.Date)
	}
	if second.Debit == nil || *second.Debit != -350.00 {
		t.Errorf("second debit: expected -350.00, got %v", second.Debit)
	}
	if second.Credit != nil {
		t.Errorf("second credit: expected nil, got %v", *second.Credit)
	}

	// Transaction with CHECKNUM: PAGTO BOLETO ENERGIA (index 4)
	boleto := txns[4]
	if boleto.Doc != "000456" {
		t.Errorf("boleto doc: expected '000456', got %q", boleto.Doc)
	}

	// Transaction with timestamp in date (YYYYMMDDHHMMSS): index 11, CREDITO SALARIO
	salary2 := txns[11]
	if salary2.Date != "2026-02-05" {
		t.Errorf("salary2 date (from YYYYMMDDHHMMSS): expected '2026-02-05', got %q", salary2.Date)
	}

	// Verify credit/debit signs: count credits and debits
	var credits, debits int
	for _, tx := range txns {
		if tx.Credit != nil {
			credits++
			if *tx.Credit < 0 {
				t.Errorf("credit should be positive: %v for %q", *tx.Credit, tx.Description)
			}
		}
		if tx.Debit != nil {
			debits++
			if *tx.Debit > 0 {
				t.Errorf("debit should be negative: %v for %q", *tx.Debit, tx.Description)
			}
		}
	}
	if credits != 6 {
		t.Errorf("expected 6 credits, got %d", credits)
	}
	if debits != 9 {
		t.Errorf("expected 9 debits, got %d", debits)
	}

	// Verify no SALDO ANTERIOR leaked through
	for _, tx := range txns {
		if strings.Contains(tx.Description, "SALDO ANTERIOR") {
			t.Error("SALDO ANTERIOR should not appear in transactions")
		}
	}

	// Verify internal transfer detection works for BB patterns
	for _, tx := range txns {
		if tx.Description == "APLICACAO POUPANCA" || tx.Description == "RESGATE POUPANCA" ||
			tx.Description == "APLICACAO FUNDOS" {
			if !IsInternalTransfer(tx.Description) {
				t.Errorf("expected %q to be detected as internal transfer", tx.Description)
			}
		}
	}
}

func TestParseOFX_CommaDecimal(t *testing.T) {
	ofx := `OFXHEADER:100
DATA:OFXSGML
VERSION:102
ENCODING:USASCII
CHARSET:1252
COMPRESSION:NONE
OLDFILEUID:NONE
NEWFILEUID:NONE

<OFX>
<SIGNONMSGSRSV1>
 <SONRS>
  <STATUS><CODE>0<SEVERITY>INFO</STATUS>
  <DTSERVER>20260301
  <LANGUAGE>POR
  <FI><ORG>Banco Teste<FID>999</FI>
 </SONRS>
</SIGNONMSGSRSV1>
<BANKMSGSRSV1>
 <STMTTRNRS>
  <TRNUID>0
  <STATUS><CODE>0<SEVERITY>INFO</STATUS>
  <STMTRS>
   <CURDEF>BRL
   <BANKACCTFROM>
    <BANKID>999
    <ACCTID>11111-1
    <ACCTTYPE>CHECKING
   </BANKACCTFROM>
   <BANKTRANLIST>
    <DTSTART>20260101
    <DTEND>20260301
    <STMTTRN>
     <TRNTYPE>DEBIT
     <DTPOSTED>20260115
     <TRNAMT>-1500,50
     <FITID>20260115001
     <MEMO>PAGAMENTO COM VIRGULA
    </STMTTRN>
    <STMTTRN>
     <TRNTYPE>CREDIT
     <DTPOSTED>20260120
     <TRNAMT>2500,75
     <FITID>20260120001
     <MEMO>CREDITO COM VIRGULA
    </STMTTRN>
   </BANKTRANLIST>
  </STMTRS>
 </STMTTRNRS>
</BANKMSGSRSV1>
</OFX>`

	txns := ParseOFX([]byte(ofx), "test_comma.ofx")

	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}

	// First: debit with comma decimal
	if txns[0].Debit == nil || *txns[0].Debit != -1500.50 {
		t.Errorf("comma debit: expected -1500.50, got %v", txns[0].Debit)
	}

	// Second: credit with comma decimal
	if txns[1].Credit == nil || *txns[1].Credit != 2500.75 {
		t.Errorf("comma credit: expected 2500.75, got %v", txns[1].Credit)
	}

	// Bank: FID 999 not in map, should use ORG
	if txns[0].Bank != "Banco Teste" {
		t.Errorf("bank: expected 'Banco Teste', got %q", txns[0].Bank)
	}

	if txns[0].Account != "Ag 999 / 11111-1" {
		t.Errorf("account: expected 'Ag 999 / 11111-1', got %q", txns[0].Account)
	}
}

func TestDetectFormat_OFX(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "OFXHEADER in first lines",
			input: "OFXHEADER:100\nDATA:OFXSGML\nVERSION:102\n",
		},
		{
			name:  "OFX tag in first 20 lines",
			input: "some preamble\n\n\n\n<OFX>\n<SIGNONMSGSRSV1>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if f := DetectFormat([]byte(tt.input)); f != FormatOFX {
				t.Errorf("expected FormatOFX, got %s", f)
			}
		})
	}
}

func TestParseFile_OFX(t *testing.T) {
	result, err := ParseFile("testdata/synthetic_bb.ofx")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("parse error: %s", result.Error)
	}
	if result.Bank != "Banco do Brasil" {
		t.Errorf("expected bank 'Banco do Brasil', got %s", result.Bank)
	}
	if len(result.Transactions) != 15 {
		t.Errorf("expected 15 transactions, got %d", len(result.Transactions))
	}
}

func TestParseOFX_FIDMapping(t *testing.T) {
	tests := []struct {
		fid      string
		org      string
		expected string
	}{
		{"001", "Banco do Brasil S/A", "Banco do Brasil"},
		{"104", "CEF", "Caixa"},
		{"033", "Santander BR", "Santander"},
		{"077", "Banco Inter", "Inter"},
		{"756", "Sicoob", "Sicoob"},
		{"999", "Custom Bank", "Custom Bank"},
		{"", "", "OFX"},
	}

	for _, tt := range tests {
		t.Run(tt.fid+"/"+tt.org, func(t *testing.T) {
			got := resolveBankName(tt.fid, tt.org)
			if got != tt.expected {
				t.Errorf("resolveBankName(%q, %q) = %q, want %q", tt.fid, tt.org, got, tt.expected)
			}
		})
	}
}
