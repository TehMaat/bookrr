import { useEffect, useState } from "react"

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
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { api, type Location, type Torrent } from "@/lib/api"
import { isDownloading } from "@/lib/format"
import { useAction } from "@/lib/queries"

/** True when the torrent is on several clients and finished downloading on all of them. */
export function canRemoveDuplicate(t: Torrent): boolean {
  return t.duplicate && !t.locations.some(isDownloading)
}

/** Confirms removing a duplicate torrent from one client, optionally with its files. */
export function RemoveFromClientDialog({
  torrent,
  location,
  onClose,
}: {
  torrent: Torrent
  location: Location | null
  onClose: () => void
}) {
  const [deleteFiles, setDeleteFiles] = useState(false)
  const remove = useAction(
    (l: Location) => api.removeFromClient(torrent.hash, l.clientId, deleteFiles),
    location ? `Removed from ${location.clientName}` : undefined
  )

  useEffect(() => setDeleteFiles(false), [location?.clientId])

  const others = location ? torrent.locations.filter((l) => l.clientId !== location.clientId) : []
  const samePath = location ? others.filter((l) => l.savePath === location.savePath) : []

  return (
    <AlertDialog open={location !== null} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove from {location?.clientName}?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="grid gap-2">
              <p className="break-all">
                "{torrent.name}" will be removed from {location?.clientName} and stay on{" "}
                {others.map((l) => l.clientName).join(", ")}.
              </p>
              {location && <p className="font-mono text-xs break-all">{location.savePath}</p>}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="grid gap-2">
          <div className="flex items-center gap-2">
            <Switch id="rm-delete-files" checked={deleteFiles} onCheckedChange={setDeleteFiles} />
            <Label htmlFor="rm-delete-files">Also delete the downloaded files</Label>
          </div>
          {deleteFiles && samePath.length > 0 && (
            <p className="text-destructive text-xs">
              Warning: {samePath.map((l) => l.clientName).join(", ")} uses the same path too. If the clients share
              the same disk you would delete the other copy as well.
            </p>
          )}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive hover:bg-destructive/90 text-white"
            disabled={remove.isPending}
            onClick={(e) => {
              // Stay open (and keep the client name) until the client confirms.
              e.preventDefault()
              if (location) remove.mutate(location, { onSuccess: onClose })
            }}
          >
            Remove
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
