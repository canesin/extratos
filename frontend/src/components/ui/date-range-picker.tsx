import * as React from "react";
import { format } from "date-fns";
import { ptBR } from "date-fns/locale";
import type { DateRange } from "react-day-picker";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";

interface DateRangePickerProps {
  from: string; // YYYY-MM-DD or ""
  to: string;   // YYYY-MM-DD or ""
  onChange: (from: string, to: string) => void;
  className?: string;
}

function parseDate(s: string): Date | undefined {
  if (!s) return undefined;
  const d = new Date(s + "T00:00:00");
  return isNaN(d.getTime()) ? undefined : d;
}

function toISO(d: Date | undefined): string {
  if (!d) return "";
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function DateRangePicker({ from, to, onChange, className }: DateRangePickerProps) {
  const [open, setOpen] = React.useState(false);

  const range: DateRange = {
    from: parseDate(from),
    to: parseDate(to),
  };

  const hasRange = from || to;

  const label = hasRange
    ? [
        range.from ? format(range.from, "dd/MM/yy", { locale: ptBR }) : "...",
        range.to ? format(range.to, "dd/MM/yy", { locale: ptBR }) : "...",
      ].join(" – ")
    : "Período";

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn(
            "h-8 text-xs font-normal gap-1.5",
            !hasRange && "text-muted-foreground",
            className
          )}
        >
          <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth={2}>
            <rect x="3" y="4" width="18" height="18" rx="2" />
            <path d="M16 2v4M8 2v4M3 10h18" />
          </svg>
          {label}
          {hasRange && (
            <span
              role="button"
              className="ml-0.5 hover:text-foreground cursor-pointer"
              onClick={(e) => {
                e.stopPropagation();
                onChange("", "");
              }}
            >
              &#x2715;
            </span>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="end" collisionPadding={16}>
        <Calendar
          mode="range"
          defaultMonth={range.from}
          selected={range}
          onSelect={(r) => {
            onChange(toISO(r?.from), toISO(r?.to));
          }}
          numberOfMonths={2}
          locale={ptBR}
        />
      </PopoverContent>
    </Popover>
  );
}
