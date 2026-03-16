#!/usr/bin/env python3
"""Verify extratos computations by independently parsing Bradesco CSV
and comparing with Go-produced results (JSON and/or XLSX).

Usage:
  python3 verify.py --csv testdata/synthetic_bradesco.csv
  python3 verify.py --csv testdata/synthetic_bradesco.csv --go-json /tmp/result.json
  python3 verify.py --csv testdata/synthetic_bradesco.csv --xlsx /tmp/export.xlsx
"""
import argparse
import json
import re
import sys
from decimal import Decimal


def parse_br_number(s):
    """Parse Brazilian number format: '1.234,56' -> Decimal('1234.56')"""
    s = s.strip().strip('"')
    if not s:
        return None
    s = s.replace(".", "").replace(",", ".")
    return Decimal(s)


def parse_br_date(s):
    """Parse DD/MM/YY or DD/MM/YYYY -> YYYY-MM-DD"""
    parts = s.split("/")
    if len(parts) != 3:
        return None
    day, month, year = int(parts[0]), int(parts[1]), int(parts[2])
    if year < 100:
        year += 2000
    return f"{year:04d}-{month:02d}-{day:02d}"


def parse_bradesco_csv(filepath):
    """Parse Bradesco CSV, mimicking Go parser logic exactly."""
    with open(filepath, "rb") as f:
        raw = f.read()

    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError:
        text = raw.decode("latin-1")

    text = text.replace("\r\n", "\n").replace("\r", "\n")
    lines = text.split("\n")

    header_re = re.compile(r"^Extrato de: Ag: (\d+) \| Conta: ([\d-]+) \| Entre")
    date_re = re.compile(r"^(\d{2}/\d{2}/\d{2});")
    skip_sections = {"Últimos Lançamentos", "Saldos Invest Fácil"}

    transactions = []
    account = ""
    in_skip = False

    i = 0
    while i < len(lines):
        line = lines[i].strip()

        m = header_re.match(line)
        if m:
            account = f"Ag {m.group(1)} / {m.group(2)}"
            in_skip = False
            i += 1
            continue

        if line in skip_sections:
            in_skip = True
            i += 1
            continue

        if in_skip or line.startswith("Data;") or line.startswith("Os dados"):
            i += 1
            continue

        if line.startswith(";Total") or line == "":
            i += 1
            continue

        if date_re.match(line):
            fields = line.split(";")
            dt = parse_br_date(fields[0])
            if dt is None:
                i += 1
                continue

            desc = fields[1].strip() if len(fields) > 1 else ""
            doc = fields[2].strip().strip('"') if len(fields) > 2 else ""
            credit = parse_br_number(fields[3]) if len(fields) > 3 else None
            debit = parse_br_number(fields[4]) if len(fields) > 4 else None
            balance = parse_br_number(fields[5]) if len(fields) > 5 else None

            if desc == "SALDO ANTERIOR":
                i += 1
                continue

            # Continuation lines
            while i + 1 < len(lines):
                nxt = lines[i + 1].strip()
                if nxt.startswith(";") and not nxt.startswith(";Total"):
                    parts = nxt.split(";", 2)
                    if len(parts) > 1:
                        extra = parts[1].strip()
                        if extra:
                            desc += " | " + extra
                    i += 1
                else:
                    break

            amount = credit if credit is not None else debit
            transactions.append(
                {
                    "date": dt,
                    "description": desc,
                    "doc": doc,
                    "credit": credit,
                    "debit": debit,
                    "balance": balance,
                    "amount": amount,
                    "account": account,
                }
            )

        i += 1

    return transactions


INTERNAL_PREFIXES = [
    "Resgate Inv",
    "Resg.autom",
    "Resg/vencto",
    "Rent.inv",
    "Rentab.invest",
    "Apl.invest",
    "Aplicacao Cdb",
    "Aplicacao Inve",
]


def is_internal_transfer(desc):
    """Check if a transaction is an internal banking movement."""
    return any(desc.startswith(p) for p in INTERNAL_PREFIXES)


def compute_aggregates(transactions):
    """Compute aggregate statistics matching Go's DB queries.

    Sums exclude internal banking movements (investment applications/redemptions)
    but the count includes all transactions.
    """
    total_credit = Decimal("0")
    total_debit = Decimal("0")
    net_amount = Decimal("0")
    dates = []

    for t in transactions:
        if not is_internal_transfer(t["description"]):
            if t["credit"] is not None:
                total_credit += t["credit"]
            if t["debit"] is not None:
                total_debit += t["debit"]  # negative values
            if t["amount"] is not None:
                net_amount += t["amount"]
        dates.append(t["date"])

    dates.sort()

    return {
        "count": len(transactions),
        "total_credit": float(total_credit),
        "total_debit": float(total_debit),  # negative
        "net_amount": float(net_amount),
        "min_date": dates[0] if dates else "",
        "max_date": dates[-1] if dates else "",
    }


def verify_go_json(go_json_path, expected):
    """Compare Go search result JSON against Python-computed expected values."""
    with open(go_json_path) as f:
        go_result = json.load(f)

    checks = [
        ("count", expected["count"], go_result.get("total", 0)),
        ("total_credit", expected["total_credit"], go_result.get("total_credit", 0)),
        ("total_debit", expected["total_debit"], go_result.get("total_debit", 0)),
        ("net_amount", expected["net_amount"], go_result.get("net_amount", 0)),
        ("min_date", expected["min_date"], go_result.get("min_date", "")),
        ("max_date", expected["max_date"], go_result.get("max_date", "")),
    ]

    ok = True
    for name, exp, got in checks:
        if isinstance(exp, float):
            if abs(exp - got) > 0.01:
                print(f"FAIL: {name}: Python={exp}, Go={got}", file=sys.stderr)
                ok = False
            else:
                print(f"OK: {name}: {exp}", file=sys.stderr)
        else:
            if exp != got:
                print(f"FAIL: {name}: Python={exp}, Go={got}", file=sys.stderr)
                ok = False
            else:
                print(f"OK: {name}: {exp}", file=sys.stderr)

    # Verify individual transaction count in Go JSON
    go_txns = go_result.get("transactions", [])
    if len(go_txns) != expected["count"]:
        print(
            f"FAIL: transaction_list_count: Python={expected['count']}, Go={len(go_txns)}",
            file=sys.stderr,
        )
        ok = False

    return ok


def verify_xlsx(xlsx_path, expected):
    """Read XLSX and verify values match expected."""
    try:
        import openpyxl
    except ImportError:
        print(
            "WARNING: openpyxl not installed, skipping XLSX verification",
            file=sys.stderr,
        )
        return True

    wb = openpyxl.load_workbook(xlsx_path)
    ws = wb.active

    # Count data rows (skip header row 1 and summary rows at bottom)
    # XLSX contains ALL transactions including internal; sums filter them out.
    data_rows = 0
    xlsx_credit = Decimal("0")
    xlsx_debit = Decimal("0")
    xlsx_amount = Decimal("0")

    for row in ws.iter_rows(min_row=2, values_only=True):
        # Summary rows have None or text in first column after data ends
        if row[0] is None:
            continue
        # Skip rows where col A looks like a label (summary section)
        if isinstance(row[0], str) and not re.match(r"\d{4}-\d{2}-\d{2}", row[0]):
            continue

        data_rows += 1
        desc = str(row[1]) if row[1] else ""
        if not is_internal_transfer(desc):
            # Credit = column D (index 3), Debit = column E (index 4), Amount = column G (index 6)
            if row[3] is not None:
                xlsx_credit += Decimal(str(row[3]))
            if row[4] is not None:
                xlsx_debit += Decimal(str(row[4]))
            if row[6] is not None:
                xlsx_amount += Decimal(str(row[6]))

    errors = []
    if data_rows != expected["count"]:
        errors.append(f"row count: got {data_rows}, expected {expected['count']}")

    if abs(float(xlsx_credit) - expected["total_credit"]) > 0.01:
        errors.append(
            f"total credit: got {float(xlsx_credit)}, expected {expected['total_credit']}"
        )

    if abs(float(xlsx_debit) - expected["total_debit"]) > 0.01:
        errors.append(
            f"total debit: got {float(xlsx_debit)}, expected {expected['total_debit']}"
        )

    if abs(float(xlsx_amount) - expected["net_amount"]) > 0.01:
        errors.append(
            f"net amount: got {float(xlsx_amount)}, expected {expected['net_amount']}"
        )

    if errors:
        for e in errors:
            print(f"FAIL XLSX: {e}", file=sys.stderr)
        return False

    print(f"OK XLSX: {data_rows} rows, credit/debit/net match", file=sys.stderr)
    return True


def main():
    parser = argparse.ArgumentParser(description="Verify extratos computations")
    parser.add_argument("--csv", required=True, help="Bradesco CSV file to parse")
    parser.add_argument("--go-json", help="Go search result JSON to compare")
    parser.add_argument("--xlsx", help="Go-exported XLSX file to verify")
    parser.add_argument("--print-json", action="store_true", help="Print results as JSON")
    args = parser.parse_args()

    transactions = parse_bradesco_csv(args.csv)
    agg = compute_aggregates(transactions)

    print(
        f"Python parsed: {agg['count']} transactions, "
        f"credit={agg['total_credit']:.2f}, debit={agg['total_debit']:.2f}, "
        f"net={agg['net_amount']:.2f}, dates={agg['min_date']}..{agg['max_date']}",
        file=sys.stderr,
    )

    if args.print_json:
        print(json.dumps(agg, indent=2))

    ok = True

    if args.go_json:
        if not verify_go_json(args.go_json, agg):
            ok = False

    if args.xlsx:
        if not verify_xlsx(args.xlsx, agg):
            ok = False

    if not ok:
        print("VERIFICATION FAILED", file=sys.stderr)
        sys.exit(1)

    print("All checks passed.", file=sys.stderr)


if __name__ == "__main__":
    main()
