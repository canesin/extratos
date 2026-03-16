package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBRNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected *float64
	}{
		{`"3.919,07"`, ptr(3919.07)},
		{`"-3.919,45"`, ptr(-3919.45)},
		{`"1,00"`, ptr(1.00)},
		{`"491.912,25"`, ptr(491912.25)},
		{`""`, nil},
		{``, nil},
		{`"0,38"`, ptr(0.38)},
		{`"-100.000,00"`, ptr(-100000.00)},
		{`"2.103.576,88"`, ptr(2103576.88)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseBRNumber(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", *result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected %v, got nil", *tt.expected)
			}
			if *result != *tt.expected {
				t.Errorf("expected %v, got %v", *tt.expected, *result)
			}
		})
	}
}

func TestParseBRDate2(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"30/12/25", "2025-12-30", false},
		{"02/01/26", "2026-01-02", false},
		{"05/01/26", "2026-01-05", false},
		{"15/03/2024", "2024-03-15", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseBRDate2(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectFormat_Bradesco(t *testing.T) {
	raw := []byte("Extrato de: Ag: 3841 | Conta: 134175-8 | Entre 01/01/2026 e 09/03/2026\r\nData;Historico;Docto.\r\n")
	if f := DetectFormat(raw); f != FormatBradesco {
		t.Errorf("expected Bradesco, got %s", f)
	}
}

func TestDetectFormat_Itau(t *testing.T) {
	raw := []byte("Itaú - Extrato\r\nData;Historico;Valor\r\n01/01/2024;PIX;1000,00\r\n")
	if f := DetectFormat(raw); f != FormatItau {
		t.Errorf("expected Itau, got %s", f)
	}
}

func TestDetectFormat_Unknown(t *testing.T) {
	raw := []byte("some random file content without bank patterns")
	if f := DetectFormat(raw); f != FormatUnknown {
		t.Errorf("expected Unknown, got %s", f)
	}
}

func TestParseBradesco_Simple(t *testing.T) {
	raw := []byte("Extrato de: Ag: 3841 | Conta: 134175-8 | Entre 01/01/2026 e 09/03/2026\nData;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$);\n30/12/25;SALDO ANTERIOR;;;;\"1,00\";\n02/01/26; Resgate Inv Fac;1965112;\"3.919,07\";;\"3.920,07\";\n02/01/26; Pagto Cobranca;0000384;;\"-3.919,45\";\"1,00\";\n;Plano sa de Janeiro;;\n")

	txns := ParseBradesco(raw, "test.csv")

	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}

	// First: Resgate
	if txns[0].Date != "2026-01-02" {
		t.Errorf("txn[0] date: expected 2026-01-02, got %s", txns[0].Date)
	}
	if txns[0].Description != "Resgate Inv Fac" {
		t.Errorf("txn[0] desc: %q", txns[0].Description)
	}
	if txns[0].Credit == nil || *txns[0].Credit != 3919.07 {
		t.Errorf("txn[0] credit: %v", txns[0].Credit)
	}
	if txns[0].Account != "Ag 3841 / 134175-8" {
		t.Errorf("txn[0] account: %q", txns[0].Account)
	}

	// Second: Pagto with continuation line
	if txns[1].Description != "Pagto Cobranca | Plano sa de Janeiro" {
		t.Errorf("txn[1] desc (with continuation): %q", txns[1].Description)
	}
	if txns[1].Debit == nil || *txns[1].Debit != -3919.45 {
		t.Errorf("txn[1] debit: %v", txns[1].Debit)
	}
}

func TestParseBradesco_SkipsSections(t *testing.T) {
	raw := []byte("Extrato de: Ag: 3841 | Conta: 134175-8 | Entre 01/01/2026 e 09/03/2026\nData;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$);\n02/01/26; Test Txn;123;\"100,00\";;\"101,00\";\n;Total;;\"100,00\";\"0,00\";\"101,00\"\nOs dados acima têm como base...\nÚltimos Lançamentos\nData;Histórico;Docto.;Crédito (R$);Débito (R$);\n09/03/26; Should Skip;999;\"50,00\";;\n;Total;;\"50,00\";\"-10,00\"\nSaldos Invest Fácil\nData;Histórico;Saldo (R$);\n09/03/26; Saldo Invest Fácil;604.558,80;\n")

	txns := ParseBradesco(raw, "test.csv")

	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction (skipping sections), got %d", len(txns))
	}
	if txns[0].Description != "Test Txn" {
		t.Errorf("expected 'Test Txn', got %q", txns[0].Description)
	}
}

func TestParseBradesco_MultipleAccounts(t *testing.T) {
	raw := []byte("Extrato de: Ag: 3841 | Conta: 134175-8 | Entre 01/01/2026 e 09/03/2026\nData;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$);\n02/01/26; Txn A;111;\"100,00\";;;\nExtrato de: Ag: 7238 | Conta: 34175-4 | Entre 01/01/2026 e 09/03/2026\nData;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$);\n03/01/26; Txn B;222;\"200,00\";;;\n")

	txns := ParseBradesco(raw, "test.csv")

	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}
	if txns[0].Account != "Ag 3841 / 134175-8" {
		t.Errorf("txn[0] account: %q", txns[0].Account)
	}
	if txns[1].Account != "Ag 7238 / 34175-4" {
		t.Errorf("txn[1] account: %q", txns[1].Account)
	}
}

func TestParseBradesco_CRLineEndings(t *testing.T) {
	// Simulate actual Bradesco files: CR-only line endings
	raw := []byte("Extrato de: Ag: 3841 | Conta: 134175-8 | Entre 01/01/2026 e 09/03/2026\rData;Histórico;Docto.;Crédito (R$);Débito (R$);Saldo (R$);\r02/01/26; Test;999;\"50,00\";;;\r")

	// ParseBradesco expects normalized lines, but ParseFile normalizes for us.
	// Simulate what ParseFile does:
	normalized := make([]byte, len(raw))
	copy(normalized, raw)
	for i, b := range normalized {
		if b == '\r' {
			normalized[i] = '\n'
		}
	}

	txns := ParseBradesco(normalized, "test.csv")
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction with CR endings, got %d", len(txns))
	}
}

func TestParseItau_Simple(t *testing.T) {
	raw := []byte("Itaú - Extrato\nData;Histórico;Valor\n05/01/2024;Pix recebido;\"1.000,00\"\n06/01/2024;Pix enviado;\"-500,00\"\n")

	txns := ParseItau(raw, "itau.txt")

	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}
	if txns[0].Date != "2024-01-05" {
		t.Errorf("txn[0] date: %s", txns[0].Date)
	}
	if txns[0].Credit == nil || *txns[0].Credit != 1000.00 {
		t.Errorf("txn[0] credit: %v", txns[0].Credit)
	}
	if txns[1].Debit == nil || *txns[1].Debit != -500.00 {
		t.Errorf("txn[1] debit: %v", txns[1].Debit)
	}
	if txns[0].Bank != "Itau" {
		t.Errorf("txn[0] bank: %s", txns[0].Bank)
	}
}

func TestParseFile_RealBradesco(t *testing.T) {
	// Test against the actual Bradesco files if available
	files, _ := filepath.Glob("../Bradesco *.csv")
	if len(files) == 0 {
		t.Skip("no Bradesco CSV files found in parent directory")
	}

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			result, err := ParseFile(f)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if result.Error != "" {
				t.Fatalf("parse error: %s", result.Error)
			}
			if result.Bank != "Bradesco" {
				t.Errorf("expected bank Bradesco, got %s", result.Bank)
			}
			if len(result.Transactions) == 0 {
				t.Error("expected transactions, got 0")
			}
			t.Logf("%s: %d transactions", filepath.Base(f), len(result.Transactions))

			// Verify all transactions have required fields
			for i, txn := range result.Transactions {
				if txn.Date == "" {
					t.Errorf("txn[%d] missing date", i)
				}
				if txn.Description == "" {
					t.Errorf("txn[%d] missing description", i)
				}
				if txn.Bank != "Bradesco" {
					t.Errorf("txn[%d] wrong bank: %s", i, txn.Bank)
				}
			}
		})
	}
}

func TestDecodeToUTF8_Latin1(t *testing.T) {
	// "Histórico" in ISO-8859-1
	latin1 := []byte{0x48, 0x69, 0x73, 0x74, 0xF3, 0x72, 0x69, 0x63, 0x6F}
	result := decodeToUTF8(latin1)
	if result != "Histórico" {
		t.Errorf("expected 'Histórico', got %q", result)
	}
}

func TestDecodeToUTF8_AlreadyUTF8(t *testing.T) {
	utf8 := []byte("Histórico")
	result := decodeToUTF8(utf8)
	if result != "Histórico" {
		t.Errorf("expected 'Histórico', got %q", result)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"a\nb\nc", 3},
		{"a\r\nb\r\nc", 3},
		{"a\rb\rc", 3},
		{"single", 1},
	}

	for _, tt := range tests {
		lines := splitLines(tt.input)
		if len(lines) != tt.expected {
			t.Errorf("splitLines(%q): expected %d lines, got %d", tt.input, tt.expected, len(lines))
		}
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Maytê", "mayte"},
		{"Histórico", "historico"},
		{"Último Lançamento", "ultimo lancamento"},
		{"FERNANDA", "fernanda"},
		{"café", "cafe"},
		{"São Paulo", "sao paulo"},
		{"plain ascii", "plain ascii"},
		{"Ação Reação", "acao reacao"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeText(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildSearchText(t *testing.T) {
	st := buildSearchText("Transfe Pix | Des: Maytê", "Ag 3841 / 134175-8", "Bradesco")
	if st != "transfe pix | des: mayte ag 3841 / 134175-8 bradesco" {
		t.Errorf("unexpected search text: %q", st)
	}
}

func TestParseFile_NonexistentFile(t *testing.T) {
	_, err := ParseFile("/nonexistent/file.csv")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseFile_UnknownFormat(t *testing.T) {
	// Write a temp file with unknown content
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.csv")
	os.WriteFile(path, []byte("just some random text\nwithout any bank patterns\n"), 0644)

	result, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for unknown format")
	}
}
