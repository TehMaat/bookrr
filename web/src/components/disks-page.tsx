import { useState } from "react"
import { Activity, Clock, Cloud, HardDrive, ListTree, MapPin, Pencil, Plus, QrCode, RefreshCw, Trash2 } from "lucide-react"

import { BucketStatus } from "@/components/bucket-status"
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
import { useAction, useDisks, useScanDisk } from "@/lib/queries"

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
  const del = useAction((id: number) => api.deleteDisk(id), "Disk deleted")
  const scan = useScanDisk()

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground text-sm">Offline disks and archives where you keep the torrents removed from the clients.</p>
        <Button className="ml-auto" variant="outline" onClick={() => openSmart(null)}>
          <Activity /> Import from SMART
        </Button>
        <Button
          onClick={() => {
            setEditing(null)
            setOpen(true)
          }}
        >
          <Plus /> New disk
        </Button>
      </div>

      {disks.isLoading && <Skeleton className="h-32 w-full" />}
      {disks.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-6 text-center text-sm">
            No disks yet. Add one to record where you archive your torrents.
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {disks.data?.map((d) => {
          const usage = d.capacity > 0 ? Math.min(100, (d.archivedSize / d.capacity) * 100) : null
          return (
            <Card
              key={d.id}
              role="link"
              tabIndex={0}
              aria-label={`Torrents archived on ${d.label}`}
              className="hover:bg-accent/50 focus-visible:ring-ring/50 cursor-pointer gap-4 transition-colors outline-none focus-visible:ring-[3px]"
              onClick={() => {
                // Selecting text, like the serial, does not open the disk.
                if (!window.getSelection()?.toString()) onOpenDisk(d.id)
              }}
              onKeyDown={(e) => e.target === e.currentTarget && e.key === "Enter" && onOpenDisk(d.id)}
            >
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  {d.s3 ? <Cloud className="text-violet-600 size-4" /> : <HardDrive className="text-violet-600 size-4" />}
                  {d.label}
                  <Badge variant="outline">{d.kind}</Badge>
                </CardTitle>
                <CardDescription className="font-mono text-xs break-all">
                  {d.s3 ? `s3://${d.s3.bucket}/${d.s3.prefix}` : d.serial ? `SN ${d.serial}` : "No serial number"}
                </CardDescription>
                <CardAction className="flex gap-1" onClick={(e) => e.stopPropagation()}>
                  {isPhysicalDisk(d.kind) && (
                    <Button variant="ghost" size="icon-sm" aria-label="QR code" title="QR code" onClick={() => setQr(d)}>
                      <QrCode />
                    </Button>
                  )}
                  {d.s3 ? (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Read the bucket"
                      title="Read the bucket"
                      disabled={scan.isPending && scan.variables === d.id}
                      onClick={() => scan.mutate(d.id)}
                    >
                      <RefreshCw className={scan.isPending && scan.variables === d.id ? "animate-spin" : undefined} />
                    </Button>
                  ) : (
                    <Button variant="ghost" size="icon-sm" aria-label="Update from SMART" title="Update from SMART" onClick={() => openSmart(d)}>
                      <Activity />
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Edit"
                    onClick={() => {
                      setEditing(d)
                      setOpen(true)
                    }}
                  >
                    <Pencil />
                  </Button>
                  <Button variant="ghost" size="icon-sm" aria-label="Delete" onClick={() => setDeleting(d)}>
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
                    <span className="text-muted-foreground text-xs">read on {formatDate(d.smartAt)}</span>
                  </div>
                )}
                {d.place && (
                  <div className="flex items-center gap-1.5">
                    <MapPin className="text-muted-foreground size-3.5" /> {d.place}
                  </div>
                )}
                {d.s3 && <BucketStatus disk={d} />}
                <div className="flex items-center gap-1.5">
                  <ListTree className="text-muted-foreground size-3.5" />
                  <span>
                    <span className="font-medium">{d.archiveCount}</span> {d.archiveCount === 1 ? "torrent" : "torrents"} · {formatBytes(d.archivedSize)}
                    {d.capacity > 0 && <span className="text-muted-foreground"> of {formatBytes(d.capacity)}</span>}
                  </span>
                </div>
                {usage !== null && (
                  <div className="bg-muted h-1.5 overflow-hidden rounded-full">
                    <div className="h-full rounded-full bg-violet-500" style={{ width: `${usage}%` }} />
                  </div>
                )}
                {d.notes && <p className="text-muted-foreground text-xs whitespace-pre-wrap">{d.notes}</p>}
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
            <AlertDialogTitle>Delete the disk "{deleting?.label}"?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleting?.archiveCount
                ? `${deleting.archiveCount} archives will be kept with their path only, without a disk.`
                : "No torrents are on this disk."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => deleting && del.mutate(deleting.id)}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

