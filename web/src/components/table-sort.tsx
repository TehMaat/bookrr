import { useState } from "react"
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp, Plus, RotateCcw, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { cn } from "@/lib/utils"

/** The sort of a table: its criteria by priority, each column at most once. */
export type SortBy<K extends string> = { key: K; desc: boolean }[]

/**
 * The sort of a table on several columns. A click on a column title sorts by that column alone
 * (again: the other way round); with Shift it adds the column as the next criterion, then reverses it,
 * then takes it out. firstDesc says which columns sort from the highest at the first click.
 */
export function useMultiSort<K extends string>(initial: SortBy<K>, firstDesc: (key: K) => boolean, onChange?: () => void) {
  const [sorts, setSorts] = useState<SortBy<K>>(initial)
  const update = (fn: (s: SortBy<K>) => SortBy<K>) => {
    setSorts(fn)
    onChange?.()
  }
  const click = (key: K, add: boolean) =>
    update((s) => {
      const i = s.findIndex((x) => x.key === key)
      if (!add) return s.length === 1 && i === 0 ? [{ key, desc: !s[0].desc }] : [{ key, desc: firstDesc(key) }]
      if (i < 0) return [...s, { key, desc: firstDesc(key) }]
      // Shift on the only criterion reverses it: taking it out would leave no sort.
      if (s[i].desc === firstDesc(key) || s.length === 1) return s.map((x, j) => (j === i ? { key, desc: !x.desc } : x))
      return s.filter((_, j) => j !== i)
    })
  return {
    sorts,
    click,
    set: (s: SortBy<K>) => update(() => s),
    add: (key: K) => update((s) => (s.some((x) => x.key === key) ? s : [...s, { key, desc: firstDesc(key) }])),
    reverse: (key: K) => update((s) => s.map((x) => (x.key === key ? { key, desc: !x.desc } : x))),
    remove: (...keys: K[]) =>
      // Only when there is something to take out, so that hiding a column doesn't move the page.
      sorts.some((x) => keys.includes(x.key)) && update((s) => s.filter((x) => !keys.includes(x.key))),
    move: (key: K, by: -1 | 1) =>
      update((s) => {
        const i = s.findIndex((x) => x.key === key)
        const j = i + by
        if (i < 0 || j < 0 || j >= s.length) return s
        const next = [...s]
        ;[next[i], next[j]] = [next[j], next[i]]
        return next
      }),
  }
}

export type MultiSort<K extends string> = ReturnType<typeof useMultiSort<K>>

/**
 * Compares two rows by the criteria in turn; when they are all equal, by tiebreak, in the direction of the first
 * criterion. Each comparator gives the order from the lowest to the highest.
 */
export function compareBy<T, K extends string>(
  sorts: SortBy<K>,
  comparators: Record<K, (a: T, b: T) => number>,
  tiebreak: (a: T, b: T) => number
) {
  return (a: T, b: T) => {
    for (const s of sorts) {
      const c = comparators[s.key](a, b)
      if (c) return s.desc ? -c : c
    }
    return tiebreak(a, b) * (sorts[0]?.desc ? -1 : 1)
  }
}

/** The arrow of a sorted column and, with several criteria, its place among them. */
function SortMark<K extends string>({ sort, k }: { sort: MultiSort<K>; k: K }) {
  const i = sort.sorts.findIndex((s) => s.key === k)
  if (i < 0) return null
  return (
    <span className="flex items-center">
      {sort.sorts[i].desc ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" />}
      {sort.sorts.length > 1 && <span className="text-[10px] leading-none tabular-nums">{i + 1}</span>}
    </span>
  )
}

/** A column title that sorts the table; with Shift (or Ctrl/Cmd) it adds the column to the sort. */
export function SortButton<K extends string>({
  sort,
  k,
  hint,
  className,
  children,
}: {
  sort: MultiSort<K>
  k: K
  /** What the column sorts by, when the title doesn't say it. */
  hint?: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      className={cn("flex items-center gap-1 select-none", className)}
      title={`${hint ? `${hint}: sort` : "Sort"}. Shift+click: sort by this too`}
      // Shift+click would select the text of the header.
      onMouseDown={(e) => e.shiftKey && e.preventDefault()}
      onClick={(e) => sort.click(k, e.shiftKey || e.ctrlKey || e.metaKey)}
    >
      {children}
      <SortMark sort={sort} k={k} />
    </button>
  )
}

/**
 * The button with the criteria of the sort: their order, direction, more of them. Shift+click on the titles
 * does the same, but there is no Shift on a phone, and the titles of some columns are hidden on small screens.
 */
export function SortPanel<K extends string>({
  sort,
  labels,
  available,
  onReset,
}: {
  sort: MultiSort<K>
  /** The names of the keys. */
  labels: Record<K, string>
  /** The keys it can add to the sort: those of the columns shown. */
  available: K[]
  onReset: () => void
}) {
  const label = (k: K) => labels[k]
  const more = available.filter((k) => !sort.sorts.some((s) => s.key === k))
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline">
          <ArrowUpDown /> Sort
          {sort.sorts.length > 1 && <span className="text-muted-foreground tabular-nums">{sort.sorts.length}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="grid w-80 max-w-[calc(100vw-2rem)] grid-cols-[minmax(0,1fr)] gap-3 p-3">
        <div className="flex h-6 items-center justify-between gap-2">
          <span className="text-sm font-medium">Sort by</span>
          <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" onClick={onReset}>
            <RotateCcw /> Reset
          </Button>
        </div>
        {sort.sorts.length === 0 ? (
          <p className="text-muted-foreground text-sm">By name.</p>
        ) : (
          <ol className="grid gap-1">
            {sort.sorts.map((s, i) => (
              <li key={s.key} className="flex items-center gap-1 text-sm">
                <span className="text-muted-foreground w-4 text-right tabular-nums">{i + 1}</span>
                <span className="min-w-0 flex-1 truncate">{label(s.key)}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  title="Reverse"
                  onClick={() => sort.reverse(s.key)}
                >
                  {s.desc ? <ArrowDown /> : <ArrowUp />} {s.desc ? "High first" : "Low first"}
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="size-7"
                  aria-label={`Move ${label(s.key)} up`}
                  title="Move up"
                  disabled={i === 0}
                  onClick={() => sort.move(s.key, -1)}
                >
                  <ChevronUp />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="size-7"
                  aria-label={`Move ${label(s.key)} down`}
                  title="Move down"
                  disabled={i === sort.sorts.length - 1}
                  onClick={() => sort.move(s.key, 1)}
                >
                  <ChevronDown />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="size-7"
                  aria-label={`Don't sort by ${label(s.key)}`}
                  title="Remove"
                  onClick={() => sort.remove(s.key)}
                >
                  <X />
                </Button>
              </li>
            ))}
          </ol>
        )}
        {more.length > 0 && (
          <div className="grid gap-1.5">
            <span className="text-muted-foreground text-xs">{sort.sorts.length ? "Then by" : "Sort by"}</span>
            <div className="flex flex-wrap gap-1">
              {more.map((k) => (
                <Button key={k} variant="outline" size="sm" className="h-7 px-2 text-xs" onClick={() => sort.add(k)}>
                  <Plus /> {label(k)}
                </Button>
              ))}
            </div>
          </div>
        )}
        <p className="text-muted-foreground text-xs">Tip: Shift+click a column title to add it to the sort.</p>
      </PopoverContent>
    </Popover>
  )
}
