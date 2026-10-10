import { useMemo, useState } from "react"
import {
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

import { ImportDialog } from "@/components/import-dialog"
import { canRemoveDuplicate, RemoveFromClientDialog } from "@/components/remove-from-client-dialog"
import { DuplicateBadge, PersonalBadge, StatusBadge, WhereBadges } from "@/components/torrent-badges"
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Alert, Location, Torrent } from "@/lib/api"
import { formatBytes, formatState, formatUnix, isDownloading } from "@/lib/format"
import { useAdopters, useClients, useDisks, useInfo, useTorrents } from "@/lib/queries"
import { cn } from "@/lib/utils"

type Filter = "all" | "client" | "duplicate" | "missing" | "archived" | "adopted" | "unknown"
type SortKey = "name" | "size" | "status"

const PAGE_SIZE = 50

const statusOrder = { missing: 0, unknown: 1, client: 2, archived: 3, adopted: 4 }

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
  const [client, setClient] = useState("all")
  const [disk, setDisk] = useState("all")
  const [adopter, setAdopter] = useState("all")
  const [onlyPersonal, setOnlyPersonal] = useState(false)
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "name", desc: false })
  const [page, setPage] = useState(0)
  const [selected, setSelected] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [importing, setImporting] = useState(false)

  const all = useMemo(() => torrents.data ?? [], [torrents.data])
  const scoped = useMemo(() => (onlyPersonal ? all.filter((t) => t.personalRelease) : all), [all, onlyPersonal])

  const counts = useMemo(() => {
    const c: Record<Filter, number> = { all: 0, client: 0, duplicate: 0, missing: 0, archived: 0, adopted: 0, unknown: 0 }
    for (const t of scoped) for (const f of Object.keys(c) as Filter[]) if (matches(t, f)) c[f]++
    return c
  }, [scoped])

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const out = scoped.filter((t) => {
      if (!matches(t, filter)) return false
      if (client !== "all" && !t.locations.some((l) => String(l.clientId) === client)) return false
      if (disk !== "all" && !t.archives.some((a) => String(a.diskId) === disk)) return false
      if (adopter !== "all" && !t.adoptions.some((a) => String(a.adopterId) === adopter)) return false
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
      else if (sort.key === "status") c = statusOrder[a.status] - statusOrder[b.status]
      if (c === 0) c = a.name.localeCompare(b.name, "it", { sensitivity: "base" })
      return c * dir
    })
    return out
  }, [scoped, filter, client, disk, adopter, query, sort])

  const pages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE))
  const current = Math.min(page, pages - 1)
  const visible = rows.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE)
  const totalSize = rows.reduce((s, t) => s + t.size, 0)
  const showDownload = filter === "duplicate"
  const cols = showDownload ? 5 : 4
  const selectedTorrent = selected ? (all.find((t) => t.hash === selected) ?? null) : null

  const reset = <T,>(fn: (v: T) => void) => (v: T) => {
    fn(v)
    setPage(0)
  }

  const toggleSort = (key: SortKey) =>
    setSort((s) => (s.key === key ? { key, desc: !s.desc } : { key, desc: key === "size" }))

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
  ]

  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-7">
        {stats.map((s) => (
          <button
            key={s.key}
            type="button"
            onClick={() => reset(setFilter)(filter === s.key && s.key !== "all" ? "all" : s.key)}
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
        <Select value={client} onValueChange={reset(setClient)}>
          <SelectTrigger className="w-[160px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tutti i client</SelectItem>
            {clients.data?.map((c) => (
              <SelectItem key={c.id} value={String(c.id)}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={disk} onValueChange={reset(setDisk)}>
          <SelectTrigger className="w-[160px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tutti i dischi</SelectItem>
            {disks.data?.map((d) => (
              <SelectItem key={d.id} value={String(d.id)}>
                {d.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={adopter} onValueChange={reset(setAdopter)}>
          <SelectTrigger className="w-[190px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tutti gli adottatori</SelectItem>
            {adopters.data?.map((a) => (
              <SelectItem key={a.id} value={String(a.id)}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
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
                <button className="flex items-center gap-1" onClick={() => toggleSort("name")}>
                  Nome <SortIcon k="name" />
                </button>
              </TableHead>
              <TableHead className="text-right">
                <button className="ml-auto flex items-center gap-1" onClick={() => toggleSort("size")}>
                  Dimensione <SortIcon k="size" />
                </button>
              </TableHead>
              <TableHead>Dove si trova</TableHead>
              {showDownload && <TableHead>In download</TableHead>}
              <TableHead>
                <button className="flex items-center gap-1" onClick={() => toggleSort("status")}>
                  Stato <SortIcon k="status" />
                </button>
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
