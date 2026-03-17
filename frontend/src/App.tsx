import { useState, useEffect, useCallback, useRef } from "react";
import {
  AppService,
  SearchResult,
  ImportPreview,
  FilePreview,
  DBInfo,
  ClauseSummary,
  MonthlySummary,
} from "../bindings/extratos-app";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { formatBRL } from "@/lib/utils";
import { sortTransactions, type SortKey, type SortDir } from "@/lib/sort";

interface Stats {
  total_transactions: number;
  banks: string[] | null;
  accounts: string[] | null;
  min_date: string;
  max_date: string;
}

const PAGE_SIZE = 50;

// ─── DB Selection Screen ──────────────────────────────────────────────

function DBSelectScreen({ onOpen }: { onOpen: () => void }) {
  const [databases, setDatabases] = useState<DBInfo[]>([]);
  const [newName, setNewName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [renamingDB, setRenamingDB] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [deletingDB, setDeletingDB] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    const dbs = await AppService.ListDatabases();
    setDatabases(dbs || []);
    setLoading(false);
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const openDB = async (name: string) => {
    setError("");
    const err = await AppService.OpenDatabase(name);
    if (err) {
      setError(err);
    } else {
      onOpen();
    }
  };

  const createDB = async () => {
    const name = newName.trim();
    if (!name) return;
    setError("");
    const err = await AppService.CreateDatabase(name);
    if (err) {
      setError(err);
    } else {
      onOpen();
    }
  };

  const handleRename = async (oldName: string) => {
    const trimmed = renameValue.trim();
    if (!trimmed || trimmed === oldName) {
      setRenamingDB(null);
      return;
    }
    setError("");
    const err = await AppService.RenameDatabase(oldName, trimmed);
    if (err) {
      setError(err);
    } else {
      setRenamingDB(null);
      refresh();
    }
  };

  const handleDelete = async (name: string) => {
    setError("");
    const err = await AppService.DeleteDatabase(name);
    if (err) {
      setError(err);
    } else {
      setDeletingDB(null);
      refresh();
    }
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-muted/30">
      <div className="w-full max-w-lg mx-4">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-foreground">Extratos</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Selecione ou crie um banco de dados
          </p>
        </div>

        {error && (
          <div className="mb-4 px-4 py-2 rounded-md text-sm bg-red-50 text-red-800 border border-red-200">
            {error}
          </div>
        )}

        {/* Existing databases */}
        <div className="rounded-lg border bg-white shadow-sm">
          {loading ? (
            <div className="p-8 text-center text-muted-foreground">
              Carregando...
            </div>
          ) : databases.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              Nenhum banco de dados encontrado. Crie um novo abaixo.
            </div>
          ) : (
            <ul className="divide-y">
              {databases.map((db) => (
                <li key={db.name}>
                  {/* Delete confirmation */}
                  {deletingDB === db.name ? (
                    <div className="px-4 py-3 bg-red-50 border-l-4 border-red-400">
                      <div className="text-sm text-red-900 mb-2">
                        Excluir <strong>{db.name}</strong>? Esta ação não pode
                        ser desfeita.
                      </div>
                      <div className="flex gap-2">
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => handleDelete(db.name)}
                        >
                          Excluir
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setDeletingDB(null)}
                        >
                          Cancelar
                        </Button>
                      </div>
                    </div>
                  ) : renamingDB === db.name ? (
                    /* Rename inline edit */
                    <div className="px-4 py-3">
                      <div className="flex gap-2">
                        <Input
                          autoFocus
                          value={renameValue}
                          onChange={(e) => setRenameValue(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") handleRename(db.name);
                            if (e.key === "Escape") setRenamingDB(null);
                          }}
                          className="flex-1 h-8 text-sm"
                        />
                        <Button
                          size="sm"
                          onClick={() => handleRename(db.name)}
                          disabled={
                            !renameValue.trim() ||
                            renameValue.trim() === db.name
                          }
                        >
                          Salvar
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setRenamingDB(null)}
                        >
                          Cancelar
                        </Button>
                      </div>
                    </div>
                  ) : (
                    /* Normal row */
                    <div className="flex items-center hover:bg-muted/50 transition-colors">
                      <button
                        className="flex-1 px-4 py-3 flex items-center justify-between text-left cursor-pointer"
                        onClick={() => openDB(db.name)}
                      >
                        <div>
                          <div className="font-medium text-foreground">
                            {db.name}
                          </div>
                          <div className="text-xs text-muted-foreground mt-0.5">
                            {formatSize(db.size_bytes)} &middot;{" "}
                            {db.modified_at}
                          </div>
                        </div>
                        <svg
                          className="w-4 h-4 text-muted-foreground"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M9 5l7 7-7 7"
                          />
                        </svg>
                      </button>
                      {/* Action buttons */}
                      <div className="flex pr-2 gap-1">
                        <button
                          className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                          title="Renomear"
                          onClick={(e) => {
                            e.stopPropagation();
                            setRenamingDB(db.name);
                            setRenameValue(db.name);
                            setDeletingDB(null);
                          }}
                        >
                          <svg
                            className="w-3.5 h-3.5"
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                          >
                            <path
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth={2}
                              d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                            />
                          </svg>
                        </button>
                        <button
                          className="p-1.5 rounded hover:bg-red-100 text-muted-foreground hover:text-red-700 transition-colors cursor-pointer"
                          title="Excluir"
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeletingDB(db.name);
                            setRenamingDB(null);
                          }}
                        >
                          <svg
                            className="w-3.5 h-3.5"
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                          >
                            <path
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth={2}
                              d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                            />
                          </svg>
                        </button>
                      </div>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Create new */}
        <div className="mt-4 rounded-lg border bg-white shadow-sm p-4">
          <div className="text-sm font-medium text-foreground mb-2">
            Criar novo banco de dados
          </div>
          <div className="flex gap-2">
            <Input
              placeholder="Nome (ex: Empresa X, Pessoal)"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && createDB()}
              className="flex-1"
            />
            <Button onClick={createDB} disabled={!newName.trim()}>
              Criar
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ─── Import Preview Dialog ────────────────────────────────────────────

function ImportPreviewDialog({
  preview,
  onConfirm,
  onCancel,
  confirming,
}: {
  preview: ImportPreview;
  onConfirm: () => void;
  onCancel: () => void;
  confirming: boolean;
}) {
  const [expandedFile, setExpandedFile] = useState<number | null>(
    preview.files.length === 1 ? 0 : null
  );

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !confirming) onCancel();
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [confirming, onCancel]);

  const totalCount = preview.files.reduce(
    (sum, f) => sum + (f.error ? 0 : f.count),
    0
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-white rounded-lg shadow-xl max-w-4xl w-full mx-4 max-h-[85vh] flex flex-col">
        {/* Header */}
        <div className="px-6 py-4 border-b flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold">
              Pré-visualização da importação
            </h2>
            <p className="text-sm text-muted-foreground">
              {preview.files.length} arquivo
              {preview.files.length > 1 ? "s" : ""} &middot; {totalCount}{" "}
              transações
            </p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={onCancel} disabled={confirming}>
              Cancelar
            </Button>
            <Button onClick={onConfirm} disabled={confirming || totalCount === 0}>
              {confirming ? "Importando..." : "Confirmar importação"}
            </Button>
          </div>
        </div>

        {/* File list */}
        <div className="flex-1 overflow-auto p-4 space-y-3">
          {preview.files.map((file, idx) => (
            <FilePreviewCard
              key={idx}
              file={file}
              expanded={expandedFile === idx}
              onToggle={() =>
                setExpandedFile(expandedFile === idx ? null : idx)
              }
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function FilePreviewCard({
  file,
  expanded,
  onToggle,
}: {
  file: FilePreview;
  expanded: boolean;
  onToggle: () => void;
}) {
  if (file.error) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3">
        <div className="font-medium text-red-900">{file.filename}</div>
        <div className="text-sm text-red-700 mt-1">{file.error}</div>
      </div>
    );
  }

  const previewTxns = expanded
    ? file.transactions.slice(0, 50)
    : file.transactions.slice(0, 5);

  return (
    <div className="rounded-lg border bg-white">
      {/* File summary header */}
      <button
        className="w-full px-4 py-3 flex items-center justify-between hover:bg-muted/30 transition-colors text-left cursor-pointer"
        onClick={onToggle}
      >
        <div className="flex items-center gap-3">
          <div>
            <div className="font-medium">{file.filename}</div>
            <div className="text-xs text-muted-foreground mt-0.5">
              <Badge variant="secondary" className="mr-2">
                {file.bank}
              </Badge>
              {file.account && (
                <span className="mr-2">{file.account}</span>
              )}
              {file.count} transações &middot; {file.date_range}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-4 text-sm">
          <span className="text-green-700">
            {formatBRL(file.total_credit)}
          </span>
          <span className="text-red-700">{formatBRL(file.total_debit)}</span>
          <svg
            className={`w-4 h-4 text-muted-foreground transition-transform ${
              expanded ? "rotate-180" : ""
            }`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M19 9l-7 7-7-7"
            />
          </svg>
        </div>
      </button>

      {/* Transaction preview table */}
      {expanded && (
        <div className="border-t px-4 py-2 overflow-auto max-h-80">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-muted-foreground">
                <th className="text-left py-1 px-2 font-medium">Data</th>
                <th className="text-left py-1 px-2 font-medium">Descrição</th>
                <th className="text-right py-1 px-2 font-medium">Crédito</th>
                <th className="text-right py-1 px-2 font-medium">Débito</th>
                <th className="text-right py-1 px-2 font-medium">Saldo</th>
              </tr>
            </thead>
            <tbody>
              {previewTxns.map((t, i) => (
                <tr key={i} className="border-t border-border/30">
                  <td className="py-1 px-2 whitespace-nowrap font-mono">
                    {t.date}
                  </td>
                  <td className="py-1 px-2 max-w-sm truncate" title={t.description}>
                    <span className="inline-flex items-center gap-1">
                      {t.description}
                      {t.is_internal && (
                        <Badge variant="outline" className="text-[9px] px-0.5 py-0 border-amber-400 text-amber-700 font-normal shrink-0">
                          interno
                        </Badge>
                      )}
                    </span>
                  </td>
                  <td className="py-1 px-2 text-right text-green-700 font-mono">
                    {formatBRL(t.credit)}
                  </td>
                  <td className="py-1 px-2 text-right text-red-700 font-mono">
                    {formatBRL(t.debit)}
                  </td>
                  <td className="py-1 px-2 text-right font-mono">
                    {formatBRL(t.balance)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {file.count > previewTxns.length && (
            <div className="text-center text-xs text-muted-foreground py-2">
              ... e mais {file.count - previewTxns.length} transações
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Column Sorting ──────────────────────────────────────────────────

function SortIcon({ active, dir }: { active: boolean; dir: SortDir }) {
  if (!active)
    return (
      <svg
        className="inline w-3 h-3 ml-1 opacity-0 group-hover:opacity-40"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M7 10l5-5 5 5M7 14l5 5 5-5"
        />
      </svg>
    );
  return (
    <svg
      className="inline w-3 h-3 ml-1 opacity-70"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2.5}
        d={dir === "asc" ? "M7 14l5-5 5 5" : "M7 10l5 5 5-5"}
      />
    </svg>
  );
}

// ─── Main App Screen ──────────────────────────────────────────────────

function MainScreen({ onSwitchDB }: { onSwitchDB: () => void }) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchResult>(
    new SearchResult()
  );
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [message, setMessage] = useState("");
  const [page, setPage] = useState(0);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();
  const [sortKey, setSortKey] = useState<SortKey | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  const [dbName, setDbName] = useState("");

  // Date range filter state
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  // Internal transfer filter state
  const [showInternal, setShowInternal] = useState<"" | "external" | "internal">("");

  // Monthly summary state
  const [showMonthly, setShowMonthly] = useState(false);
  const [monthlySummary, setMonthlySummary] = useState<MonthlySummary[]>([]);

  // Import preview state
  const [importPreview, setImportPreview] = useState<ImportPreview | null>(
    null
  );
  const [previewing, setPreviewing] = useState(false);
  const [confirming, setConfirming] = useState(false);

  const [dbError, setDbError] = useState("");

  const loadStats = useCallback(async () => {
    const s = await AppService.GetStats();
    setStats(s as Stats);
  }, []);

  const doSearch = useCallback(async (q: string, p: number, df: string = dateFrom, dt: string = dateTo, si: string = showInternal) => {
    setLoading(true);
    try {
      const r = await AppService.SearchFiltered(q, PAGE_SIZE, p * PAGE_SIZE, df, dt, si);
      if (r) setResults(r);
    } finally {
      setLoading(false);
    }
  }, [dateFrom, dateTo, showInternal]);

  useEffect(() => {
    AppService.GetDBError().then((err: string) => {
      if (err) setDbError(err);
    });
    AppService.GetCurrentDB().then((name: string) => setDbName(name));
    loadStats();
    // Initial load — call API directly to avoid depending on doSearch
    // (doSearch changes when dateFrom/dateTo/showInternal change, which would re-trigger this effect)
    AppService.SearchFiltered("", PAGE_SIZE, 0, "", "", "").then(r => {
      if (r) setResults(r);
    });
  }, [loadStats]);

  const onQueryChange = (value: string) => {
    setQuery(value);
    setPage(0);
    setSortKey(null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => doSearch(value, 0), 250);
  };

  const onDateChange = (df: string, dt: string) => {
    setDateFrom(df);
    setDateTo(dt);
    setPage(0);
    setSortKey(null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => doSearch(query, 0, df, dt), 250);
  };

  const onInternalChange = (val: "" | "external" | "internal") => {
    setShowInternal(val);
    setPage(0);
    setSortKey(null);
    doSearch(query, 0, dateFrom, dateTo, val);
  };

  // --- Import preview flow ---
  const handleImportStart = async () => {
    setPreviewing(true);
    setMessage("");
    try {
      const preview = await AppService.PreviewImport();
      if (!preview) {
        // User cancelled file dialog
        return;
      }
      if (preview.error) {
        setMessage(`err:${preview.error}`);
        return;
      }
      setImportPreview(preview);
    } catch (e: any) {
      setMessage(`err:${e}`);
    } finally {
      setPreviewing(false);
    }
  };

  const handleImportConfirm = async () => {
    setConfirming(true);
    try {
      const result = await AppService.ConfirmImport();
      setImportPreview(null);
      if (!result) {
        setMessage("err:Erro desconhecido");
        return;
      }
      if (result.error) {
        setMessage(`err:${result.error}`);
      } else {
        setMessage(
          `ok:${result.bank}: ${result.inserted} transações importadas` +
            (result.skipped > 0
              ? `, ${result.skipped} duplicadas ignoradas`
              : "")
        );
        loadStats();
        doSearch(query, 0);
      }
    } catch (e: any) {
      setMessage(`err:${e}`);
      setImportPreview(null);
    } finally {
      setConfirming(false);
    }
  };

  const handleImportCancel = () => {
    AppService.CancelImport();
    setImportPreview(null);
  };

  const handleToggleInternal = async (id: number) => {
    const err = await AppService.ToggleInternal(id);
    if (err) {
      setMessage(`err:${err}`);
    } else {
      doSearch(query, page);
    }
  };

  // Load monthly summary when toggled or search results change
  useEffect(() => {
    if (showMonthly) {
      AppService.GetMonthlySummary(query, dateFrom, dateTo, showInternal).then(setMonthlySummary);
    }
  }, [showMonthly, results, query, dateFrom, dateTo, showInternal]);

  const handleExport = async () => {
    setExporting(true);
    setMessage("");
    try {
      const msg = await AppService.ExportResults(query, showInternal);
      if (msg.startsWith("Erro") || msg.startsWith("Nenhuma")) {
        setMessage(`err:${msg}`);
      } else {
        setMessage(`ok:${msg}`);
      }
    } catch (e: any) {
      setMessage(`err:${e}`);
    } finally {
      setExporting(false);
    }
  };

  const totalPages = Math.ceil(results.total / PAGE_SIZE);

  const goPage = (p: number) => {
    setPage(p);
    setSortKey(null);
    doSearch(query, p);
  };

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir(sortDir === "asc" ? "desc" : "asc");
    } else {
      setSortKey(key);
      setSortDir(key === "date" ? "desc" : "asc");
    }
  };

  const sortedTxns = sortTransactions(results.transactions, sortKey, sortDir);

  const isError = message.startsWith("err:");
  const messageText = message.replace(/^(ok:|err:)/, "");

  return (
    <div className="min-h-screen flex flex-col">
      {/* Import preview overlay */}
      {importPreview && (
        <ImportPreviewDialog
          preview={importPreview}
          onConfirm={handleImportConfirm}
          onCancel={handleImportCancel}
          confirming={confirming}
        />
      )}

      {/* Header */}
      <header className="border-b bg-white px-4 py-3">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold text-foreground">Extratos</h1>
              <button
                onClick={onSwitchDB}
                className="text-xs text-muted-foreground hover:text-foreground transition-colors px-2 py-0.5 rounded border border-transparent hover:border-border cursor-pointer"
                title="Trocar banco de dados"
              >
                {dbName}
              </button>
            </div>
            {stats && stats.total_transactions > 0 && (
              <p className="text-sm text-muted-foreground mt-0.5">
                {stats.total_transactions.toLocaleString("pt-BR")} transações
                {stats.min_date &&
                  ` · ${stats.min_date} a ${stats.max_date}`}
                {stats.banks &&
                  stats.banks.length > 0 &&
                  stats.banks.map((b) => (
                    <Badge key={b} variant="secondary" className="ml-1">
                      {b}
                    </Badge>
                  ))}
              </p>
            )}
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={handleImportStart}
              disabled={previewing}
            >
              {previewing ? "Carregando..." : "Importar extratos"}
            </Button>
            <Button
              onClick={handleExport}
              disabled={exporting || results.total === 0}
            >
              {exporting
                ? "Exportando..."
                : `Exportar XLSX${query ? " (filtrado)" : ""}`}
            </Button>
          </div>
        </div>
      </header>

      {/* Search bar */}
      <div className="px-4 py-2 bg-white border-b space-y-2">
        <div className="flex items-center gap-2">
          <div className="relative flex-1">
            <svg
              className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
            <Input
              placeholder="Buscar (separe com vírgula para múltiplos)..."
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              className="pl-10 h-9"
            />
          </div>
          <span className="text-xs text-muted-foreground whitespace-nowrap">
            {results.total.toLocaleString("pt-BR")} resultado{results.total !== 1 ? "s" : ""}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Input type="date" value={dateFrom} onChange={(e) => onDateChange(e.target.value, dateTo)} className="w-32 h-8 text-xs" />
          <span className="text-muted-foreground text-xs">a</span>
          <Input type="date" value={dateTo} onChange={(e) => onDateChange(dateFrom, e.target.value)} className="w-32 h-8 text-xs" />
          {(dateFrom || dateTo) && (
            <button onClick={() => onDateChange("", "")} className="text-muted-foreground hover:text-foreground cursor-pointer text-xs" title="Limpar datas">&#x2715;</button>
          )}
          <div className="flex-1" />
          <div className="flex border rounded-md overflow-hidden text-xs">
            {([["", "Todas"], ["external", "Externas"], ["internal", "Internas"]] as ["" | "external" | "internal", string][]).map(([val, label]) => (
              <button
                key={val}
                onClick={() => onInternalChange(val)}
                className={`px-2 py-1 cursor-pointer transition-colors ${
                  showInternal === val
                    ? "bg-foreground text-white"
                    : "hover:bg-muted/50"
                }`}
              >
                {label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Bank/account quick filter */}
      {stats && ((stats.banks && stats.banks.length > 1) || (stats.accounts && stats.accounts.length > 1)) && (
        <div className="px-4 py-1.5 bg-white border-b flex items-center gap-2 flex-wrap">
          <span className="text-xs text-muted-foreground mr-1">Filtrar:</span>
          {stats.banks?.map(b => (
            <button key={b} onClick={() => onQueryChange(b)}
              className={`text-xs px-2 py-0.5 rounded border cursor-pointer transition-colors ${query === b ? 'bg-foreground text-white border-foreground' : 'hover:bg-muted/50 border-border'}`}>
              {b}
            </button>
          ))}
          {stats.accounts && stats.accounts.length > 1 && stats.accounts.map(a => (
            <button key={a} onClick={() => onQueryChange(a)}
              className={`text-xs px-2 py-0.5 rounded border cursor-pointer transition-colors ${query === a ? 'bg-foreground text-white border-foreground' : 'hover:bg-muted/50 border-border'}`}>
              {a}
            </button>
          ))}
          {query && (
            <button onClick={() => onQueryChange("")}
              className="text-xs text-muted-foreground hover:text-foreground cursor-pointer ml-1">
              Limpar
            </button>
          )}
        </div>
      )}

      {/* Overall summary bar */}
      {results.total > 0 && (
        <div className="mx-4 mt-3 grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div className="rounded-lg border bg-white px-3 py-2">
            <div className="text-[11px] text-muted-foreground">Transações</div>
            <div className="text-base font-semibold">
              {results.total.toLocaleString("pt-BR")}
            </div>
            {results.min_date && (
              <div className="text-[11px] text-muted-foreground">
                {results.min_date} — {results.max_date}
              </div>
            )}
            {results.internal_count > 0 && showInternal === "" && (
              <div className="text-[11px] text-amber-600 mt-0.5">
                ({results.internal_count} interna{results.internal_count !== 1 ? "s" : ""} excluída{results.internal_count !== 1 ? "s" : ""} dos totais)
              </div>
            )}
          </div>
          <div className="rounded-lg border bg-white px-3 py-2">
            <div className="text-[11px] text-muted-foreground">Total Créditos</div>
            <div className="text-base font-semibold text-green-700">
              {formatBRL(results.total_credit)}
            </div>
          </div>
          <div className="rounded-lg border bg-white px-3 py-2">
            <div className="text-[11px] text-muted-foreground">Total Débitos</div>
            <div className="text-base font-semibold text-red-700">
              {formatBRL(results.total_debit)}
            </div>
          </div>
          <div className="rounded-lg border bg-white px-3 py-2">
            <div className="text-[11px] text-muted-foreground">Saldo Líquido</div>
            <div
              className={`text-base font-semibold ${results.net_amount >= 0 ? "text-green-700" : "text-red-700"}`}
            >
              {formatBRL(results.net_amount)}
            </div>
          </div>
        </div>
      )}

      {/* Monthly summary toggle */}
      {results.total > 0 && (
        <>
          <div className="mx-4 mt-2 flex justify-end">
            <button onClick={() => setShowMonthly(!showMonthly)} className="text-xs text-muted-foreground hover:text-foreground cursor-pointer">
              {showMonthly ? "Ocultar resumo mensal" : "Resumo mensal \u25BE"}
            </button>
          </div>
          {showMonthly && monthlySummary.length > 0 && (
            <div className="mx-4 mt-2 rounded-lg border bg-white overflow-auto max-h-72">
              <table className="w-full text-xs">
                <thead className="sticky top-0 bg-muted/80 backdrop-blur-sm">
                  <tr>
                    <th className="text-left py-2 px-3 font-medium text-muted-foreground">M&#xEA;s</th>
                    <th className="text-right py-2 px-3 font-medium text-muted-foreground">Transa&#xE7;&#xF5;es</th>
                    <th className="text-right py-2 px-3 font-medium text-muted-foreground">Cr&#xE9;ditos</th>
                    <th className="text-right py-2 px-3 font-medium text-muted-foreground">D&#xE9;bitos</th>
                    <th className="text-right py-2 px-3 font-medium text-muted-foreground">Saldo</th>
                  </tr>
                </thead>
                <tbody>
                  {monthlySummary.map((ms) => (
                    <tr key={ms.month} className="border-t border-border/50 hover:bg-muted/50 transition-colors">
                      <td className="py-1.5 px-3 font-mono">{ms.month}</td>
                      <td className="py-1.5 px-3 text-right">{ms.count}</td>
                      <td className="py-1.5 px-3 text-right text-green-700 font-mono">{formatBRL(ms.total_credit)}</td>
                      <td className="py-1.5 px-3 text-right text-red-700 font-mono">{formatBRL(ms.total_debit)}</td>
                      <td className={`py-1.5 px-3 text-right font-mono font-semibold ${ms.net_amount >= 0 ? "text-green-700" : "text-red-700"}`}>{formatBRL(ms.net_amount)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      {/* Per-clause summaries */}
      {results.clause_summaries && results.clause_summaries.length > 0 && (
        <div className="mx-4 mt-3 space-y-2">
          <div className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
            Por termo de busca
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
            {results.clause_summaries.map((cs: ClauseSummary, idx: number) => (
              <div
                key={idx}
                className="rounded-lg border bg-white px-3 py-2 flex items-center justify-between"
              >
                <div>
                  <div className="font-medium text-sm">{cs.clause}</div>
                  <div className="text-xs text-muted-foreground">
                    {cs.total} transações
                    {cs.min_date && ` · ${cs.min_date} — ${cs.max_date}`}
                  </div>
                </div>
                <div className="text-right text-xs space-y-0.5">
                  <div className="text-green-700 font-mono">
                    {formatBRL(cs.total_credit)}
                  </div>
                  <div className="text-red-700 font-mono">
                    {formatBRL(cs.total_debit)}
                  </div>
                  <div
                    className={`font-mono font-semibold ${cs.net_amount >= 0 ? "text-green-700" : "text-red-700"}`}
                  >
                    {formatBRL(cs.net_amount)}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* DB init error */}
      {dbError && (
        <div className="mx-4 mt-3 px-4 py-3 rounded-md text-sm bg-red-50 text-red-900 border border-red-300 font-mono whitespace-pre-wrap select-all">
          <strong>Erro de inicialização do banco de dados:</strong>
          <br />
          {dbError}
        </div>
      )}

      {/* Status message */}
      {message && (
        <div
          className={`mx-6 mt-3 px-4 py-2 rounded-md text-sm ${
            isError
              ? "bg-red-50 text-red-800 border border-red-200"
              : "bg-green-50 text-green-800 border border-green-200"
          }`}
        >
          {messageText}
          <button
            className="float-right text-current opacity-50 hover:opacity-100 cursor-pointer"
            onClick={() => setMessage("")}
          >
            ✕
          </button>
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto px-4 py-2">
        <table className="w-full text-sm border-collapse">
          <thead className="sticky top-0 bg-muted/80 backdrop-blur-sm z-10">
            <tr>
              {(
                [
                  ["date", "Data", "text-left"],
                  ["description", "Descrição", "text-left"],
                  ["doc", "Doc", "text-left"],
                  ["credit", "Crédito", "text-right"],
                  ["debit", "Débito", "text-right"],
                  ["balance", "Saldo", "text-right"],
                  ["account", "Conta", "text-left"],
                  ["bank", "Banco", "text-left"],
                ] as [SortKey, string, string][]
              ).map(([key, label, align]) => (
                <th
                  key={key}
                  className={`${align} py-1.5 px-2 font-medium text-muted-foreground cursor-pointer select-none hover:text-foreground transition-colors group text-xs`}
                  onClick={() => toggleSort(key)}
                >
                  {label}
                  <SortIcon active={sortKey === key} dir={sortDir} />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {loading && sortedTxns.length === 0 ? (
              <tr>
                <td
                  colSpan={8}
                  className="text-center py-8 text-muted-foreground"
                >
                  Carregando...
                </td>
              </tr>
            ) : sortedTxns.length === 0 ? (
              <tr>
                <td
                  colSpan={8}
                  className="text-center py-8 text-muted-foreground"
                >
                  {stats?.total_transactions === 0
                    ? 'Nenhuma transação importada. Clique em "Importar extratos" para começar.'
                    : "Nenhum resultado encontrado."}
                </td>
              </tr>
            ) : (
              sortedTxns.map((t) => (
                <tr
                  key={t.id}
                  className={`border-b border-border/50 hover:bg-muted/50 transition-colors ${t.is_internal ? "bg-amber-50/50" : ""}`}
                >
                  <td className="py-1.5 px-2 whitespace-nowrap font-mono text-xs">
                    {t.date}
                  </td>
                  <td
                    className="py-1.5 px-2 max-w-[260px] truncate"
                    title={t.description}
                  >
                    <span className="inline-flex items-center gap-1">
                      {t.description}
                      {t.is_internal && (
                        <Badge variant="outline" className="text-[9px] px-0.5 py-0 border-amber-400 text-amber-700 font-normal shrink-0">
                          int
                        </Badge>
                      )}
                    </span>
                  </td>
                  <td className="py-1.5 px-2 text-muted-foreground text-xs">
                    {t.doc}
                  </td>
                  <td className="py-1.5 px-2 text-right whitespace-nowrap text-green-700 font-mono text-xs">
                    {formatBRL(t.credit)}
                  </td>
                  <td className="py-1.5 px-2 text-right whitespace-nowrap text-red-700 font-mono text-xs">
                    {formatBRL(t.debit)}
                  </td>
                  <td className="py-1.5 px-2 text-right whitespace-nowrap font-mono text-xs">
                    {formatBRL(t.balance)}
                  </td>
                  <td className="py-1.5 px-2 text-xs text-muted-foreground">
                    {t.account}
                  </td>
                  <td className="py-1.5 px-2">
                    <div className="flex items-center gap-1">
                      <Badge variant="secondary" className="text-xs">
                        {t.bank}
                      </Badge>
                      <button
                        onClick={() => handleToggleInternal(t.id)}
                        className={`p-0.5 rounded cursor-pointer transition-colors ${
                          t.is_internal
                            ? "text-amber-600 hover:text-amber-800 hover:bg-amber-100"
                            : "text-muted-foreground/30 hover:text-muted-foreground hover:bg-muted"
                        }`}
                        title={t.is_internal ? "Marcar como externa" : "Marcar como interna"}
                      >
                        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                            d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
                        </svg>
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <footer className="border-t bg-white px-4 py-2 flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            Página {page + 1} de {totalPages}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page === 0}
              onClick={() => goPage(page - 1)}
            >
              Anterior
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages - 1}
              onClick={() => goPage(page + 1)}
            >
              Próxima
            </Button>
          </div>
        </footer>
      )}
    </div>
  );
}

// ─── App Router ───────────────────────────────────────────────────────

function App() {
  const [screen, setScreen] = useState<"db-select" | "main">("db-select");

  if (screen === "db-select") {
    return <DBSelectScreen onOpen={() => setScreen("main")} />;
  }

  return <MainScreen onSwitchDB={() => setScreen("db-select")} />;
}

export default App;
