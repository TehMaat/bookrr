import { useEffect, useMemo, useRef, useState } from "react"
import { ChevronLeft, ChevronRight, Download, FileWarning, Pause, Play, Search } from "lucide-react"

import {
  ActiveFilters,
  ColumnFilter,
  isActive,
  OptionsFilter,
  TextFilter,
  textMatch,
  optionsMatch,
  useColumnFilters,
} from "@/components/column-filter"
import { TableCard } from "@/components/table-card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, type LogEntry, type LogFile, type LogLevel, type LogResult } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { useLogFiles } from "@/lib/queries"
import { cn } from "@/lib/utils"

const PAGE_SIZE = 100
/** Lines kept while following today's file: the oldest drop out. */
const MAX_ENTRIES = 5000
const POLL_MS = 5_000

const levels: LogLevel[] = ["DEBUG", "INFO", "WARN", "ERROR"]
const levelLabels: Record<LogLevel, string> = { DEBUG: "Debug", INFO: "Info", WARN: "Warning", ERROR: "Error" }
const ALL = "all"

/** The base level of a level as slog writes it ("WARN+2" → "WARN"). */
function baseLevel(l: string): LogLevel {
  const b = l.toUpperCase().replace(/[+-]\d+$/, "")
  return (levels as string[]).includes(b) ? (b as LogLevel) : "INFO"
}

function LevelBadge({ level }: { level: string }) {
  const b = baseLevel(level)
  const variant = b === "ERROR" ? "destructive" : b === "WARN" ? "warning" : b === "INFO" ? "info" : "outline"
  return (
    <Badge variant={variant} className={cn(b === "DEBUG" && "text-muted-foreground")}>
      {level || "—"}
    </Badge>
  )
}

/** The attributes as "key=value", groups as "group.key". */
function flatten(attrs: Record<string, unknown>, prefix = ""): [string, string][] {
  const out: [string, string][] = []
  for (const [k, v] of Object.entries(attrs)) {
    const key = prefix + k
    if (v && typeof v === "object" && !Array.isArray(v)) out.push(...flatten(v as Record<string, unknown>, key + "."))
    else out.push([key, typeof v === "string" ? v : JSON.stringify(v)])
  }
  return out
}

const timeFmt = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit" })
const dayFmt = new Intl.DateTimeFormat("en-GB", { weekday: "short", day: "numeric", month: "short", year: "numeric" })

function formatTime(s: string) {
  const d = new Date(s)
  return isNaN(d.getTime()) ? s : timeFmt.format(d)
}

function formatDay(day: string, today: string) {
  if (day === today) return "Today"
  const d = new Date(`${day}T12:00:00`)
  return isNaN(d.getTime()) ? day : dayFmt.format(d)
}

/** Waits for the typing to stop before searching on the server. */
function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return v
}

const noFilters = { level: [] as string[], msg: "", attrs: "" }

export function LogsPage() {
  const files = useLogFiles()
  const today = files.data?.today ?? ""
  const [chosenDay, setDay] = useState("")
  const day = chosenDay || today
  const [minLevel, setMinLevel] = useState<string>(ALL)
  const [query, setQuery] = useState("")
  const q = useDebounced(query, 400)
  const [live, setLive] = useState(true)
  const [page, setPage] = useState(0)
  const columns = useColumnFilters(noFilters, () => setPage(0))
  const cf = columns.filters

  const [data, setData] = useState<LogResult | null>(null)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  // Bumped on every new read: the answers of older ones are dropped.
  const generation = useRef(0)

  const level = minLevel === ALL ? undefined : minLevel
  const enabled = files.data?.enabled ?? false

  useEffect(() => {
    if (!day || !enabled) return
    const gen = ++generation.current
    setLoading(true)
    setError("")
    setPage(0)
    api
      .logs({ day, level, q })
      .then((r) => gen === generation.current && setData(r))
      .catch((e: Error) => gen === generation.current && (setError(e.message), setData(null)))
      .finally(() => gen === generation.current && setLoading(false))
  }, [day, level, q, enabled])

  // Today's file grows: read only the lines added since the last read.
  const following = live && day === today && enabled && !!data
  const next = data?.next ?? 0
  useEffect(() => {
    if (!following) return
    const gen = generation.current
    const t = setInterval(() => {
      api
        .logs({ day, level, q, after: next })
        .then((r) => {
          if (gen !== generation.current || (r.entries.length === 0 && r.next === next)) return
          setData((d) => {
            if (!d) return r
            const all = [...d.entries, ...r.entries]
            const cut = Math.max(0, all.length - MAX_ENTRIES)
            return { entries: all.slice(cut), next: r.next, truncated: d.truncated || cut > 0 }
          })
        })
        .catch(() => {
          /* the next poll tries again */
        })
    }, POLL_MS)
    return () => clearInterval(t)
  }, [following, day, level, q, next])

  const rows = useMemo(() => {
    const out = (data?.entries ?? []).filter((e) => {
      const attrs = flatten(e.attrs)
        .map(([k, v]) => `${k}=${v}`)
        .join(" ")
      return optionsMatch(cf.level, baseLevel(e.level)) && textMatch(cf.msg, e.msg) && textMatch(cf.attrs, attrs)
    })
    // Newest first.
    return out.reverse()
  }, [data, cf])

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)

  const days = useMemo(() => {
    const ds = [...new Set((files.data?.files ?? []).map((f) => f.day))]
    if (today && !ds.includes(today)) ds.unshift(today)
    return ds.sort().reverse()
  }, [files.data, today])
  const dayFiles = (files.data?.files ?? []).filter((f) => f.day === day)

  if (files.isLoading) return <Skeleton className="h-48 w-full" />
  if (files.isError)
    return (
      <Card>
        <CardContent className="text-destructive text-sm">Error: {files.error.message}</CardContent>
      </Card>
    )
  if (!enabled)
    return (
      <Card>
        <CardContent className="flex items-start gap-3 text-sm">
          <FileWarning className="text-amber-600 mt-0.5 size-5 shrink-0" />
          <div>
            File logs are not active: bookrr could not create the <code>logs/</code> folder in the data folder. Messages
            only go to the container's stderr (<code>docker logs</code>).
          </div>
        </CardContent>
      </Card>
    )

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Select value={day} onValueChange={(d) => setDay(d === today ? "" : d)}>
          <SelectTrigger className="w-[190px]" aria-label="Day">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {days.map((d) => (
              <SelectItem key={d} value={d}>
                {formatDay(d, today)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={minLevel} onValueChange={setMinLevel}>
          <SelectTrigger className="w-[160px]" aria-label="Minimum level">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>All levels</SelectItem>
            {levels.slice(1).map((l) => (
              <SelectItem key={l} value={l}>
                {levelLabels[l]} or above
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="relative min-w-[220px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search messages and attributes (client, hash, error…)"
            className="pl-8"
          />
        </div>
        {day === today && (
          <Button variant="outline" onClick={() => setLive(!live)} aria-pressed={live}>
            {live ? <Pause /> : <Play />}
            {live ? "Pause" : "Live"}
          </Button>
        )}
        <DownloadButton files={dayFiles} />
      </div>

      {error && (
        <Card>
          <CardContent className="text-destructive text-sm">Error: {error}</CardContent>
        </Card>
      )}

      <TableCard>
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="w-24">Time</TableHead>
              <TableHead className="w-24">
                <div className="flex items-center gap-1">
                  Level
                  <ColumnFilter label="Level" active={isActive(cf.level)} onClear={() => columns.clear("level")}>
                    <OptionsFilter
                      options={levels.map((l) => ({ value: l, label: levelLabels[l] }))}
                      value={cf.level}
                      onChange={(v) => columns.set("level", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead>
                <div className="flex items-center gap-1">
                  Message
                  <ColumnFilter label="Message" active={isActive(cf.msg)} onClear={() => columns.clear("msg")}>
                    <TextFilter value={cf.msg} onChange={(v) => columns.set("msg", v)} />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="w-full">
                <div className="flex items-center gap-1">
                  Attributes
                  <ColumnFilter label="Attributes" align="end" active={isActive(cf.attrs)} onClear={() => columns.clear("attrs")}>
                    <TextFilter value={cf.attrs} onChange={(v) => columns.set("attrs", v)} placeholder="E.g. client=nas…" />
                  </ColumnFilter>
                </div>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && !data && (
              <TableRow>
                <TableCell colSpan={4}>
                  <Skeleton className="h-6 w-full" />
                </TableCell>
              </TableRow>
            )}
            {data && rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="text-muted-foreground py-10 text-center">
                  {data.entries.length === 0
                    ? q || level
                      ? "No messages match the search."
                      : "No messages on this day."
                    : "No messages match the filters."}
                </TableCell>
              </TableRow>
            )}
            {visible.map((e) => (
              <LogRow key={e.id} e={e} />
            ))}
          </TableBody>
        </Table>
        <div className="text-muted-foreground flex flex-wrap items-center gap-2 border-t px-4 py-2 text-xs sm:px-6">
          <span>
            {rows.length === 1 ? "1 message" : `${rows.length} messages`}
            {data?.truncated && ` (only the last ${MAX_ENTRIES}: narrow the search or download the file)`}
          </span>
          {following && <span className="text-emerald-600 dark:text-emerald-400">· live</span>}
          <ActiveFilters count={columns.active} onClear={() => columns.clear()} />
          {pages > 1 && (
            <div className="ml-auto flex items-center gap-1">
              <Button variant="ghost" size="icon-sm" disabled={current === 0} onClick={() => setPage(current - 1)}>
                <ChevronLeft />
              </Button>
              <span className="tabular-nums">
                {current + 1} / {pages}
              </span>
              <Button variant="ghost" size="icon-sm" disabled={current >= pages - 1} onClick={() => setPage(current + 1)}>
                <ChevronRight />
              </Button>
            </div>
          )}
        </div>
      </TableCard>
      {files.data?.dir && (
        <p className="text-muted-foreground text-xs">
          Files in <code>{files.data.dir}</code>, one per day in JSON Lines format.
        </p>
      )}
    </div>
  )
}

function LogRow({ e }: { e: LogEntry }) {
  const b = baseLevel(e.level)
  return (
    <TableRow
      className={cn(
        "align-top",
        b === "ERROR" && "bg-destructive/5 hover:bg-destructive/10",
        b === "WARN" && "bg-warning/10 hover:bg-warning/20"
      )}
    >
      <TableCell className="text-muted-foreground font-mono text-xs tabular-nums" title={e.time}>
        {formatTime(e.time)}
      </TableCell>
      <TableCell>
        <LevelBadge level={e.level} />
      </TableCell>
      <TableCell className="min-w-[200px] font-medium whitespace-normal">{e.msg}</TableCell>
      <TableCell className="max-w-0 min-w-[240px] whitespace-normal">
        <div className="flex flex-wrap gap-x-3 gap-y-0.5 font-mono text-xs">
          {flatten(e.attrs).map(([k, v]) => (
            <span key={k} className="break-all">
              <span className="text-muted-foreground">{k}=</span>
              {v}
            </span>
          ))}
        </div>
      </TableCell>
    </TableRow>
  )
}

/** Downloads the file of the day as it is; a menu when the day has more than one. */
function DownloadButton({ files }: { files: LogFile[] }) {
  if (files.length === 0)
    return (
      <Button variant="outline" disabled>
        <Download /> Download
      </Button>
    )
  if (files.length === 1)
    return (
      <Button variant="outline" asChild>
        <a href={api.logFileUrl(files[0].name)} download title={`${files[0].name} · ${formatBytes(files[0].size)}`}>
          <Download /> Download
        </a>
      </Button>
    )
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <Download /> Download
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Files of the day</DropdownMenuLabel>
        {files.map((f) => (
          <DropdownMenuItem key={f.name} asChild>
            <a href={api.logFileUrl(f.name)} download>
              <span className="font-mono text-xs">{f.name}</span>
              <span className="text-muted-foreground ml-auto text-xs">{formatBytes(f.size)}</span>
            </a>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
