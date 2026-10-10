import { useMemo, useState } from "react"
import { ArrowLeft, FolderOpen, HardDrive, MapPin, QrCode, Search } from "lucide-react"

import { isPhysicalDisk } from "@/components/disk-form-dialog"
import { DiskQrDialog } from "@/components/disk-qr-dialog"
import { PersonalBadge } from "@/components/torrent-badges"
import { TorrentSheet } from "@/components/torrent-sheet"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import type { Alert, Disk } from "@/lib/api"
import { formatBytes, formatDate } from "@/lib/format"
import { useDisks, useTorrents } from "@/lib/queries"

/** The torrents archived on one disk: the page opened by the disk's QR code. */
export function DiskArchivesPage({
  diskId,
  onBack,
  onResolve,
}: {
  diskId: number
  onBack: () => void
  onResolve: (a: Alert) => void
}) {
  const disks = useDisks()
  const torrents = useTorrents()
  const [query, setQuery] = useState("")
  const [selected, setSelected] = useState<string | null>(null)
  const [qr, setQr] = useState<Disk | null>(null)

  const disk = disks.data?.find((d) => d.id === diskId)
  const all = useMemo(() => torrents.data ?? [], [torrents.data])
  const entries = useMemo(() => {
    const q = query.trim().toLowerCase()
    return all
      .flatMap((t) => t.archives.filter((a) => a.diskId === diskId).map((a) => ({ t, a })))
      .filter(({ t, a }) => !q || t.name.toLowerCase().includes(q) || a.path.toLowerCase().includes(q) || a.notes.toLowerCase().includes(q))
      .sort((x, y) => x.t.name.localeCompare(y.t.name, "it", { sensitivity: "base" }))
  }, [all, diskId, query])
  const selectedTorrent = selected ? (all.find((t) => t.hash === selected) ?? null) : null

  const back = (
    <Button variant="ghost" size="sm" className="w-fit" onClick={onBack}>
      <ArrowLeft /> Tutti i dischi
    </Button>
  )

  if (disks.isLoading || torrents.isLoading) return <Skeleton className="h-48 w-full" />
  if (!disk)
    return (
      <div className="grid gap-4">
        {back}
        <Card>
          <CardContent className="text-muted-foreground py-6 text-center text-sm">Questo disco non esiste più.</CardContent>
        </Card>
      </div>
    )

  return (
    <div className="grid gap-4">
      {back}
      <Card className="gap-3">
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            <HardDrive className="text-violet-600 size-4" />
            {disk.label}
            <Badge variant="outline">{disk.kind}</Badge>
          </CardTitle>
          <CardDescription className="font-mono text-xs">{disk.serial ? `SN ${disk.serial}` : "Seriale non indicato"}</CardDescription>
          {isPhysicalDisk(disk.kind) && (
            <CardAction>
              <Button variant="outline" size="sm" onClick={() => setQr(disk)}>
                <QrCode /> QR code
              </Button>
            </CardAction>
          )}
        </CardHeader>
        <CardContent className="grid gap-1.5 text-sm">
          {disk.model && <div className="text-muted-foreground">{disk.model}</div>}
          {disk.place && (
            <div className="flex items-center gap-1.5">
              <MapPin className="text-muted-foreground size-3.5" /> {disk.place}
            </div>
          )}
          <div>
            <span className="font-medium">{disk.archiveCount}</span> torrent · {formatBytes(disk.archivedSize)}
            {disk.capacity > 0 && <span className="text-muted-foreground"> su {formatBytes(disk.capacity)}</span>}
          </div>
          {disk.notes && <p className="text-muted-foreground text-xs whitespace-pre-wrap">{disk.notes}</p>}
        </CardContent>
      </Card>

      <div className="relative">
        <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
        <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Cerca per nome, cartella, note…" className="pl-8" />
      </div>

      <Card className="py-0">
        <CardContent className="px-0">
          {entries.length === 0 ? (
            <p className="text-muted-foreground py-6 text-center text-sm">
              {query ? "Nessun torrent trovato." : "Nessun torrent archiviato su questo disco."}
            </p>
          ) : (
            <ul className="divide-y">
              {entries.map(({ t, a }) => (
                <li key={a.id}>
                  <button
                    type="button"
                    className="hover:bg-muted/50 grid w-full gap-1 px-4 py-3 text-left"
                    onClick={() => setSelected(t.hash)}
                  >
                    <div className="flex items-start gap-2">
                      <span className="min-w-0 flex-1 font-medium break-words">{t.name}</span>
                      <span className="text-muted-foreground shrink-0 text-xs tabular-nums">{formatBytes(t.size)}</span>
                    </div>
                    <div className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                      {t.personalRelease && <PersonalBadge />}
                      {a.path && (
                        <span className="flex min-w-0 items-center gap-1 font-mono break-all">
                          <FolderOpen className="size-3.5 shrink-0" /> {a.path}
                        </span>
                      )}
                      <span>archiviato il {formatDate(a.createdAt)}</span>
                    </div>
                    {a.notes && <p className="text-muted-foreground text-xs whitespace-pre-wrap">{a.notes}</p>}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <DiskQrDialog disk={qr} onClose={() => setQr(null)} />
      <TorrentSheet torrent={selectedTorrent} onClose={() => setSelected(null)} onResolve={onResolve} onHashChange={setSelected} />
    </div>
  )
}
