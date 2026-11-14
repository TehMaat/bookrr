import { useMemo, useState } from "react"
import {
  Archive,
  ArchiveX,
  ArrowDown,
  ArrowUp,
  ChevronLeft,
  ChevronRight,
  CircleArrowUp,
  CheckCheck,
  CloudUpload,
  Copy,
  Download,
  ExternalLink,
  HandHeart,
  HardDrive,
  Layers,
  Plus,
  Search,
  Server,
  Skull,
  Trash2,
  TriangleAlert,
  Upload,
} from "lucide-react"
import { toast } from "sonner"

import {
  ActiveFilters,
  ColumnFilter,
  dateMatch,
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
import { ColumnPicker, useHiddenColumns } from "@/components/column-picker"
import { ImportDialog } from "@/components/import-dialog"
import { NavbarStats, StatCard } from "@/components/navbar-stats"
import { NeverArchiveBar, PickBox, usePicker } from "@/components/never-archive"
import { BestBadge, bestKind, ReleaseStatusBadge, RemovedBadge } from "@/components/release-badges"
import { canRemoveDuplicate, RemoveFromClientDialog } from "@/components/remove-from-client-dialog"
import {
  ArchivableBadge,
  archivableDays,
  archivableTone,
  DuplicateBadge,
  NeverArchiveBadge,
  PeerCounts,
  PersonalBadge,
  WaitingArchivableBadge,
  WhereBadges,
} from "@/components/torrent-badges"
import { TorrentFormDialog } from "@/components/torrent-form-dialog"
import { TorrentSheet } from "@/components/torrent-sheet"
import { TrackerPanel } from "@/components/tracker-panel"
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
import { api, type Alert, type Location, type ReleaseStatus, type Torrent, type TrackerRelease } from "@/lib/api"
import { formatBytes, formatDate, formatDay, formatRelative, formatState, formatUnix, isDownloading, plural } from "@/lib/format"
import { useAction, useAdopters, useClients, useDisks, useInfo, useReleases, useTorrents } from "@/lib/queries"
import { cn } from "@/lib/utils"

/** The counters on top, each a filter: besides "all", the torrents with something to do. "dead" needs the tracker. */
const torrentFilters = ["all", "duplicate", "missing", "dead", "archivable", "better"] as const
type Filter = (typeof torrentFilters)[number]
type SortKey = "name" | "size" | "date" | "seeders" | "completed" | "archivable" | "status"
/** The columns that can be hidden. */
type Col = "size" | "location" | "type" | "best" | "uploaded" | "peers" | "archivable"

const PAGE_SIZE = 50

const statusOrder: Record<ReleaseStatus, number> = { missing: 0, untracked: 1, unknown: 2, client: 3, archived: 4, adopted: 5 }

/** One filter per column; "where" holds "c:<client>", "d:<disk>", "a:<adopter>" or "none". */
const noFilters = {
  name: "",
  size: noRange,
  where: [] as string[],
  download: [] as string[],
  type: [] as string[],
  resolution: [] as string[],
  best: [] as string[],
  date: noRange,
  seeders: noRange,
  completed: noRange,
  archivable: [] as string[],
  archivableDays: noRange,
  status: [] as string[],
  upload: [] as string[],
}

/**
 * The counters of the earlier versions, now column filters: their addresses (#torrents/adopted…) open the same list.
 * "c:*", "d:*" and "a:*" of the Location filter are any client, disk or adopter.
 */
const oldFilters: Record<string, Partial<typeof noFilters>> = {
  client: { where: ["c:*"] },
  archived: { where: ["d:*"] },
  adopted: { where: ["a:*"] },
  unknown: { status: ["unknown"] },
  uploads: { upload: ["yes"] },
  untracked: { status: ["untracked"] },
  removed: { upload: ["removed"] },
}

/** What can follow "#torrents/" in the address: a counter or a filter of the earlier counters. */
export const torrentViews: readonly string[] = [...torrentFilters, ...Object.keys(oldFilters)]

/** The filters of each column that can be hidden: hiding it clears them. */
const colFilters: Record<Col, (keyof typeof noFilters)[]> = {
  size: ["size"],
  location: ["where"],
  type: ["type", "resolution"],
  best: ["best"],
  uploaded: ["date"],
  peers: ["seeders", "completed"],
  archivable: ["archivable", "archivableDays"],
}

/**
 * A torrent of bookrr with its upload on the tracker (same hash), if any, or an upload bookrr has no torrent for.
 * key is the hash of the torrent, or "r:<id>" of the upload.
 */
type Row = { key: string; t: Torrent | null; r: TrackerRelease | null }

const statusOf = (x: Row): ReleaseStatus => x.t?.status ?? "untracked"
/** The counters of the upload: those of the torrent, or the last known of an upload no longer on the tracker. */
const peersOf = (x: Row) => x.t?.peers ?? x.r
const archivableOf = (x: Row) => (x.t ? x.t.archivable : (x.r?.archivable ?? null))
const nameOf = (x: Row) => x.t?.name ?? x.r?.name ?? ""
const sizeOf = (x: Row) => x.t?.size || x.r?.size || 0

/** The values a row has for the "Location" filter: each place, and "c:*", "d:*", "a:*" when on any of that kind. */
function places(t: Torrent | null) {
  if (!t) return ["none"]
  const any = (prefix: string, ids: (number | null)[]) => (ids.length > 0 ? [`${prefix}:*`, ...ids.map((id) => `${prefix}:${id}`)] : [])
  const out = [
    ...any("c", t.locations.map((l) => l.clientId)),
    ...any("d", t.archives.map((a) => a.diskId)),
    ...any("a", t.adoptions.map((a) => a.adopterId)),
  ]
  return out.length > 0 ? out : ["none"]
}

/** The values a row has for the "Upload" filter of the Status column. */
function uploadFlags(r: TrackerRelease | null) {
  if (!r) return ["no"]
  return r.removedAt ? ["yes", "removed"] : ["yes"]
}

function matches(x: Row, f: Filter) {
  const { t, r } = x
  switch (f) {
    case "all":
      return true
    case "duplicate":
      return !!t?.duplicate
    case "missing":
      return t?.status === "missing"
    case "dead":
      return !!r && !r.removedAt && r.seeders === 0
    case "archivable":
      return !!archivableOf(x)?.archivable
    case "better":
      return !!r?.best?.better
  }
}

/** The distinct non-empty values, sorted. */
function distinct(values: string[]) {
  return [...new Set(values.filter(Boolean))].sort((a, b) => a.localeCompare(b, "en", { numeric: true }))
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

const Dash = () => <span className="text-muted-foreground text-xs">—</span>

/**
 * The torrents of bookrr together with the uploads on the UNIT3D tracker: each upload is on the row of the torrent with
 * the same hash, and the uploads bookrr has no torrent for have a row of their own. view is what follows "#torrents/" in
 * the address (one of torrentViews); onFilter tells the counter chosen later (to keep it in the address).
 */
export function TorrentsPage({
  onResolve,
  view = "all",
  onFilter,
}: {
  onResolve: (a: Alert) => void
  view?: string
  onFilter?: (f: Filter) => void
}) {
  const initialFilter: Filter = (torrentFilters as readonly string[]).includes(view) ? (view as Filter) : "all"
  const torrents = useTorrents()
  const releases = useReleases()
  const clients = useClients()
  const disks = useDisks()
  const adopters = useAdopters()
  const info = useInfo()
  const [filter, setFilterState] = useState<Filter>(initialFilter)
  const [query, setQuery] = useState("")
  const [onlyPersonal, setOnlyPersonal] = useState(false)
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>(
    view === "uploads" || view === "dead"
      ? { key: "date", desc: true }
      : view === "archivable"
        ? { key: "archivable", desc: true }
        : { key: "name", desc: false }
  )
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [importing, setImporting] = useState(false)
  const columns = useColumnFilters(noFilters, () => setPage(0), { ...noFilters, ...oldFilters[view] })
  const cf = columns.filters
  const hidden = useHiddenColumns<Col>("bookrr.torrents.hidden-columns")
  const add = useAction((ids: number[]) => api.addReleases(ids))

  const setFilter = (f: Filter) => {
    setFilterState(f)
    // The archivable ones held the longest come first.
    if (f === "archivable") setSort({ key: "archivable", desc: true })
    onFilter?.(f)
  }

  const tracker = releases.data
  const best = tracker?.best ?? null
  const allTorrents = useMemo(() => torrents.data ?? [], [torrents.data])
  const allReleases = useMemo(() => tracker?.releases ?? [], [tracker])
  // With no tracker, and no uploads read before, the columns and the counters of the uploads would stay empty.
  const hasTracker = !!tracker && (tracker.enabled || allReleases.length > 0)

  const all = useMemo<Row[]>(() => {
    const linked = new Map<string, TrackerRelease>()
    const loose: Row[] = []
    for (const r of allReleases) {
      if (!r.torrentHash) {
        loose.push({ key: `r:${r.id}`, t: null, r })
        continue
      }
      // An upload still on the tracker wins over a removed one with the same hash.
      const prev = linked.get(r.torrentHash)
      if (!prev || (prev.removedAt && !r.removedAt)) linked.set(r.torrentHash, r)
    }
    return [...allTorrents.map((t) => ({ key: t.hash, t, r: linked.get(t.hash) ?? null })), ...loose]
  }, [allTorrents, allReleases])
  const scoped = useMemo(
    () => (onlyPersonal ? all.filter((x) => x.t?.personalRelease ?? x.r?.personalRelease) : all),
    [all, onlyPersonal]
  )
  const types = useMemo(() => distinct(allReleases.map((r) => r.type)), [allReleases])
  const resolutions = useMemo(() => distinct(allReleases.map((r) => r.resolution)), [allReleases])

  const counts = useMemo(() => {
    const c = Object.fromEntries(torrentFilters.map((f) => [f, 0])) as Record<Filter, number>
    for (const x of scoped) for (const f of torrentFilters) if (matches(x, f)) c[f]++
    return c
  }, [scoped])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const out = scoped.filter((x) => {
      const { t, r } = x
      if (!matches(x, filter)) return false
      const texts = [t?.name, t?.hash, t?.category, t?.notes, ...(t?.tags ?? []), r?.name, r?.category, r?.hash, r && String(r.id)]
      if (!textMatch(cf.name, ...texts.filter((s): s is string => !!s))) return false
      if (!rangeMatch(sizeOf(x), cf.size, parseSize)) return false
      if (!optionsMatch(cf.where, ...places(t))) return false
      if (!optionsMatch(cf.download, t?.locations.some(isDownloading) ? "yes" : "no")) return false
      if (!optionsMatch(cf.type, r?.type ?? "")) return false
      if (!optionsMatch(cf.resolution, r?.resolution ?? "")) return false
      const kind = bestKind(r?.best)
      if (!optionsMatch(cf.best, ...(kind ? [kind] : []))) return false
      if (!dateMatch(r?.createdAt, cf.date)) return false
      // Torrents that are not uploads have no counters: a bound leaves them out.
      const peers = peersOf(x)
      if (!rangeMatch(peers?.seeders ?? -1, cf.seeders, parseCount)) return false
      if (!rangeMatch(peers?.timesCompleted ?? -1, cf.completed, parseCount)) return false
      const a = archivableOf(x)
      if (!optionsMatch(cf.archivable, a?.archivable ? "yes" : "no", ...(a?.neverArchive ? ["never"] : []))) return false
      // Only the days shown in the column: a bound leaves out those that don't meet the thresholds or are never to archive.
      if (!rangeMatch(archivableDays(a), cf.archivableDays, parseCount)) return false
      const flags = [statusOf(x), ...(t?.openAlert ? ["alert"] : [])]
      if (!optionsMatch(cf.status, ...flags)) return false
      if (!optionsMatch(cf.upload, ...uploadFlags(r))) return false
      if (!q) return true
      return (
        texts.some((s) => !!s && s.toLowerCase().includes(q)) ||
        !!r?.type.toLowerCase().includes(q) ||
        !!r?.resolution.toLowerCase().includes(q) ||
        !!t?.adoptions.some((a) => a.adopterName.toLowerCase().includes(q)) ||
        !!t?.archives.some(
          (a) =>
            a.diskLabel.toLowerCase().includes(q) ||
            a.diskSerial.toLowerCase().includes(q) ||
            a.path.toLowerCase().includes(q)
        ) ||
        !!t?.locations.some((l) => l.savePath.toLowerCase().includes(q))
      )
    })
    const dir = sort.desc ? -1 : 1
    out.sort((a, b) => {
      let c = 0
      if (sort.key === "size") c = sizeOf(a) - sizeOf(b)
      else if (sort.key === "date") c = (a.r?.createdAt ?? "").localeCompare(b.r?.createdAt ?? "") || (a.r?.id ?? 0) - (b.r?.id ?? 0)
      else if (sort.key === "seeders") c = (peersOf(a)?.seeders ?? -1) - (peersOf(b)?.seeders ?? -1)
      else if (sort.key === "completed") c = (peersOf(a)?.timesCompleted ?? -1) - (peersOf(b)?.timesCompleted ?? -1)
      else if (sort.key === "archivable") {
        const aa = archivableOf(a)
        const ab = archivableOf(b)
        // Same days: the one that meets the thresholds since earlier counts as more.
        c = archivableDays(aa) - archivableDays(ab) || (ab?.since ?? "").localeCompare(aa?.since ?? "")
      } else if (sort.key === "status") c = statusOrder[statusOf(a)] - statusOrder[statusOf(b)]
      if (c === 0) c = nameOf(a).localeCompare(nameOf(b), "en", { sensitivity: "base" })
      return c * dir
    })
    return out
  }, [scoped, filter, cf, query, sort])

  const optional: { id: Col; label: string }[] = [
    { id: "size", label: "Size" },
    { id: "location", label: "Location" },
    ...(hasTracker ? [{ id: "type" as const, label: "Type" }] : []),
    ...(best ? [{ id: "best" as const, label: "Max" }] : []),
    ...(hasTracker ? [{ id: "uploaded" as const, label: "Uploaded" }] : []),
    { id: "peers", label: "Seeds / Completed" },
    { id: "archivable", label: "Archivable" },
  ]
  // A column with a filter set shows also when hidden, e.g. Location opened from #torrents/adopted.
  const show = (c: Col) =>
    (hidden.shows(c) || colFilters[c].some((k) => isActive(cf[k]))) && optional.some((o) => o.id === c)
  const toggleColumn = (c: Col, visible: boolean) => {
    hidden.toggle(c, visible)
    if (!visible && colFilters[c].some((k) => isActive(cf[k]))) columns.clear(...colFilters[c])
  }

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const totalSize = rows.reduce((s, x) => s + sizeOf(x), 0)
  const showDownload = filter === "duplicate"
  // Ticks to mark uploads as never to archive, among the archivable ones or those already marked.
  const picking = filter === "archivable" || cf.archivable.includes("never")
  const picker = usePicker(picking ? rows.flatMap((x) => (x.t ? [{ hash: x.t.hash, archivable: archivableOf(x) }] : [])) : [])
  const kept = scoped.filter((x) => archivableOf(x)?.neverArchive).length
  const cols = 2 + optional.filter((o) => show(o.id)).length + (showDownload ? 1 : 0) + (picking ? 1 : 0)
  const selectedTorrent = selected ? (allTorrents.find((t) => t.hash === selected) ?? null) : null

  const reset = <T,>(fn: (v: T) => void) => (v: T) => {
    fn(v)
    setPage(0)
  }

  const toggleSort = (key: SortKey) =>
    setSort((s) => (s.key === key ? { key, desc: !s.desc } : { key, desc: key !== "name" && key !== "status" }))

  const SortIcon = ({ k }: { k: SortKey }) =>
    sort.key === k ? sort.desc ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" /> : null

  const addOne = (r: TrackerRelease) =>
    add.mutate([r.id], {
      onSuccess: () => {
        toast.success("Upload added to bookrr", { description: "Set where it is in its details." })
        setSelected(r.hash)
      },
    })

  // Besides Total, a counter shows only with something to do (or when chosen): at 0 it would be noise.
  const counters: { key: Filter; label: string; icon: React.ElementType; tone?: string }[] = [
    { key: "all", label: "Total", icon: Layers },
    { key: "duplicate", label: "Duplicates", icon: Copy, tone: "text-destructive" },
    { key: "missing", label: "To locate", icon: TriangleAlert, tone: "text-amber-600 dark:text-amber-400" },
    ...(hasTracker ? [{ key: "dead" as const, label: "Dead", icon: Skull, tone: "text-destructive" }] : []),
    { key: "archivable", label: "Archivable", icon: Archive, tone: archivableTone },
    ...(best ? [{ key: "better" as const, label: "Upgradable", icon: CircleArrowUp, tone: "text-amber-600 dark:text-amber-400" }] : []),
  ]
  const stats = counters.filter((s) => s.key === "all" || s.key === filter || counts[s.key] > 0)

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4">
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
              // The "Downloading" column, and so its filter, exists only among the duplicates.
              if (next !== "duplicate" && isActive(cf.download)) columns.clear("download")
              picker.clear()
              reset(setFilter)(next)
            }}
          />
        ))}
      </NavbarStats>

      {hasTracker && tracker && <TrackerPanel data={tracker} torrents={allTorrents} onOpen={setSelected} />}

      {filter === "dead" && (
        <p className="text-muted-foreground text-sm">
          Your uploads still on the tracker that nobody is seeding: seed them again from a client or an archive.
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => reset(setQuery)(e.target.value)}
            placeholder="Search by name, hash, type, disk, serial, adopter, path…"
            className="pl-8"
          />
        </div>
        <div className="flex items-center gap-2 px-1">
          <Switch id="only-personal" checked={onlyPersonal} onCheckedChange={reset(setOnlyPersonal)} />
          <Label htmlFor="only-personal" className="whitespace-nowrap">
            Only {info.data?.releaseTag ?? "Personal Release"}
          </Label>
        </div>
        <ColumnPicker columns={optional} shows={show} onToggle={toggleColumn} />
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
                      placeholder="Name, hash, category, tags, notes, ID…"
                    />
                  </ColumnFilter>
                </div>
              </TableHead>
              {show("size") && (
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
              )}
              {show("location") && (
                <TableHead>
                  <div className="flex items-center gap-1">
                    Location
                    <ColumnFilter label="Location" active={isActive(cf.where)} onClear={() => columns.clear("where")}>
                      {(
                        [
                          ["Client", "c", "Any client", (clients.data ?? []).map((c) => ({ value: `c:${c.id}`, label: <><Server /> {c.name}</> }))],
                          ["Disks", "d", "Any disk", (disks.data ?? []).map((d) => ({ value: `d:${d.id}`, label: <><HardDrive /> {d.label}</> }))],
                          ["Adopters", "a", "Any adopter", (adopters.data ?? []).map((a) => ({ value: `a:${a.id}`, label: <><HandHeart /> {a.name}</> }))],
                        ] as const
                      )
                        .filter(([, , , options]) => options.length > 0)
                        .map(([title, prefix, any, options]) => (
                          <OptionsFilter
                            key={title}
                            title={title}
                            options={[{ value: `${prefix}:*`, label: <span className="font-medium">{any}</span> }, ...options]}
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
              )}
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
              {show("type") && (
                <TableHead className="hidden md:table-cell">
                  <div className="flex items-center gap-1">
                    Type
                    <ColumnFilter
                      label="Type"
                      active={isActive(cf.type) || isActive(cf.resolution)}
                      onClear={() => columns.clear("type", "resolution")}
                    >
                      <OptionsFilter
                        title="Type"
                        options={types.map((t) => ({ value: t, label: t }))}
                        value={cf.type}
                        onChange={(v) => columns.set("type", v)}
                      />
                      <OptionsFilter
                        title="Resolution"
                        options={resolutions.map((t) => ({ value: t, label: t }))}
                        value={cf.resolution}
                        onChange={(v) => columns.set("resolution", v)}
                      />
                    </ColumnFilter>
                  </div>
                </TableHead>
              )}
              {show("best") && best && (
                <TableHead className="hidden xl:table-cell">
                  <div className="flex items-center gap-1">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span>Max</span>
                      </TooltipTrigger>
                      <TooltipContent className="max-w-sm">
                        The best existing version of each film, in any language, searched with Prowlarr once a day (each film
                        every {best.recheck}): the type counts first (Remux &gt; Encode &gt; WEB-DL &gt; WEBRip &gt; HDTV), then the
                        resolution, so a 1080p Remux beats a 2160p WEB-DL.{" "}
                        {best.running
                          ? `Searching: ${best.done} of ${best.total} films.`
                          : best.lastRunAt
                            ? `Last search ${formatRelative(best.lastRunAt)}: ${best.lastResult}.`
                            : `First search ${formatRelative(best.nextRunAt)}.`}
                      </TooltipContent>
                    </Tooltip>
                    <ColumnFilter label="Max" active={isActive(cf.best)} onClear={() => columns.clear("best")}>
                      <OptionsFilter
                        options={[
                          { value: "better", label: "Exists in a better version" },
                          { value: "max", label: "You have the best" },
                          { value: "none", label: "Not found" },
                        ]}
                        value={cf.best}
                        onChange={(v) => columns.set("best", v)}
                      />
                    </ColumnFilter>
                  </div>
                </TableHead>
              )}
              {show("uploaded") && (
                <TableHead className="hidden 2xl:table-cell">
                  <div className="flex items-center gap-1">
                    <button className="flex items-center gap-1" onClick={() => toggleSort("date")}>
                      Uploaded <SortIcon k="date" />
                    </button>
                    <ColumnFilter label="Uploaded" active={isActive(cf.date)} onClear={() => columns.clear("date")}>
                      <RangeFilter type="date" value={cf.date} onChange={(v) => columns.set("date", v)} />
                    </ColumnFilter>
                  </div>
                </TableHead>
              )}
              {show("peers") && (
                <TableHead className="hidden sm:table-cell">
                  <span className="flex items-center gap-1.5">
                    <button className="flex items-center gap-0.5" title="Seeders: sort" onClick={() => toggleSort("seeders")}>
                      <ArrowUp className="size-3.5 text-emerald-600 dark:text-emerald-400" />
                      Seeds <SortIcon k="seeders" />
                    </button>
                    <button className="flex items-center gap-0.5" title="Completed: sort" onClick={() => toggleSort("completed")}>
                      <CheckCheck className="text-muted-foreground size-3.5" />
                      <SortIcon k="completed" />
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
              )}
              {show("archivable") && (
                <TableHead className="hidden md:table-cell">
                  <div className="flex items-center gap-1">
                    <button className="flex items-center gap-1" title="Days the thresholds hold: sort" onClick={() => toggleSort("archivable")}>
                      Archivable <SortIcon k="archivable" />
                    </button>
                    <ColumnFilter
                      label="Archivable"
                      active={isActive(cf.archivable) || isActive(cf.archivableDays)}
                      onClear={() => columns.clear("archivable", "archivableDays")}
                    >
                      <OptionsFilter
                        options={[
                          { value: "yes", label: <><Archive className={archivableTone} /> Archivable</> },
                          { value: "no", label: "Not archivable" },
                          { value: "never", label: <><ArchiveX /> Never archive</> },
                        ]}
                        value={cf.archivable}
                        onChange={(v) => columns.set("archivable", v)}
                      />
                      <RangeFilter
                        title="Days the thresholds hold"
                        value={cf.archivableDays}
                        onChange={(v) => columns.set("archivableDays", v)}
                        parse={parseCount}
                      />
                    </ColumnFilter>
                  </div>
                </TableHead>
              )}
              <TableHead>
                <div className="flex items-center gap-1">
                  <button className="flex items-center gap-1" onClick={() => toggleSort("status")}>
                    Status <SortIcon k="status" />
                  </button>
                  <ColumnFilter
                    label="Status"
                    align="end"
                    active={isActive(cf.status) || isActive(cf.upload)}
                    onClear={() => columns.clear("status", "upload")}
                  >
                    <OptionsFilter
                      options={[
                        ...(["client", "missing", "archived", "adopted", "unknown"] as const).map((st) => ({
                          value: st,
                          label: <ReleaseStatusBadge status={st} />,
                        })),
                        ...(hasTracker ? [{ value: "untracked", label: <ReleaseStatusBadge status="untracked" /> }] : []),
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
                    {(hasTracker || isActive(cf.upload)) && (
                      <OptionsFilter
                        title="Tracker"
                        options={[
                          { value: "yes", label: <><CloudUpload /> Uploaded</> },
                          { value: "no", label: "Not uploaded" },
                          { value: "removed", label: <RemovedBadge /> },
                        ]}
                        value={cf.upload}
                        onChange={(v) => columns.set("upload", v)}
                      />
                    )}
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
            {!torrents.isLoading &&
              visible.map((x) => {
                const { t, r } = x
                const peers = peersOf(x)
                const archivable = archivableOf(x)
                const link = r?.detailsLink || t?.trackerUrl
                return (
                  <TableRow
                    key={x.key}
                    onClick={t ? () => setSelected(t.hash) : undefined}
                    className={cn(
                      t && "cursor-pointer",
                      t?.openAlert && "bg-warning/10 hover:bg-warning/20 shadow-[inset_3px_0_0_var(--warning)]",
                      !t?.openAlert && t?.duplicate && "bg-destructive/5 hover:bg-destructive/10 shadow-[inset_3px_0_0_var(--destructive)]",
                      !t && r?.removedAt && "opacity-70"
                    )}
                  >
                    {picking && (
                      <TableCell>
                        {t && archivable && (
                          <PickBox label={`Select ${t.name}`} checked={picker.isPicked(t.hash)} onChange={(on) => picker.toggle(t.hash, on)} />
                        )}
                      </TableCell>
                    )}
                    <TableCell className="max-w-0 min-w-[260px] whitespace-normal">
                      <div className="flex items-center gap-1.5">
                        <span className={cn("truncate font-medium", !t && r?.removedAt && "line-through")} title={nameOf(x)}>
                          {nameOf(x)}
                        </span>
                        {(t?.personalRelease ?? r?.personalRelease) && <PersonalBadge />}
                        {t?.duplicate && <DuplicateBadge count={t.locations.length} />}
                        {r?.removedAt && <RemovedBadge since={r.removedAt} />}
                        {link && (
                          <a
                            href={link}
                            target="_blank"
                            rel="noreferrer"
                            aria-label="Open on the tracker"
                            title="Open on the tracker"
                            className="text-muted-foreground hover:text-foreground shrink-0"
                            onClick={(e) => e.stopPropagation()}
                          >
                            <ExternalLink className="size-3.5" />
                          </a>
                        )}
                      </div>
                      {t ? (
                        (t.category || t.tags.length > 0 || t.notes) && (
                          <div className="text-muted-foreground truncate text-xs">
                            {[t.category, t.tags.join(", "), t.notes].filter(Boolean).join(" · ")}
                          </div>
                        )
                      ) : (
                        <div className="text-muted-foreground truncate text-xs">
                          {[r?.category, r?.hash ? null : r?.hashError ? `hash not computed: ${r.hashError}` : "hash to compute"]
                            .filter(Boolean)
                            .join(" · ")}
                        </div>
                      )}
                    </TableCell>
                    {show("size") && <TableCell className="text-right tabular-nums">{formatBytes(sizeOf(x))}</TableCell>}
                    {show("location") && <TableCell className="whitespace-normal">{t ? <WhereBadges t={t} /> : <Dash />}</TableCell>}
                    {showDownload && <TableCell className="whitespace-normal">{t && <DownloadCell t={t} />}</TableCell>}
                    {show("type") && (
                      <TableCell className="hidden md:table-cell">
                        {r && (r.type || r.resolution) ? (
                          <div className="flex gap-1">
                            {r.type && <Badge variant="secondary">{r.type}</Badge>}
                            {r.resolution && <Badge variant="outline">{r.resolution}</Badge>}
                          </div>
                        ) : (
                          <Dash />
                        )}
                      </TableCell>
                    )}
                    {show("best") && best && (
                      <TableCell className="hidden xl:table-cell">
                        <BestBadge b={r?.best} />
                      </TableCell>
                    )}
                    {show("uploaded") && (
                      <TableCell className="text-muted-foreground hidden text-xs whitespace-nowrap 2xl:table-cell">
                        {r ? <span title={formatDate(r.createdAt)}>{formatDay(r.createdAt)}</span> : <Dash />}
                      </TableCell>
                    )}
                    {show("peers") && <TableCell className="hidden sm:table-cell">{peers ? <PeerCounts p={peers} /> : <Dash />}</TableCell>}
                    {show("archivable") && (
                      <TableCell className="hidden md:table-cell">
                        <ArchivableBadge a={archivable} />
                        <WaitingArchivableBadge a={archivable} />
                        <NeverArchiveBadge a={archivable} />
                      </TableCell>
                    )}
                    <TableCell>
                      {t?.openAlert ? (
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
                        <div className="flex flex-wrap items-center gap-1.5">
                          <ReleaseStatusBadge status={statusOf(x)} />
                          {!t && r && (
                            <Button
                              size="sm"
                              variant="outline"
                              className="h-7"
                              disabled={!r.hash || add.isPending}
                              title={r.hash ? undefined : "Hash not computed yet"}
                              onClick={() => addOne(r)}
                            >
                              <Plus /> Add to bookrr
                            </Button>
                          )}
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
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
              <Button variant="ghost" size="icon-sm" disabled={current >= pages - 1} onClick={() => setPage(current + 1)}>
                <ChevronRight />
              </Button>
            </div>
          )}
        </div>
      </TableCard>

      <TorrentSheet torrent={selectedTorrent} onClose={() => setSelected(null)} onResolve={onResolve} onHashChange={setSelected} />
      <TorrentFormDialog open={adding} onOpenChange={setAdding} />
      <ImportDialog open={importing} onOpenChange={setImporting} />
    </div>
  )
}
