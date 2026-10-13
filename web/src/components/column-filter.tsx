import { useState } from "react"
import { ListFilter, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { parseBytes } from "@/lib/format"
import { cn } from "@/lib/utils"

/** "From" and "to" of a numeric or date column, as typed: empty means no bound. */
export type Range = { min: string; max: string }

export const noRange: Range = { min: "", max: "" }

/** True when a filter value restricts the rows: some text, a ticked option or a bound. */
export function isActive(v: unknown): boolean {
  if (typeof v === "string") return v.trim() !== ""
  if (Array.isArray(v)) return v.length > 0
  if (v && typeof v === "object") return Object.values(v).some(isActive)
  return false
}

/**
 * The filters of the columns of a table, one entry per filter. `onChange` runs on every change,
 * to go back to the first page.
 */
export function useColumnFilters<T extends object>(empty: T, onChange?: () => void) {
  const [filters, setFilters] = useState<T>(empty)
  const set = <K extends keyof T>(key: K, value: T[K]) => {
    setFilters((f) => ({ ...f, [key]: value }))
    onChange?.()
  }
  const clear = (...keys: (keyof T)[]) => {
    setFilters((f) => {
      if (keys.length === 0) return empty
      const next = { ...f }
      for (const k of keys) next[k] = empty[k]
      return next
    })
    onChange?.()
  }
  const active = (Object.keys(empty) as (keyof T)[]).filter((k) => isActive(filters[k])).length
  return { filters, set, clear, active }
}

/** Case-insensitive "contains" on any of the fields; an empty query matches everything. */
export function textMatch(query: string, ...fields: string[]) {
  const q = query.trim().toLowerCase()
  return !q || fields.some((f) => f.toLowerCase().includes(q))
}

/** No ticked option matches everything; otherwise the row needs one of them. */
export function optionsMatch(selected: string[], ...values: string[]) {
  return selected.length === 0 || values.some((v) => selected.includes(v))
}

/** A whole number, or null when it isn't one. */
export function parseCount(s: string): number | null {
  return /^\s*\d+\s*$/.test(s) ? Number(s) : null
}

/** "1.5 GB", "700mb" or a plain byte count, or null when it isn't one. */
export function parseSize(s: string): number | null {
  return /^\d[\d.,]*\s*[kmgtp]?(i?b)?$/i.test(s.trim()) ? parseBytes(s) : null
}

/** Bounds that don't parse are ignored (and the field shows them as invalid). */
export function rangeMatch(v: number, r: Range, parse: (s: string) => number | null) {
  const min = r.min.trim() ? parse(r.min) : null
  const max = r.max.trim() ? parse(r.max) : null
  return (min === null || v >= min) && (max === null || v <= max)
}

/** The local day of a timestamp, "2026-10-13", as given by <input type="date">. */
function localDay(iso: string) {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ""
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** Both days are included; rows without a date pass only when there are no bounds. */
export function dateMatch(iso: string | null | undefined, r: Range) {
  if (!r.min && !r.max) return true
  const day = iso ? localDay(iso) : ""
  if (!day) return false
  return (!r.min || day >= r.min) && (!r.max || day <= r.max)
}

/** The funnel next to a column title, opening the filter of that column. */
export function ColumnFilter({
  label,
  active,
  onClear,
  align = "start",
  children,
}: {
  label: string
  active: boolean
  onClear: () => void
  align?: "start" | "center" | "end"
  children: React.ReactNode
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Filtra ${label}`}
          title={`Filtra ${label}`}
          className={cn(
            "size-6 shrink-0",
            active ? "text-primary bg-primary/10 hover:bg-primary/20" : "text-muted-foreground/70 hover:text-foreground"
          )}
        >
          <ListFilter className="size-3.5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align={align} className="grid w-64 gap-3 p-3 font-normal">
        <div className="flex h-6 items-center justify-between gap-2">
          <span className="text-sm font-medium">Filtra: {label}</span>
          {active && (
            <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={onClear}>
              <X /> Azzera
            </Button>
          )}
        </div>
        {children}
      </PopoverContent>
    </Popover>
  )
}

/** "Contiene…" on the text of a column. */
export function TextFilter({
  value,
  onChange,
  placeholder = "Contiene…",
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  return <Input autoFocus value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="h-8" />
}

export type FilterOption = { value: string; label: React.ReactNode }

/** Checkboxes: the row passes if it matches any ticked option. */
export function OptionsFilter({
  title,
  options,
  value,
  onChange,
}: {
  title?: string
  options: FilterOption[]
  value: string[]
  onChange: (v: string[]) => void
}) {
  return (
    <div className="grid gap-1">
      {title && <div className="text-muted-foreground px-1.5 text-xs font-medium">{title}</div>}
      {options.length === 0 && <span className="text-muted-foreground px-1.5 text-xs">Nessuna voce</span>}
      <div className="grid max-h-56 gap-0.5 overflow-y-auto">
        {options.map((o) => (
          <label
            key={o.value}
            className="hover:bg-accent flex cursor-pointer items-center gap-2 rounded-sm px-1.5 py-1 text-sm [&_svg]:size-3.5"
          >
            <input
              type="checkbox"
              className="accent-primary size-4 shrink-0 cursor-pointer"
              checked={value.includes(o.value)}
              onChange={(e) => onChange(e.target.checked ? [...value, o.value] : value.filter((v) => v !== o.value))}
            />
            <span className="flex min-w-0 flex-1 items-center gap-1.5 truncate">{o.label}</span>
          </label>
        ))}
      </div>
    </div>
  )
}

/** "Da" and "A" bounds; `type="date"` for days, otherwise free text checked by `parse`. */
export function RangeFilter({
  title,
  value,
  onChange,
  type = "text",
  parse,
  placeholders = ["Da", "A"],
}: {
  title?: string
  value: Range
  onChange: (v: Range) => void
  type?: "text" | "date"
  parse?: (s: string) => number | null
  placeholders?: [string, string]
}) {
  const invalid = (s: string) => !!parse && s.trim() !== "" && parse(s) === null
  // Date fields are too wide to sit side by side.
  if (type === "date")
    return (
      <div className="grid grid-cols-[auto_1fr] items-center gap-x-2 gap-y-1.5">
        {title && <div className="text-muted-foreground col-span-2 text-xs font-medium">{title}</div>}
        <span className="text-muted-foreground text-xs">Dal</span>
        <Input
          autoFocus
          type="date"
          value={value.min}
          max={value.max || undefined}
          onChange={(e) => onChange({ ...value, min: e.target.value })}
          aria-label="Dal giorno"
          className="h-8"
        />
        <span className="text-muted-foreground text-xs">Al</span>
        <Input
          type="date"
          value={value.max}
          min={value.min || undefined}
          onChange={(e) => onChange({ ...value, max: e.target.value })}
          aria-label="Al giorno"
          className="h-8"
        />
      </div>
    )
  return (
    <div className="grid gap-1">
      {title && <div className="text-muted-foreground text-xs font-medium">{title}</div>}
      <div className="flex items-center gap-1.5">
        <Input
          autoFocus
          inputMode="decimal"
          value={value.min}
          aria-invalid={invalid(value.min) || undefined}
          onChange={(e) => onChange({ ...value, min: e.target.value })}
          placeholder={placeholders[0]}
          aria-label={`${title ?? ""} da`.trim()}
          className="h-8"
        />
        <span className="text-muted-foreground">–</span>
        <Input
          inputMode="decimal"
          value={value.max}
          aria-invalid={invalid(value.max) || undefined}
          onChange={(e) => onChange({ ...value, max: e.target.value })}
          placeholder={placeholders[1]}
          aria-label={`${title ?? ""} a`.trim()}
          className="h-8"
        />
      </div>
    </div>
  )
}

/** Footer note: how many column filters are on, with a button to clear them all. */
export function ActiveFilters({ count, onClear }: { count: number; onClear: () => void }) {
  if (count === 0) return null
  return (
    <span className="flex items-center gap-1">
      <ListFilter className="text-primary size-3.5" />
      {count === 1 ? "1 filtro sulle colonne" : `${count} filtri sulle colonne`}
      <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={onClear}>
        <X /> Azzera
      </Button>
    </span>
  )
}
