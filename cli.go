package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runCLI(args []string) {
	if len(args) == 0 {
		cliUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		cliImport(args[1:])
	case "export":
		cliExport(args[1:])
	case "stats":
		cliStats(args[1:])
	case "search":
		cliSearch(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		cliUsage()
		os.Exit(1)
	}
}

func cliUsage() {
	fmt.Fprintln(os.Stderr, "Usage: extratos-app cli <command> [options]")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  import  --db <name> <file1> [file2...]")
	fmt.Fprintln(os.Stderr, "  export  --db <name> [-q <query>] -o <file.xlsx>")
	fmt.Fprintln(os.Stderr, "  stats   --db <name> [--json]")
	fmt.Fprintln(os.Stderr, "  search  --db <name> [-q <query>] [--json]")
}

func cliImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dbName := fs.String("db", "", "database name")
	fs.Parse(args)

	if *dbName == "" {
		fmt.Fprintln(os.Stderr, "error: --db required")
		os.Exit(1)
	}

	files := fs.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one file required")
		os.Exit(1)
	}

	db, err := OpenNamedDB(*dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	totalInserted := 0
	totalSkipped := 0

	for _, path := range files {
		result, err := ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", path, err)
			continue
		}
		if result.Error != "" {
			fmt.Fprintf(os.Stderr, "error parsing %s: %s\n", path, result.Error)
			continue
		}

		inserted, err := db.InsertTransactions(result.Transactions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error inserting from %s: %v\n", path, err)
			os.Exit(1)
		}

		skipped := len(result.Transactions) - inserted
		totalInserted += inserted
		totalSkipped += skipped
		fmt.Printf("%s: %s, %d transactions (%d inserted, %d duplicates)\n",
			filepath.Base(path), result.Bank, len(result.Transactions), inserted, skipped)
	}

	fmt.Printf("Total: %d inserted, %d duplicates\n", totalInserted, totalSkipped)
}

func cliExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dbName := fs.String("db", "", "database name")
	query := fs.String("q", "", "search query")
	output := fs.String("o", "", "output XLSX file")
	fs.Parse(args)

	if *dbName == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "error: --db and -o required")
		os.Exit(1)
	}

	db, err := OpenNamedDB(*dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	txns, err := db.SearchAll(*query, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error searching: %v\n", err)
		os.Exit(1)
	}

	if len(txns) == 0 {
		fmt.Fprintln(os.Stderr, "no transactions found")
		os.Exit(1)
	}

	if filepath.Ext(*output) != ".xlsx" {
		*output += ".xlsx"
	}

	if err := ExportXLSX(txns, *output, *query); err != nil {
		fmt.Fprintf(os.Stderr, "error exporting: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Exported %d transactions to %s\n", len(txns), *output)
}

func cliStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dbName := fs.String("db", "", "database name")
	jsonOut := fs.Bool("json", false, "output as JSON")
	fs.Parse(args)

	if *dbName == "" {
		fmt.Fprintln(os.Stderr, "error: --db required")
		os.Exit(1)
	}

	db, err := OpenNamedDB(*dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	stats, err := db.GetStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(stats)
	} else {
		for k, v := range stats {
			fmt.Printf("%s: %v\n", k, v)
		}
	}
}

func cliSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	dbName := fs.String("db", "", "database name")
	query := fs.String("q", "", "search query")
	jsonOut := fs.Bool("json", false, "output as JSON")
	limit := fs.Int("limit", 0, "max results (0 = all)")
	fs.Parse(args)

	if *dbName == "" {
		fmt.Fprintln(os.Stderr, "error: --db required")
		os.Exit(1)
	}

	db, err := OpenNamedDB(*dbName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if *limit == 0 {
		*limit = 1000000
	}

	result, err := db.Search(*query, *limit, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
	} else {
		fmt.Printf("Total: %d transactions\n", result.Total)
		fmt.Printf("Credits: %.2f\n", result.TotalCredit)
		fmt.Printf("Debits: %.2f\n", result.TotalDebit)
		fmt.Printf("Net: %.2f\n", result.NetAmount)
		if result.MinDate != "" {
			fmt.Printf("Date range: %s to %s\n", result.MinDate, result.MaxDate)
		}
	}
}
