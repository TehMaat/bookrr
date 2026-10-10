import { useState } from "react"
import { Activity, Clock, HardDrive, ListTree, MapPin, Pencil, Plus, QrCode, Trash2 } from "lucide-react"

import { DiskArchivesPage } from "@/components/disk-archives-page"
import { DiskFormDialog, isPhysicalDisk } from "@/components/disk-form-dialog"
import { DiskQrDialog } from "@/components/disk-qr-dialog"
import { SmartImportDialog } from "@/components/smart-import-dialog"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { api, type Alert, type Disk } from "@/lib/api"
import { formatBytes, formatDate, formatHours, healthVariant } from "@/lib/format"
import { useAction, useDisks } from "@/lib/queries"

export function DisksPage({
  diskId,
  onOpenDisk,
  onResolve,
}: {
  /** Show the torrents archived on this disk instead of the list of disks. */
  diskId: number | null
  onOpenDisk: (id: number | null) => void
  onResolve: (a: Alert) => void
}) {
  if (diskId !== null) return <DiskArchivesPage diskId={diskId} onBack={() => onOpenDisk(null)} onResolve={onResolve} />
  return <DiskList onOpenDisk={onOpenDisk} />
}

function DiskList({ onOpenDisk }: { onOpenDisk: (id: number) => void }) {
  const disks = useDisks()
  const [editing, setEditing] = useState<Disk | null>(null)
  const [open, setOpen] = useState(false)
  const [deleting, setDeleting] = useState<Disk | null>(null)
  const [smartOpen, setSmartOpen] = useState(false)
  const [smartDisk, setSmartDisk] = useState<Disk | null>(null)
  const [qr, setQr] = useState<Disk | null>(null)
  const openSmart = (d: Disk | null) => {
    setSmartDisk(d)
    setSmartOpen(true)
  }
  const del = useAction((id: number) => api.deleteDisk(id), "Disco eliminato")

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground text-sm">Dischi e archivi offline su cui conservi i torrent tolti dai client.</p>
        <Button className="ml-auto" variant="outline" onClick={() => openSmart(null)}>
          <Activity /> Importa da SMART
        </Button>
        <Button
          onClick={() => {
            setEditing(null)
            setOpen(true)
          }}
        >
          <Plus /> Nuovo disco
        </Button>
      </div>

      {disks.isLoading && <Skeleton className="h-32 w-full" />}
      {disks.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-6 text-center text-sm">
            Nessun disco registrato. Aggiungine uno per poter indicare dove archivi i torrent.
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {disks.data?.map((d) => {
          const usage = d.capacity > 0 ? Math.min(100, (d.archivedSize / d.capacity) * 100) : null
          return (
            <Card key={d.id} className="gap-4">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <HardDrive className="text-violet-600 size-4" />
                  {d.label}
                  <Badge variant="outline">{d.kind}</Badge>
                </CardTitle>
                <CardDescription className="font-mono text-xs">{d.serial ? `SN ${d.serial}` : "Seriale non indicato"}</CardDescription>
                <CardAction className="flex gap-1">
                  {isPhysicalDisk(d.kind) && (
                    <Button variant="ghost" size="icon-sm" aria-label="QR code" title="QR code" onClick={() => setQr(d)}>
                      <QrCode />
                    </Button>
                  )}
                  <Button variant="ghost" size="icon-sm" aria-label="Aggiorna da SMART" title="Aggiorna da SMART" onClick={() => openSmart(d)}>
                    <Activity />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Modifica"
                    onClick={() => {
                      setEditing(d)
                      setOpen(true)
                    }}
                  >
                    <Pencil />
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label="Elimina" onClick={() => setDeleting(d)}>
                    <Trash2 />
                  </Button>
                </CardAction>
              </CardHeader>
              <CardContent className="grid gap-2 text-sm">
                {d.model && (
                  <div className="text-muted-foreground">
                    {d.model}
                    {d.firmware && <span className="font-mono text-xs whitespace-nowrap"> · FW {d.firmware}</span>}
                  </div>
                )}
                {d.smartAt && (
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    {d.health && <Badge variant={healthVariant(d.health)}>SMART {d.health}</Badge>}
                    {d.powerOnHours > 0 && (
                      <span className="flex items-center gap-1">
                        <Clock className="text-muted-foreground size-3.5" /> {formatHours(d.powerOnHours)}
                      </span>
                    )}
                    <span className="text-muted-foreground text-xs">letto il {formatDate(d.smartAt)}</span>
                  </div>
                )}
                {d.place && (
                  <div className="flex items-center gap-1.5">
                    <MapPin className="text-muted-foreground size-3.5" /> {d.place}
                  </div>
                )}
                <div>
                  <span className="font-medium">{d.archiveCount}</span> torrent · {formatBytes(d.archivedSize)}
                  {d.capacity > 0 && <span className="text-muted-foreground"> su {formatBytes(d.capacity)}</span>}
                </div>
                {usage !== null && (
                  <div className="bg-muted h-1.5 overflow-hidden rounded-full">
                    <div className="h-full rounded-full bg-violet-500" style={{ width: `${usage}%` }} />
                  </div>
                )}
                {d.notes && <p className="text-muted-foreground text-xs whitespace-pre-wrap">{d.notes}</p>}
                <Button variant="outline" size="sm" className="mt-1 w-fit" onClick={() => onOpenDisk(d.id)}>
                  <ListTree /> Torrent archiviati
                </Button>
              </CardContent>
            </Card>
          )
        })}
      </div>

      <DiskFormDialog open={open} disk={editing} onOpenChange={setOpen} />
      <SmartImportDialog open={smartOpen} disk={smartDisk} onOpenChange={setSmartOpen} />
      <DiskQrDialog disk={qr} onClose={() => setQr(null)} />
      <AlertDialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Eliminare il disco "{deleting?.label}"?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting?.archiveCount
                ? `${deleting.archiveCount} archivi resteranno registrati con il solo percorso, senza disco.`
                : "Nessun torrent è associato a questo disco."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annulla</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => deleting && del.mutate(deleting.id)}
            >
              Elimina
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
