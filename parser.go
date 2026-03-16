package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/shakinm/xlsReader/xls"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	dateRE2   = regexp.MustCompile(`^(\d{2}/\d{2}/\d{2});`)
	dateRE4   = regexp.MustCompile(`^(\d{2}/\d{2}/\d{4});`)
	headerRE  = regexp.MustCompile(`^Extrato de: Ag: (\d+) \| Conta: ([\d-]+) \| Entre`)
	itauDateRE = regexp.MustCompile(`^(\d{2}/\d{2}/\d{4})`)
)

type BankFormat string

const (
	FormatBradesco BankFormat = "Bradesco"
	FormatItau     BankFormat = "Itau"
	FormatUnknown  BankFormat = "Unknown"
)

type ParseResult struct {
	Transactions []Transaction `json:"transactions"`
	Bank         string        `json:"bank"`
	Inserted     int           `json:"inserted"`
	Skipped      int           `json:"skipped"`
	Error        string        `json:"error,omitempty"`
}

func DetectFormat(raw []byte) BankFormat {
	// Normalize to UTF-8 for detection
	text := decodeToUTF8(raw)

	if headerRE.MatchString(text) {
		return FormatBradesco
	}

	// Itaú: look for characteristic patterns
	lower := strings.ToLower(text)
	if strings.Contains(lower, "itaú") || strings.Contains(lower, "itau") {
		return FormatItau
	}

	// Itaú OFX-style or different header
	lines := splitLines(text)
	for _, line := range lines[:min(10, len(lines))] {
		// Itaú TXT format: date;desc;value pattern
		if itauDateRE.MatchString(strings.TrimSpace(line)) {
			return FormatItau
		}
	}

	// If it has Bradesco-style semicolons and dates, assume Bradesco
	for _, line := range lines[:min(20, len(lines))] {
		if dateRE2.MatchString(strings.TrimSpace(line)) {
			return FormatBradesco
		}
	}

	return FormatUnknown
}

func decodeToUTF8(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	// Fall back to ISO-8859-1
	decoder := charmap.ISO8859_1.NewDecoder()
	result, err := decoder.Bytes(raw)
	if err != nil {
		return string(raw)
	}
	return string(result)
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func parseBRNumber(s string) *float64 {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseBRDate2(s string) (string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid date: %s", s)
	}
	day, _ := strconv.Atoi(parts[0])
	month, _ := strconv.Atoi(parts[1])
	year, _ := strconv.Atoi(parts[2])
	if year < 100 {
		year += 2000
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return t.Format("2006-01-02"), nil
}

var skipSections = map[string]bool{
	"Últimos Lançamentos": true,
	"Saldos Invest Fácil": true,
}

func ParseBradesco(raw []byte, filename string) []Transaction {
	text := decodeToUTF8(raw)
	lines := splitLines(text)

	var txns []Transaction
	account := ""
	inSkip := false

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		if m := headerRE.FindStringSubmatch(line); m != nil {
			account = fmt.Sprintf("Ag %s / %s", m[1], m[2])
			inSkip = false
			i++
			continue
		}

		if skipSections[line] {
			inSkip = true
			i++
			continue
		}

		if inSkip || strings.HasPrefix(line, "Data;") || strings.HasPrefix(line, "Os dados") {
			i++
			continue
		}
		if strings.HasPrefix(line, ";Total") || line == "" {
			i++
			continue
		}

		if dateRE2.MatchString(line) {
			fields := strings.Split(line, ";")
			dt, err := parseBRDate2(fields[0])
			if err != nil {
				i++
				continue
			}

			desc := ""
			if len(fields) > 1 {
				desc = strings.TrimSpace(fields[1])
			}
			doc := ""
			if len(fields) > 2 {
				doc = strings.Trim(strings.TrimSpace(fields[2]), `"`)
			}

			credit := (*float64)(nil)
			if len(fields) > 3 {
				credit = parseBRNumber(fields[3])
			}
			debit := (*float64)(nil)
			if len(fields) > 4 {
				debit = parseBRNumber(fields[4])
			}
			balance := (*float64)(nil)
			if len(fields) > 5 {
				balance = parseBRNumber(fields[5])
			}

			if desc == "SALDO ANTERIOR" {
				i++
				continue
			}

			// Collect continuation lines
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(next, ";") && !strings.HasPrefix(next, ";Total") {
					parts := strings.SplitN(next, ";", 3)
					if len(parts) > 1 {
						extra := strings.TrimSpace(parts[1])
						if extra != "" {
							desc += " | " + extra
						}
					}
					i++
				} else {
					break
				}
			}

			var amount *float64
			if credit != nil {
				amount = credit
			} else {
				amount = debit
			}

			txns = append(txns, Transaction{
				Date:        dt,
				Description: desc,
				Doc:         doc,
				Credit:      credit,
				Debit:       debit,
				Balance:     balance,
				Amount:      amount,
				Account:     account,
				Bank:        string(FormatBradesco),
				SourceFile:  filename,
			})
		}
		i++
	}
	return txns
}

func ParseItau(raw []byte, filename string) []Transaction {
	text := decodeToUTF8(raw)
	lines := splitLines(text)

	var txns []Transaction
	account := "Itaú"

	// Try to find account info from header
	for _, line := range lines[:min(5, len(lines))] {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "ag") && strings.Contains(lower, "conta") {
			account = strings.TrimSpace(line)
			break
		}
	}

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Skip header lines
		if strings.HasPrefix(line, "Data;") || strings.HasPrefix(line, "data;") {
			continue
		}

		fields := strings.Split(line, ";")
		if len(fields) < 3 {
			continue
		}

		// Try to parse date (DD/MM/YYYY or DD/MM/YY)
		dt, err := parseBRDate2(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}

		desc := strings.TrimSpace(fields[1])
		if desc == "SALDO ANTERIOR" || desc == "SALDO DO DIA" || desc == "" {
			continue
		}

		// Itaú format varies: may have separate credit/debit columns or a single value column
		var credit, debit, balance *float64

		if len(fields) >= 6 {
			// Full format: date;desc;doc;credit;debit;balance
			credit = parseBRNumber(fields[3])
			debit = parseBRNumber(fields[4])
			balance = parseBRNumber(fields[5])
		} else if len(fields) >= 3 {
			// Simple format: date;desc;value
			val := parseBRNumber(fields[2])
			if val != nil {
				if *val >= 0 {
					credit = val
				} else {
					debit = val
				}
			}
		}

		var amount *float64
		if credit != nil {
			amount = credit
		} else {
			amount = debit
		}

		// Collect continuation lines (Itaú also has these sometimes)
		for i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if strings.HasPrefix(next, ";") && !strings.HasPrefix(next, ";Total") {
				parts := strings.SplitN(next, ";", 3)
				if len(parts) > 1 {
					extra := strings.TrimSpace(parts[1])
					if extra != "" {
						desc += " | " + extra
					}
				}
				i++
			} else {
				break
			}
		}

		txns = append(txns, Transaction{
			Date:        dt,
			Description: desc,
			Doc:         "",
			Credit:      credit,
			Debit:       debit,
			Balance:     balance,
			Amount:      amount,
			Account:     account,
			Bank:        string(FormatItau),
			SourceFile:  filename,
		})
	}
	return txns
}

// ParseItauXLS parses an Itaú .xls (BIFF) file exported from internet banking.
// Format: Sheet "Lançamentos" with columns: data, lançamento, ag./origem, valor (R$), saldos (R$)
func ParseItauXLS(path string) (*ParseResult, error) {
	wb, err := xls.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xls: %w", err)
	}

	sheet, err := wb.GetSheet(0)
	if err != nil {
		return nil, fmt.Errorf("get sheet: %w", err)
	}

	filename := filepath.Base(path)

	// Extract account info from header rows (3=agency, 4=account)
	account := "Itaú"
	agency := ""
	acctNum := ""

	if sheet.GetNumberRows() > 4 {
		row3, _ := sheet.GetRow(3)
		cols3 := row3.GetCols()
		if len(cols3) > 1 {
			// Agency is a number cell (e.g. 7017.0)
			v := cols3[1].GetFloat64()
			if v > 0 {
				agency = strconv.Itoa(int(v))
			} else {
				agency = strings.TrimSpace(cols3[1].GetString())
			}
		}
		row4, _ := sheet.GetRow(4)
		cols4 := row4.GetCols()
		if len(cols4) > 1 {
			acctNum = strings.TrimSpace(cols4[1].GetString())
		}
	}
	if agency != "" && acctNum != "" {
		account = fmt.Sprintf("Ag %s / %s", agency, acctNum)
	}

	var txns []Transaction
	nrows := sheet.GetNumberRows()

	// Data starts at row 10 (after header rows 0-9)
	for r := 10; r < nrows; r++ {
		row, _ := sheet.GetRow(r)
		cols := row.GetCols()
		if len(cols) < 2 {
			continue
		}

		dateStr := strings.TrimSpace(cols[0].GetString())
		desc := strings.TrimSpace(cols[1].GetString())

		if dateStr == "" || desc == "" {
			continue
		}

		// Parse date DD/MM/YYYY
		dt, err := parseBRDate2(dateStr)
		if err != nil {
			continue
		}

		// Skip balance summary rows and opening balance
		if desc == "SALDO ANTERIOR" {
			continue
		}
		if strings.HasPrefix(desc, "SALDO TOTAL DISPON") {
			continue
		}

		// Value column (col 3): positive = credit, negative = debit
		var credit, debit, amount *float64

		if len(cols) > 3 {
			valType := cols[3].GetType()
			if strings.Contains(valType, "Number") || strings.Contains(valType, "Rk") {
				v := cols[3].GetFloat64()
				if v != 0 || cols[3].GetString() != "" {
					// Round to 2 decimal places to avoid float precision issues
					v = math.Round(v*100) / 100
					if v >= 0 {
						credit = &v
					} else {
						debit = &v
					}
					amount = &v
				}
			}
		}

		txns = append(txns, Transaction{
			Date:        dt,
			Description: desc,
			Doc:         "",
			Credit:      credit,
			Debit:       debit,
			Balance:     nil,
			Amount:      amount,
			Account:     account,
			Bank:        string(FormatItau),
			SourceFile:  filename,
		})
	}

	return &ParseResult{
		Transactions: txns,
		Bank:         string(FormatItau),
	}, nil
}

func ParseFile(path string) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(path))

	// Binary .xls files (Itaú exports) need a dedicated parser
	if ext == ".xls" {
		return ParseItauXLS(path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Normalize line endings
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	raw = bytes.ReplaceAll(raw, []byte("\r"), []byte("\n"))

	filename := filepath.Base(path)

	format := DetectFormat(raw)
	var txns []Transaction

	switch format {
	case FormatBradesco:
		txns = ParseBradesco(raw, filename)
	case FormatItau:
		txns = ParseItau(raw, filename)
	default:
		return &ParseResult{Error: "Formato não reconhecido. Suporta: Bradesco, Itaú"}, nil
	}

	return &ParseResult{
		Transactions: txns,
		Bank:         string(format),
	}, nil
}

// normalizeText strips accents and lowercases text for accent-insensitive search.
func normalizeText(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return strings.ToLower(result)
}

// buildSearchText creates the normalized search text for FTS indexing.
func buildSearchText(desc, account, bank string) string {
	return normalizeText(desc + " " + account + " " + bank)
}

// fileHash returns a SHA256 hash of file contents for dedup
func fileHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}
