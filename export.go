package main

import (
	"fmt"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

func ExportXLSX(txns []Transaction, outPath string, query string) error {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Extratos"
	f.SetSheetName("Sheet1", sheet)

	// -- Styles --
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F3864"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "000000", Style: 2},
		},
	})
	dateStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt:    14,
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	moneyStylePos, _ := f.NewStyle(&excelize.Style{
		NumFmt: 4,
		Font:   &excelize.Font{Color: "006100"},
		Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"C6EFCE"}},
	})
	moneyStyleNeg, _ := f.NewStyle(&excelize.Style{
		NumFmt: 4,
		Font:   &excelize.Font{Color: "9C0006"},
		Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFC7CE"}},
	})
	moneyStyleNeutral, _ := f.NewStyle(&excelize.Style{
		NumFmt: 4,
	})
	textStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true},
	})
	boldStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 11}})
	boldSmallStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 10, Color: "444444"}})

	// -- Headers --
	headers := []string{"Data", "Descrição", "Doc", "Crédito (R$)", "Débito (R$)", "Saldo (R$)", "Valor (R$)", "Conta", "Banco", "Tipo", "Arquivo"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// -- Data rows --
	for row, t := range txns {
		r := row + 2

		cellDate, _ := excelize.CoordinatesToCellName(1, r)
		f.SetCellValue(sheet, cellDate, t.Date)
		f.SetCellStyle(sheet, cellDate, cellDate, dateStyle)

		cellDesc, _ := excelize.CoordinatesToCellName(2, r)
		f.SetCellValue(sheet, cellDesc, t.Description)
		f.SetCellStyle(sheet, cellDesc, cellDesc, textStyle)

		cellDoc, _ := excelize.CoordinatesToCellName(3, r)
		f.SetCellValue(sheet, cellDoc, t.Doc)

		cellCredit, _ := excelize.CoordinatesToCellName(4, r)
		if t.Credit != nil {
			f.SetCellValue(sheet, cellCredit, *t.Credit)
			f.SetCellStyle(sheet, cellCredit, cellCredit, moneyStylePos)
		}

		cellDebit, _ := excelize.CoordinatesToCellName(5, r)
		if t.Debit != nil {
			f.SetCellValue(sheet, cellDebit, *t.Debit)
			f.SetCellStyle(sheet, cellDebit, cellDebit, moneyStyleNeg)
		}

		cellBal, _ := excelize.CoordinatesToCellName(6, r)
		if t.Balance != nil {
			f.SetCellValue(sheet, cellBal, *t.Balance)
			f.SetCellStyle(sheet, cellBal, cellBal, moneyStyleNeutral)
		}

		cellAmt, _ := excelize.CoordinatesToCellName(7, r)
		if t.Amount != nil {
			f.SetCellValue(sheet, cellAmt, *t.Amount)
			if *t.Amount >= 0 {
				f.SetCellStyle(sheet, cellAmt, cellAmt, moneyStylePos)
			} else {
				f.SetCellStyle(sheet, cellAmt, cellAmt, moneyStyleNeg)
			}
		}

		cellAcc, _ := excelize.CoordinatesToCellName(8, r)
		f.SetCellValue(sheet, cellAcc, t.Account)

		cellBank, _ := excelize.CoordinatesToCellName(9, r)
		f.SetCellValue(sheet, cellBank, t.Bank)

		cellTipo, _ := excelize.CoordinatesToCellName(10, r)
		if t.IsInternal {
			f.SetCellValue(sheet, cellTipo, "Interno")
		}

		cellSrc, _ := excelize.CoordinatesToCellName(11, r)
		f.SetCellValue(sheet, cellSrc, t.SourceFile)
	}

	// -- Column widths --
	widths := map[string]float64{
		"A": 12, "B": 55, "C": 10, "D": 15, "E": 15, "F": 15, "G": 15, "H": 22, "I": 10, "J": 10, "K": 22,
	}
	for col, w := range widths {
		f.SetColWidth(sheet, col, col, w)
	}

	// -- Autofilter + freeze --
	lastDataRow := len(txns) + 1
	lastCell, _ := excelize.CoordinatesToCellName(11, lastDataRow)
	f.AutoFilter(sheet, "A1:"+lastCell, nil)
	f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, XSplit: 0, YSplit: 1,
		TopLeftCell: "A2", ActivePane: "bottomLeft",
	})

	// -- Summary section (below data) --
	nextRow := lastDataRow + 2

	// Filter label (if search was applied)
	if query != "" {
		cell, _ := excelize.CoordinatesToCellName(1, nextRow)
		f.SetCellValue(sheet, cell, "Filtro:")
		f.SetCellStyle(sheet, cell, cell, boldSmallStyle)
		valCell, _ := excelize.CoordinatesToCellName(2, nextRow)
		f.SetCellValue(sheet, valCell, query)
		nextRow++
	}

	// Transaction count + sum formulas
	labelCell, _ := excelize.CoordinatesToCellName(2, nextRow)
	f.SetCellValue(sheet, labelCell, fmt.Sprintf("Total: %d transações", len(txns)))
	f.SetCellStyle(sheet, labelCell, labelCell, boldStyle)

	if len(txns) > 0 {
		creditSum, _ := excelize.CoordinatesToCellName(4, nextRow)
		f.SetCellFormula(sheet, creditSum, fmt.Sprintf("SUM(D2:D%d)", lastDataRow))
		f.SetCellStyle(sheet, creditSum, creditSum, moneyStylePos)

		debitSum, _ := excelize.CoordinatesToCellName(5, nextRow)
		f.SetCellFormula(sheet, debitSum, fmt.Sprintf("SUM(E2:E%d)", lastDataRow))
		f.SetCellStyle(sheet, debitSum, debitSum, moneyStyleNeg)

		amtSum, _ := excelize.CoordinatesToCellName(7, nextRow)
		f.SetCellFormula(sheet, amtSum, fmt.Sprintf("SUM(G2:G%d)", lastDataRow))
		f.SetCellStyle(sheet, amtSum, amtSum, moneyStyleNeutral)
	}

	// Ensure directory exists
	dir := filepath.Dir(outPath)
	if dir != "" && dir != "." {
		// Directory should already exist (user chose it)
	}

	return f.SaveAs(outPath)
}
