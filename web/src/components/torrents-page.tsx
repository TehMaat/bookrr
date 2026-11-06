import { useMemo, useState } from "react"
import {
  Archive,
  ArchiveX,
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
import { NavbarStats, StatCard } from "@/components/navbar-stats"
import { NeverArchiveBar, PickBox, usePicker } from "@/components/never-archive"
import { canRemoveDuplicate, RemoveFromClientDialog } from "@/components/remove-from-client-dialog"
import {
  ArchivableBadge,
  archivableTone,
  DuplicateBadge,
  NeverArchiveBadge,
  PeerCounts,
  PersonalBadge,
  StatusBadge,
  statusLabels,
  WhereBadges,
} from "@/components/torrent-badges"
import { TorrentFormDialog } from "@/components/torrent-form-dialog"
import { TorrentSheet } from "@/components/torrent-sheet"
import { TableCard } from "@/components/table-card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import { formatBytes, formatState, formatUnix, isDownloading, plural } from "@/lib/format"
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

/** The values a torrent has for the "Location" filter. */
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
            {formatState(l.state)} · added on {formatUnix(l.addedOn)}
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  )
}

/** "Complete everywhere" plus a menu to delete the torrent from one of the clients. */
function RemoveDuplicate({ t }: { t: Torrent }) {
  const [removing, setRemoving] = useState<Location | null>(null)
  return (
    // Events from the menu and the dialog (portals included) must not open the sheet.
    <div className="flex items-center gap-1.5" onClick={(e) => e.stopPropagation()}>
      <span className="text-muted-foreground text-xs whitespace-nowrap">Complete everywhere</span>
      {canRemoveDuplicate(t) && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" className="h-7">
              <Trash2 /> Remove from…
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuLabel>Remove the duplicate from</DropdownMenuLabel>
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
      const archivable = [t.archivable?.archivable ? "yes" : "no", ...(t.archivable?.neverArchive ? ["never"] : [])]
      if (!optionsMatch(cf.archivable, ...archivable)) return false
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
      if (c === 0) c = a.name.localeCompare(b.name, "en", { sensitivity: "base" })
      return c * dir
    })
    return out
  }, [scoped, filter, cf, query, sort])

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const totalSize = rows.reduce((s, t) => s + t.size, 0)
  const showDownload = filter === "duplicate"
  // Ticks to mark uploads as never to archive, among the archivable ones or those already marked.
  const picking = filter === "archivable" || cf.archivable.includes("never")
  const picker = usePicker(picking ? rows : [])
  const kept = scoped.filter((t) => t.archivable?.neverArchive).length
  const cols = (showDownload ? 7 : 6) + (picking ? 1 : 0)
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
    { key: "all", label: "Total", icon: Layers },
    { key: "client", label: "On a client", icon: Server, tone: "text-sky-600 dark:text-sky-400" },
    { key: "duplicate", label: "Duplicates", icon: Copy, tone: "text-destructive" },
    { key: "missing", label: "To locate", icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    { key: "archived", label: "Archived", icon: HardDrive, tone: "text-violet-600 dark:text-violet-400" },
    { key: "adopted", label: "Adopted", icon: HandHeart, tone: "text-emerald-600 dark:text-emerald-400" },
    { key: "unknown", label: "Unknown location", icon: CircleHelp },
    { key: "archivable", label: "Archivable", icon: Archive, tone: archivableTone },
  ]

  return (
    <div className="grid gap-4">
      <NavbarStats>
        {stats.map((s) => (
          <StatCard
            key={s.key}
            label={s.label}
            value={counts[s.key]}
            icon={s.icon}
            tone={s.tone}
            active={filter === s.key}
            loading={torrents.isLoading}
            onClick={() => {
              const next = filter === s.key && s.key !== "all" ? "all" : s.key
              // The "In download" column, and so its filter, exists only among the duplicates.
              if (next !== "duplicate" && isActive(cf.download)) columns.clear("download")
              picker.clear()
              reset(setFilter)(next)
            }}
          />
        ))}
      </NavbarStats>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => reset(setQuery)(e.target.value)}
            placeholder="Search by name, hash, disk, serial, adopter, path…"
            className="pl-8"
          />
        </div>
        <div className="flex items-center gap-2 px-1">
          <Switch id="only-personal" checked={onlyPersonal} onCheckedChange={reset(setOnlyPersonal)} />
          <Label htmlFor="only-personal" className="whitespace-nowrap">
            Only {info.data?.releaseTag ?? "Personal Release"}
          </Label>
        </div>
        <Button variant="outline" onClick={() => setImporting(true)}>
          <Upload /> Import
        </Button>
        <Button onClick={() => setAdding(true)}>
          <Plus /> Add
        </Button>
      </div>

      {picking && (
        <NeverArchiveBar
          chosen={picker.chosen}
          kept={kept}
          onShowKept={
            cf.archivable.includes("never")
              ? undefined
              : () => {
                  picker.clear()
                  setFilter("all")
                  columns.set("archivable", ["never"])
                }
          }
          onClear={picker.clear}
        />
      )}

      <TableCard>
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              {picking && (
                <TableHead className="w-0">
                  <PickBox label="Select all" checked={picker.all} indeterminate={picker.some} onChange={picker.toggleAll} />
                </TableHead>
              )}
              <TableHead className="w-full">
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("name")}>
                    Name <SortIcon k="name" />
                  </button>
                  <ColumnFilter label="Name" active={isActive(cf.name)} onClear={() => columns.clear("name")}>
                    <TextFilter
                      value={cf.name}
                      onChange={(v) => columns.set("name", v)}
                      placeholder="Name, hash, category, tags, notes…"
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead className="text-right">
                <div className="flex items-center justify-end gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("size")}>
                    Size <SortIcon k="size" />
                  </button>
                  <ColumnFilter label="Size" active={isActive(cf.size)} onClear={() => columns.clear("size")}>
                    <RangeFilter
                      value={cf.size}
                      onChange={(v) => columns.set("size", v)}
                      parse={parseSize}
                      placeholders={["e.g. 1 GB", "e.g. 50 GB"]}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              <TableHead>
                <div className="flex items-center gap-1">
                  Location
                  <ColumnFilter label="Location" active={isActive(cf.where)} onClear={() => columns.clear("where")}>
                    {(
                      [
                        ["Client", (clients.data ?? []).map((c) => ({ value: `c:${c.id}`, label: <><Server /> {c.name}</> }))],
                        ["Disks", (disks.data ?? []).map((d) => ({ value: `d:${d.id}`, label: <><HardDrive /> {d.label}</> }))],
                        ["Adopters", (adopters.data ?? []).map((a) => ({ value: `a:${a.id}`, label: <><HandHeart /> {a.name}</> }))],
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
                      options={[{ value: "none", label: "Nowhere" }]}
                      value={cf.where}
                      onChange={(v) => columns.set("where", v)}
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              {showDownload && (
                <TableHead>
                  <div className="flex items-center gap-1">
                    Downloading
                    <ColumnFilter label="Downloading" active={isActive(cf.download)} onClear={() => columns.clear("download")}>
                      <OptionsFilter
                        options={[
                          { value: "yes", label: <><Download /> Downloading on at least one client</> },
                          { value: "no", label: "Complete everywhere" },
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
                    Seeds <SortIcon k="seeders" />
                  </button>
                  <span className="text-muted-foreground">/</span>
                  <button className="flex items-center gap-1" onClick={() => toggleSort("completed")}>
                    Completed <SortIcon k="completed" />
                  </button>
                  <ColumnFilter
                    label="Seeds / Completed"
                    active={isActive(cf.seeders) || isActive(cf.completed)}
                    onClear={() => columns.clear("seeders", "completed")}
                  >
                    <RangeFilter title="Seeders" value={cf.seeders} onChange={(v) => columns.set("seeders", v)} parse={parseCount} />
                    <RangeFilter
                      title="Completed"
                      value={cf.completed}
                      onChange={(v) => columns.set("completed", v)}
                      parse={parseCount}
                    />
                  </ColumnFilter>
                </span>
              </TableHead>
              <TableHead className="hidden md:table-cell">
                <div className="flex items-center gap-1">
                  Archivable
                  <ColumnFilter label="Archivable" active={isActive(cf.archivable)} onClear={() => columns.clear("archivable")}>
                    <OptionsFilter
                      options={[
                        { value: "yes", label: <><Archive className={archivableTone} /> Archivable</> },
                        { value: "no", label: "Not archivable" },
                        { value: "never", label: <><ArchiveX /> Never archive</> },
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
                    Status <SortIcon k="status" />
                  </button>
                  <ColumnFilter label="Status" align="end" active={isActive(cf.status)} onClear={() => columns.clear("status")}>
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
                              <TriangleAlert /> Where did it go?
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
                  Loading failed: {torrents.error.message}
                </TableCell>
              </TableRow>
            )}
            {!torrents.isLoading && visible.length === 0 && !torrents.isError && (
              <TableRow>
                <TableCell colSpan={cols} className="text-muted-foreground py-10 text-center">
                  {all.length === 0
                    ? "No torrents. Add a qBittorrent client in the Clients page or enter a torrent by hand."
                    : "No torrents match the filters."}
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
                {picking && (
                  <TableCell>
                    {t.archivable && (
                      <PickBox
                        label={`Select ${t.name}`}
                        checked={picker.isPicked(t.hash)}
                        onChange={(on) => picker.toggle(t.hash, on)}
                      />
                    )}
                  </TableCell>
                )}
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
                  <NeverArchiveBadge a={t.archivable} />
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
                      <TriangleAlert className="text-amber-600" /> Where did it go?
                    </Button>
                  ) : (
                    <StatusBadge status={t.status} />
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div className="text-muted-foreground flex flex-wrap items-center gap-2 border-t px-4 py-2 text-xs sm:px-6">
          <span>
            {plural(rows.length, "torrent")} · {formatBytes(totalSize)}
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
      </TableCard>

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
