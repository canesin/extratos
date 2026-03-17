#!/usr/bin/env python3
"""Verify extratos computations by independently parsing bank CSV/OFX files
and comparing with Go-produced results (JSON and/or XLSX).

Usage:
  python3 verify.py --csv testdata/synthetic_bradesco.csv
  python3 verify.py --csv testdata/synthetic_itau.csv --format itau
  python3 verify.py --csv testdata/synthetic_nubank.csv --format nubank
  python3 verify.py --csv testdata/synthetic_bb.ofx --format ofx
  python3 verify.py --csv testdata/synthetic_bradesco.csv --go-json /tmp/result.json
  python3 verify.py --csv testdata/synthetic_bradesco.csv --xlsx /tmp/export.xlsx
  python3 verify.py --multi file1.csv file2.csv ... --go-json /tmp/combined.json
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


def read_file(filepath):
    """Read file with UTF-8/Latin-1 fallback, normalize line endings."""
    with open(filepath, "rb") as f:
        raw = f.read()
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError:
        text = raw.decode("latin-1")
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    return text.split("\n")


def parse_bradesco_csv(filepath):
    """Parse Bradesco CSV, mimicking Go parser logic exactly."""
    lines = read_file(filepath)

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


def parse_itau_csv(filepath):
    """Parse Itaú CSV/TXT, mimicking Go ParseItau logic."""
    lines = read_file(filepath)

    date_re = re.compile(r"^(\d{2}/\d{2}/\d{4})")
    transactions = []
    account = "Itaú"

    # Detect account from first 5 lines
    for line in lines[:5]:
        lower = line.strip().lower()
        if "ag" in lower and "conta" in lower:
            account = line.strip()
            break

    i = 0
    while i < len(lines):
        line = lines[i].strip()
        if not line or line.lower().startswith("data;"):
            i += 1
            continue

        fields = line.split(";")
        if len(fields) < 3:
            i += 1
            continue

        dt = parse_br_date(fields[0].strip())
        if dt is None:
            i += 1
            continue

        desc = fields[1].strip()
        if desc in ("SALDO ANTERIOR", "SALDO DO DIA", ""):
            i += 1
            continue

        credit = None
        debit = None
        balance = None

        if len(fields) >= 6:
            # Full format: date;desc;doc;credit;debit;balance
            credit = parse_br_number(fields[3])
            debit = parse_br_number(fields[4])
            balance = parse_br_number(fields[5])
        elif len(fields) >= 3:
            # Simple format: date;desc;value
            val = parse_br_number(fields[2])
            if val is not None:
                if val >= 0:
                    credit = val
                else:
                    debit = val

        amount = credit if credit is not None else debit

        # Continuation lines
        while i + 1 < len(lines):
            nxt = lines[i + 1].strip()
            if nxt.startswith(";") and not nxt.startswith(";Total"):
                parts = nxt.split(";", 3)
                if len(parts) > 1:
                    extra = parts[1].strip()
                    if extra:
                        desc += " | " + extra
                i += 1
            else:
                break

        transactions.append(
            {
                "date": dt,
                "description": desc,
                "doc": "",
                "credit": credit,
                "debit": debit,
                "balance": balance,
                "amount": amount,
                "account": account,
            }
        )

        i += 1

    return transactions


def parse_nubank_csv(filepath):
    """Parse Nubank CSV: Data,Valor,Identificador,Descrição (UTF-8, YYYY-MM-DD)."""
    lines = read_file(filepath)

    transactions = []
    for i, line in enumerate(lines):
        line = line.strip()
        if not line:
            continue
        # Skip header
        if i == 0 and "data" in line.lower() and "valor" in line.lower():
            continue

        fields = line.split(",", 3)
        if len(fields) < 4:
            continue

        dt = fields[0].strip()
        if not re.match(r"\d{4}-\d{2}-\d{2}", dt):
            continue

        val = Decimal(fields[1].strip())
        desc = fields[3].strip()

        credit = val if val > 0 else None
        debit = val if val < 0 else None
        amount = val

        transactions.append(
            {
                "date": dt,
                "description": desc,
                "doc": fields[2].strip(),
                "credit": credit,
                "debit": debit,
                "balance": None,
                "amount": amount,
                "account": "Nubank",
            }
        )

    return transactions


def parse_ofx(filepath):
    """Parse OFX/SGML: extract <STMTTRN> blocks with DTPOSTED, TRNAMT, MEMO."""
    lines = read_file(filepath)
    text = "\n".join(lines)

    # Extract account info
    acctid_m = re.search(r"<ACCTID>([^<\n]+)", text)
    fid_m = re.search(r"<FID>([^<\n]+)", text)

    fid_banks = {
        "001": "Banco do Brasil",
        "104": "Caixa",
        "033": "Santander",
        "077": "Inter",
    }
    bank = "OFX"
    account = "OFX"
    if fid_m:
        fid = fid_m.group(1).strip()
        bank = fid_banks.get(fid, f"Bank {fid}")
        if acctid_m:
            account = f"Ag {fid} / {acctid_m.group(1).strip()}"

    # Extract transactions from STMTTRN blocks
    transactions = []
    blocks = re.findall(r"<STMTTRN>(.*?)</STMTTRN>", text, re.DOTALL)

    for block in blocks:
        dtposted_m = re.search(r"<DTPOSTED>(\d{8})", block)
        trnamt_m = re.search(r"<TRNAMT>([^<\n]+)", block)
        memo_m = re.search(r"<MEMO>([^<\n]+)", block)
        checknum_m = re.search(r"<CHECKNUM>([^<\n]+)", block)

        if not dtposted_m or not trnamt_m:
            continue

        raw_date = dtposted_m.group(1)
        dt = f"{raw_date[:4]}-{raw_date[4:6]}-{raw_date[6:8]}"

        val = Decimal(trnamt_m.group(1).strip())
        desc = memo_m.group(1).strip() if memo_m else ""
        doc = checknum_m.group(1).strip() if checknum_m else ""

        credit = val if val > 0 else None
        debit = val if val < 0 else None
        amount = val

        transactions.append(
            {
                "date": dt,
                "description": desc,
                "doc": doc,
                "credit": credit,
                "debit": debit,
                "balance": None,
                "amount": amount,
                "account": account,
            }
        )

    return transactions


INTERNAL_PREFIXES = [
    # Bradesco
    "Resgate Inv",
    "Resg.autom",
    "Resg/vencto",
    "Rent.inv",
    "Rentab.invest",
    "Apl.invest",
    "Aplicacao Cdb",
    "Aplicacao Inve",
    # Itaú
    "REND PAGO APLIC AUT",
    "RESGATE CDB",
    "APLICACAO CDB",
    # Banco do Brasil
    "APLICACAO POUPANCA",
    "RESGATE POUPANCA",
    "APLICACAO FUNDOS",
    "RESGATE FUNDOS",
    # Caixa
    "APL POUP",
    "RES POUP",
    # Nubank
    "Aplicação RDB",
    "Resgate RDB",
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


def detect_format(filepath):
    """Auto-detect file format from extension and content."""
    lower = filepath.lower()
    if lower.endswith(".ofx"):
        return "ofx"

    lines = read_file(filepath)
    first_line = lines[0].strip().lower() if lines else ""

    if "data,valor,identificador,descri" in first_line:
        return "nubank"

    if re.match(r"^extrato de: ag:", lines[0].strip(), re.IGNORECASE) if lines else False:
        return "bradesco"

    # Check for Itaú patterns
    text_lower = "\n".join(lines[:10]).lower()
    if "itaú" in text_lower or "itau" in text_lower:
        return "itau"

    # Check for DD/MM/YYYY dates (Itaú style)
    for line in lines[:10]:
        if re.match(r"^\d{2}/\d{2}/\d{4}", line.strip()):
            return "itau"

    # Default to Bradesco for semicolon-delimited with DD/MM/YY dates
    for line in lines[:20]:
        if re.match(r"^\d{2}/\d{2}/\d{2};", line.strip()):
            return "bradesco"

    return "bradesco"


PARSERS = {
    "bradesco": parse_bradesco_csv,
    "itau": parse_itau_csv,
    "nubank": parse_nubank_csv,
    "ofx": parse_ofx,
}


def parse_file(filepath, fmt=None):
    """Parse a file with auto-detection or explicit format."""
    if fmt is None:
        fmt = detect_format(filepath)
    parser = PARSERS.get(fmt)
    if parser is None:
        raise ValueError(f"Unknown format: {fmt}")
    return parser(filepath)


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
    parser.add_argument("--csv", help="Single file to parse (backward compat)")
    parser.add_argument(
        "--format",
        choices=["bradesco", "itau", "nubank", "ofx"],
        help="File format (default: auto-detect)",
    )
    parser.add_argument("--go-json", help="Go search result JSON to compare")
    parser.add_argument("--xlsx", help="Go-exported XLSX file to verify")
    parser.add_argument("--print-json", action="store_true", help="Print results as JSON")
    parser.add_argument(
        "--multi",
        nargs="+",
        help="Multiple files to parse and combine",
    )
    args = parser.parse_args()

    if not args.csv and not args.multi:
        parser.error("either --csv or --multi is required")

    all_transactions = []

    if args.multi:
        for fpath in args.multi:
            txns = parse_file(fpath)
            print(
                f"  {fpath}: {len(txns)} transactions",
                file=sys.stderr,
            )
            all_transactions.extend(txns)
    else:
        all_transactions = parse_file(args.csv, args.format)

    agg = compute_aggregates(all_transactions)

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
