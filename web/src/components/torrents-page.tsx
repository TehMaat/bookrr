import { useMemo, useState } from "react"
import {
  Archive,
  ArrowDown,
  ArrowUp,
  ChevronLeft,
  ChevronRight,
  CircleHelp,
  Copy,
  Download,
  HandHeart,
  HardDrive,
  Layers,
  Plus,
  Search,
  Server,
  Trash2,
  TriangleAlert,
  Upload,
} from "lucide-react"

import {
  ActiveFilters,
  ColumnFilter,
  isActive,
  noRange,
  OptionsFilter,
  optionsMatch,
  parseCount,
  parseSize,
  RangeFilter,
  rangeMatch,
  TextFilter,
  textMatch,
  useColumnFilters,
} from "@/components/column-filter"
import { ImportDialog } from "@/components/import-dialog"
import { canRemoveDuplicate, RemoveFromClientDialog } from "@/components/remove-from-client-dialog"
import {
  ArchivableBadge,
  archivableTone,
  DuplicateBadge,
  PeerCounts,
  PersonalBadge,
  StatusBadge,
  statusLabels,
  WhereBadges,
} from "@/components/torrent-badges"
import { Badge } from "@/components/ui/badge"
import { TorrentFormDialog } from "@/components/torrent-form-dialog"
import { TorrentSheet } from "@/components/torrent-sheet"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Alert, Location, Torrent, TorrentStatus } from "@/lib/api"
import { formatBytes, formatState, formatUnix, isDownloading } from "@/lib/format"
import { useAdopters, useClients, useDisks, useInfo, useTorrents } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Filter = "all" | "client" | "duplicate" | "missing" | "archived" | "adopted" | "unknown" | "archivable"
type SortKey = "name" | "size" | "seeders" | "completed" | "status"

const PAGE_SIZE = 50

const statusOrder = { missing: 0, unknown: 1, client: 2, archived: 3, adopted: 4 }

/** One filter per column; "where" holds "c:<client>", "d:<disk>", "a:<adopter>" or "none". */
const noFilters = {
  name: "",
  size: noRange,
  where: [] as string[],
  download: [] as string[],
  seeders: noRange,
  completed: noRange,
  archivable: [] as string[],
  status: [] as string[],
}

/** The values a torrent has for the "Dove si trova" filter. */
function places(t: Torrent) {
  const out = [
    ...t.locations.map((l) => `c:${l.clientId}`),
    ...t.archives.map((a) => `d:${a.diskId}`),
    ...t.adoptions.map((a) => `a:${a.adopterId}`),
  ]
  return out.length > 0 ? out : ["none"]
}

function matches(t: Torrent, f: Filter) {
  switch (f) {
    case "all":
      return true
    case "duplicate":
      return t.duplicate
    case "adopted":
      return t.adoptions.length > 0
    case "archived":
      return t.archives.length > 0
    case "archivable":
      return !!t.archivable?.archivable
    default:
      return t.status === f
  }
}

/** Per-client download state, used in the duplicates view to pick which copy to keep. */
function DownloadCell({ t }: { t: Torrent }) {
  const downloading = t.locations.filter(isDownloading)
  if (downloading.length === 0) return <RemoveDuplicate t={t} />
  return (
    <div className="flex flex-wrap gap-1">
      {downloading.map((l) => (
        <Tooltip key={l.clientId}>
          <TooltipTrigger asChild>
            <Badge variant="warning">
              <Download /> {l.clientName} {Math.floor(l.progress * 100)}%
            </Badge>
          </TooltipTrigger>
          <TooltipContent>
            {formatState(l.state)} · aggiunto il {formatUnix(l.addedOn)}
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  )
}

/** "Completato ovunque" plus a menu to delete the torrent from one of the clients. */
function RemoveDuplicate({ t }: { t: Torrent }) {
  const [removing, setRemoving] = useState<Location | null>(null)
  return (
    // Events from the menu and the dialog (portals included) must not open the sheet.
    <div className="flex items-center gap-1.5" onClick={(e) => e.stopPropagation()}>
      <span className="text-muted-foreground text-xs whitespace-nowrap">Completato ovunque</span>
      {canRemoveDuplicate(t) && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="h-7">
              <Trash2 /> Rimuovi da…
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuLabel>Togli il duplicato da</DropdownMenuLabel>
            {t.locations.map((l) => (
              <DropdownMenuItem key={l.clientId} onSelect={() => setRemoving(l)}>
                <Server /> {l.clientName}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      <RemoveFromClientDialog torrent={t} location={removing} onClose={() => setRemoving(null)} />
    </div>
  )
}

export function TorrentsPage({ onResolve }: { onResolve: (a: Alert) => void }) {
  const torrents = useTorrents()
  const clients = useClients()
  const disks = useDisks()
  const adopters = useAdopters()
  const info = useInfo()
  const [filter, setFilter] = useState<Filter>("all")
  const [query, setQuery] = useState("")
  const [onlyPersonal, setOnlyPersonal] = useState(false)
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "name", desc: false })
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [importing, setImporting] = useState(false)
  const columns = useColumnFilters(noFilters, () => setPage(0))
  const cf = columns.filters

  const all = useMemo(() => torrents.data ?? [], [torrents.data])
  const scoped = useMemo(() => (onlyPersonal ? all.filter((t) => t.personalRelease) : all), [all, onlyPersonal])

  const counts = useMemo(() => {
    const c: Record<Filter, number> = {
      all: 0,
      client: 0,
      duplicate: 0,
      missing: 0,
      archived: 0,
      adopted: 0,
      unknown: 0,
      archivable: 0,
    }
    for (const t of scoped) for (const f of Object.keys(c) as Filter[]) if (matches(t, f)) c[f]++
    return c
  }, [scoped])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const out = scoped.filter((t) => {
      if (!matches(t, filter)) return false
      if (!textMatch(cf.name, t.name, t.hash, t.category, t.notes, ...t.tags)) return false
      if (!rangeMatch(t.size, cf.size, parseSize)) return false
      if (!optionsMatch(cf.where, ...places(t))) return false
      if (!optionsMatch(cf.download, t.locations.some(isDownloading) ? "yes" : "no")) return false
      // Torrents that are not uploads have no counters: a bound leaves them out.
      if (!rangeMatch(t.peers?.seeders ?? -1, cf.seeders, parseCount)) return false
      if (!rangeMatch(t.peers?.timesCompleted ?? -1, cf.completed, parseCount)) return false
      if (!optionsMatch(cf.archivable, t.archivable?.archivable ? "yes" : "no")) return false
      if (!optionsMatch(cf.status, t.status, ...(t.openAlert ? ["alert"] : []))) return false
      if (!q) return true
      return (
        t.name.toLowerCase().includes(q) ||
        t.hash.includes(q) ||
        t.adoptions.some((a) => a.adopterName.toLowerCase().includes(q)) ||
        t.notes.toLowerCase().includes(q) ||
        t.category.toLowerCase().includes(q) ||
        t.tags.some((tag) => tag.toLowerCase().includes(q)) ||
        t.archives.some(
          (a) =>
            a.diskLabel.toLowerCase().includes(q) ||
            a.diskSerial.toLowerCase().includes(q) ||
            a.path.toLowerCase().includes(q)
        ) ||
        t.locations.some((l) => l.savePath.toLowerCase().includes(q))
      )
    })
    const dir = sort.desc ? -1 : 1
    out.sort((a, b) => {
      let c = 0
      if (sort.key === "size") c = a.size - b.size
      else if (sort.key === "seeders") c = (a.peers?.seeders ?? -1) - (b.peers?.seeders ?? -1)
      else if (sort.key === "completed") c = (a.peers?.timesCompleted ?? -1) - (b.peers?.timesCompleted ?? -1)
      else if (sort.key === "status") c = statusOrder[a.status] - statusOrder[b.status]
      if (c === 0) c = a.name.localeCompare(b.name, "it", { sensitivity: "base" })
      return c * dir
    })
    return out
  }, [scoped, filter, cf, query, sort])

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const totalSize = rows.reduce((s, t) => s + t.size, 0)
  const showDownload = filter === "duplicate"
  const cols = showDownload ? 7 : 6
  const selectedTorrent = selected ? (all.find((t) => t.hash === selected) ?? null) : null

  const reset = <T,>(fn: (v: T) => void) => (v: T) => {
    fn(v)
    setPage(0)
  }

  const toggleSort = (key: SortKey) =>
    setSort((s) => (s.key === key ? { key, desc: !s.desc } : { key, desc: key !== "name" && key !== "status" }))

  const SortIcon = ({ k }: { k: SortKey }) =>
    sort.key === k ? sort.desc ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" /> : null

  const stats: { key: Filter; label: string; icon: React.ElementType; tone?: string }[] = [
    { key: "all", label: "Totale", icon: Layers },
    { key: "client", label: "Su client", icon: Server, tone: "text-sky-600 dark:text-sky-400" },
    { key: "duplicate", label: "Duplicati", icon: Copy, tone: "text-destructive" },
    { key: "missing", label: "Da localizzare", icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    { key: "archived", label: "Archiviati", icon: HardDrive, tone: "text-violet-600 dark:text-violet-400" },
    { key: "adopted", label: "Adottati", icon: HandHeart, tone: "text-emerald-600 dark:text-emerald-400" },
    { key: "unknown", label: "Posizione ignota", icon: CircleHelp },
    { key: "archivable", label: "Archiviabili", icon: Archive, tone: archivableTone },
  ]

  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-8">
        {stats.map((s) => (
          <button
            key={s.key}
            type="button"
            onClick={() => {
              const next = filter === s.key && s.key !== "all" ? "all" : s.key
              // The "In download" column, and so its filter, exists only among the duplicates.
              if (next !== "duplicate" && isActive(cf.download)) columns.clear("download")
              reset(setFilter)(next)
            }}
            className="text-left"
          >
            <Card
              className={cn(
                "hover:bg-accent/50 gap-1 px-4 py-3 transition-colors",
                filter === s.key && "ring-primary/40 border-primary/50 ring-2"
              )}
            >
              <div className="text-muted-foreground flex items-center gap-1.5 text-xs">
                <s.icon className={cn("size-3.5", s.tone)} />
                {s.label}
              </div>
              <div className={cn("text-2xl font-semibold tabular-nums", counts[s.key] > 0 && s.tone)}>
                {torrents.isLoading ? <Skeleton className="h-8 w-12" /> : counts[s.key]}
              </div>
            </Card>
          </button>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => reset(setQuery)(e.target.value)}
            placeholder="Cerca per nome, hash, disco, seriale, adottante, percorso…"
            className="pl-8"
          />
        </div>
        <div className="flex items-center gap-2 px-1">
          <Switch id="only-personal" checked={onlyPersonal} onCheckedChange={reset(setOnlyPersonal)} />
          <Label htmlFor="only-personal" className="whitespace-nowrap">
            Solo {info.data?.releaseTag ?? "Personal Release"}
          </Label>
        </div>
        <Button variant="outline" onClick={() => setImporting(true)}>
          <Upload /> Importa
        </Button>
        <Button onClick={() => setAdding(true)}>
          <Plus /> Aggiungi
        </Button>
      </div>

      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="w-full">
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("name")}>
                    Nome <SortIcon k="name" />
                  </button>
                  <ColumnFilter label="Nome" active={isActive(cf.name)} onClear={() => columns.clear("name")}>
                    <TextFilter
                      value={cf.name}
                      onChange={(v) => columns.set("name", v)}
                      placeholder="Nome, hash, categoria, tag, note…"
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="text-right">
                <div className="flex items-center justify-end gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("size")}>
                    Dimensione <SortIcon k="size" />
                  </button>
                  <ColumnFilter label="Dimensione" active={isActive(cf.size)} onClear={() => columns.clear("size")}>
                    <RangeFilter
                      value={cf.size}
                      onChange={(v) => columns.set("size", v)}
                      parse={parseSize}
                      placeholders={["es. 1 GB", "es. 50 GB"]}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead>
                <div className="flex items-center gap-1">
                  Dove si trova
                  <ColumnFilter label="Dove si trova" active={isActive(cf.where)} onClear={() => columns.clear("where")}>
                    {(
                      [
                        ["Client", (clients.data ?? []).map((c) => ({ value: `c:${c.id}`, label: <><Server /> {c.name}</> }))],
                        ["Dischi", (disks.data ?? []).map((d) => ({ value: `d:${d.id}`, label: <><HardDrive /> {d.label}</> }))],
                        ["Adottatori", (adopters.data ?? []).map((a) => ({ value: `a:${a.id}`, label: <><HandHeart /> {a.name}</> }))],
                      ] as const
                    )
                      .filter(([, options]) => options.length > 0)
                      .map(([title, options]) => (
                        <OptionsFilter
                          key={title}
                          title={title}
                          options={options}
                          value={cf.where}
                          onChange={(v) => columns.set("where", v)}
                        />
                      ))}
                    <OptionsFilter
                      options={[{ value: "none", label: "Da nessuna parte" }]}
                      value={cf.where}
                      onChange={(v) => columns.set("where", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              {showDownload && (
                <TableHead>
                  <div className="flex items-center gap-1">
                    In download
                    <ColumnFilter label="In download" active={isActive(cf.download)} onClear={() => columns.clear("download")}>
                      <OptionsFilter
                        options={[
                          { value: "yes", label: <><Download /> In download su almeno un client</> },
                          { value: "no", label: "Completato ovunque" },
                        ]}
                        value={cf.download}
                        onChange={(v) => columns.set("download", v)}
                      />
                    </ColumnFilter>
                  </div>
                </TableHead>
              )}
              <TableHead className="hidden sm:table-cell">
                <span className="flex items-center gap-2">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("seeders")}>
                    Seed <SortIcon k="seeders" />
                  </button>
                  <span className="text-muted-foreground">/</span>
                  <button className="flex items-center gap-1" onClick={() => toggleSort("completed")}>
                    Completati <SortIcon k="completed" />
                  </button>
                  <ColumnFilter
                    label="Seed / Completati"
                    active={isActive(cf.seeders) || isActive(cf.completed)}
                    onClear={() => columns.clear("seeders", "completed")}
                  >
                    <RangeFilter title="Seeder" value={cf.seeders} onChange={(v) => columns.set("seeders", v)} parse={parseCount} />
                    <RangeFilter
                      title="Completati"
                      value={cf.completed}
                      onChange={(v) => columns.set("completed", v)}
                      parse={parseCount}
                    />
                  </ColumnFilter>
                </span>
              </TableHead>
              <TableHead className="hidden md:table-cell">
                <div className="flex items-center gap-1">
                  Archiviabile
                  <ColumnFilter label="Archiviabile" active={isActive(cf.archivable)} onClear={() => columns.clear("archivable")}>
                    <OptionsFilter
                      options={[
                        { value: "yes", label: <><Archive className={archivableTone} /> Archiviabile</> },
                        { value: "no", label: "Non archiviabile" },
                      ]}
                      value={cf.archivable}
                      onChange={(v) => columns.set("archivable", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead>
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("status")}>
                    Stato <SortIcon k="status" />
                  </button>
                  <ColumnFilter label="Stato" align="end" active={isActive(cf.status)} onClear={() => columns.clear("status")}>
                    <OptionsFilter
                      options={[
                        ...(Object.keys(statusLabels) as TorrentStatus[]).map((st) => ({
                          value: st,
                          label: <StatusBadge status={st} />,
                        })),
                        {
                          value: "alert",
                          label: (
                            <Badge variant="warning">
                              <TriangleAlert /> Dove è finito?
                            </Badge>
                          ),
                        },
                      ]}
                      value={cf.status}
                      onChange={(v) => columns.set("status", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {torrents.isLoading &&
              Array.from({ length: 6 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell colSpan={cols}>
                    <Skeleton className="h-6 w-full" />
                  </TableCell>
                </TableRow>
              ))}
            {torrents.isError && (
              <TableRow>
                <TableCell colSpan={cols} className="text-destructive py-10 text-center">
                  Errore nel caricamento: {torrents.error.message}
                </TableCell>
              </TableRow>
            )}
            {!torrents.isLoading && visible.length === 0 && !torrents.isError && (
              <TableRow>
                <TableCell colSpan={cols} className="text-muted-foreground py-10 text-center">
                  {all.length === 0
                    ? "Nessun torrent. Aggiungi un client qBittorrent nella scheda Client oppure inserisci un torrent a mano."
                    : "Nessun torrent corrisponde ai filtri."}
                </TableCell>
              </TableRow>
            )}
            {visible.map((t) => (
              <TableRow
                key={t.hash}
                onClick={() => setSelected(t.hash)}
                className={cn(
                  "cursor-pointer",
                  t.openAlert && "bg-warning/10 hover:bg-warning/20 shadow-[inset_3px_0_0_var(--warning)]",
                  !t.openAlert && t.duplicate && "bg-destructive/5 hover:bg-destructive/10 shadow-[inset_3px_0_0_var(--destructive)]"
                )}
              >
                <TableCell className="max-w-0 min-w-[260px] whitespace-normal">
                  <div className="flex items-center gap-1.5">
                    <span className="truncate font-medium" title={t.name}>
                      {t.name}
                    </span>
                    {t.personalRelease && <PersonalBadge />}
                    {t.duplicate && <DuplicateBadge count={t.locations.length} />}
                  </div>
                  {(t.category || t.tags.length > 0 || t.notes) && (
                    <div className="text-muted-foreground truncate text-xs">
                      {[t.category, t.tags.join(", "), t.notes].filter(Boolean).join(" · ")}
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-right tabular-nums">{formatBytes(t.size)}</TableCell>
                <TableCell className="whitespace-normal">
                  <WhereBadges t={t} />
                </TableCell>
                {showDownload && (
                  <TableCell className="whitespace-normal">
                    <DownloadCell t={t} />
                  </TableCell>
                )}
                <TableCell className="hidden sm:table-cell">
                  {t.peers ? <PeerCounts p={t.peers} /> : <span className="text-muted-foreground text-xs">—</span>}
                </TableCell>
                <TableCell className="hidden md:table-cell">
                  <ArchivableBadge a={t.archivable} />
                </TableCell>
                <TableCell>
                  {t.openAlert ? (
                    <Button
                      size="sm"
                      variant="outline"
                      className="border-warning/60 h-7"
                      onClick={(e) => {
                        e.stopPropagation()
                        onResolve(t.openAlert!)
                      }}
                    >
                      <TriangleAlert className="text-amber-600" /> Dove è finito?
                    </Button>
                  ) : (
                    <StatusBadge status={t.status} />
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div className="text-muted-foreground flex flex-wrap items-center gap-2 border-t px-4 py-2 text-xs">
          <span>
            {rows.length} torrent · {formatBytes(totalSize)}
          </span>
          <ActiveFilters count={columns.active} onClear={() => columns.clear()} />
          {pages > 1 && (
            <div className="ml-auto flex items-center gap-1">
              <Button variant="ghost" size="icon-sm" disabled={current === 0} onClick={() => setPage(current - 1)}>
                <ChevronLeft />
              </Button>
              <span className="tabular-nums">
                {current + 1} / {pages}
              </span>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={current >= pages - 1}
                onClick={() => setPage(current + 1)}
              >
                <ChevronRight />
              </Button>
            </div>
          )}
        </div>
      </Card>

      <TorrentSheet
        torrent={selectedTorrent}
        onClose={() => setSelected(null)}
        onResolve={onResolve}
        onHashChange={setSelected}
      />
      <TorrentFormDialog open={adding} onOpenChange={setAdding} />
      <ImportDialog open={importing} onOpenChange={setImporting} />
    </div>
  )
}
