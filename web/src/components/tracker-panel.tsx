import { useState } from "react"
import { ChevronDown, ChevronRight, CloudOff, Plus, RefreshCw, TagX, TriangleAlert } from "lucide-react"
import { toast } from "sonner"

import { StatusBadge } from "@/components/torrent-badges"
import { Alert as AlertBox, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, type Releases, type Torrent } from "@/lib/api"
import { formatRelative, plural } from "@/lib/format"
import { useAction, useInfo } from "@/lib/queries"
import { cn } from "@/lib/utils"

/**
 * Above the list of torrents, with a UNIT3D tracker: when the uploads were last read (and Refresh now), adding the
 * uploads bookrr doesn't know, the errors, and the personal releases of bookrr that are not among the uploads.
 */
export function TrackerPanel({
  data,
  torrents,
  onOpen,
}: {
  data: Releases
  torrents: Torrent[]
  onOpen: (hash: string) => void
}) {
  const info = useInfo()
  const [addingAll, setAddingAll] = useState(false)
  const [unflaggingAll, setUnflaggingAll] = useState(false)
  const [showNotOnTracker, setShowNotOnTracker] = useState(false)
  const syncNow = useAction(() => api.syncReleases())
  const add = useAction((ids: number[] | "all") => api.addReleases(ids))
  const unflag = useAction((hashes: string[] | "all") => api.unflagReleases(hashes))

  const state = data.state
  const running = !!state?.running || syncNow.isPending
  const addable = data.releases.filter((r) => r.status === "untracked" && r.hash)
  const tag = info.data?.releaseTag ?? "Personal Release"
  const best = data.best

  const refresh = () =>
    syncNow.mutate(undefined, {
      onSuccess: () => toast.success("Refresh started", { description: "Uploads are being read from the tracker in the background." }),
    })
  const unflagDone = (r: { unflagged: number; errors: string[] }) => {
    const what = r.unflagged === 1 ? "1 torrent is no longer" : `${r.unflagged} torrents are no longer`
    if (r.errors.length === 0) toast.success(`${what} ${tag}`)
    else toast.warning(`${what} ${tag}`, { description: `Tag not removed from: ${r.errors.join("; ")}` })
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground hidden text-sm sm:block">
          Your uploads on the tracker
          {state?.username && (
            <>
              {" "}
              as <span className="text-foreground font-medium">{state.username}</span>
            </>
          )}
          {" "}are linked to the torrents with the same hash.
        </p>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground text-xs">
                {running
                  ? state?.pending
                    ? `Downloading the .torrent files (${state.pending} to go)…`
                    : "Refreshing…"
                  : `Refreshed ${formatRelative(data.syncedAt)}`}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {state?.lastResult || "No refresh yet"}
              {info.data?.releasesInterval && ` · every ${info.data.releasesInterval}`}
            </TooltipContent>
          </Tooltip>
          {addable.length > 0 && (
            <Button variant="outline" size="sm" onClick={() => setAddingAll(true)} disabled={add.isPending}>
              <Plus /> Add all to bookrr ({addable.length})
            </Button>
          )}
          {data.enabled && (
            <Button variant="outline" size="sm" disabled={running} onClick={refresh}>
              <RefreshCw className={cn(running && "animate-spin")} /> Refresh now
            </Button>
          )}
        </div>
      </div>

      {!data.enabled && (
        <AlertBox variant="warning">
          <CloudOff />
          <AlertTitle>UNIT3D tracker not configured</AlertTitle>
          <AlertDescription>Uploads are no longer refreshed: set the tracker in Settings, Tracker tab.</AlertDescription>
        </AlertBox>
      )}
      {state?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Last refresh of the uploads failed</AlertTitle>
          <AlertDescription>{state.lastError}</AlertDescription>
        </AlertBox>
      )}
      {best?.lastError && (
        <AlertBox variant="destructive">
          <TriangleAlert />
          <AlertTitle>Last Prowlarr search failed</AlertTitle>
          <AlertDescription>{best.lastError}</AlertDescription>
        </AlertBox>
      )}

      {data.notOnTracker.length > 0 && (
        <Card className="border-warning/60 gap-3 py-4">
          <CardHeader>
            <CardTitle>
              <button
                type="button"
                className="flex items-center gap-2 text-left"
                onClick={() => setShowNotOnTracker((v) => !v)}
                aria-expanded={showNotOnTracker}
              >
                <TriangleAlert className="size-4 shrink-0 text-amber-600" />
                {tag} not found among your uploads ({data.notOnTracker.length})
                {showNotOnTracker ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
              </button>
            </CardTitle>
            {showNotOnTracker && (
              <CardDescription>
                bookrr torrents flagged {tag} whose hash is not among your uploads on the tracker: deleted or replaced,
                uploaded elsewhere, or added without a hash. If they aren't your releases, remove the flag: the tag is
                removed on qBittorrent too.
              </CardDescription>
            )}
            <CardAction>
              <Button variant="outline" size="sm" disabled={unflag.isPending} onClick={() => setUnflaggingAll(true)}>
                <TagX /> Unflag all
              </Button>
            </CardAction>
          </CardHeader>
          {showNotOnTracker && (
            <CardContent>
              <ul className="max-h-56 divide-y overflow-y-auto rounded-lg border">
                {data.notOnTracker.map((m) => {
                  const t = torrents.find((t) => t.hash === m.hash)
                  return (
                    <li key={m.hash} className="flex items-center gap-2 py-0.5 pr-1 pl-3">
                      <button
                        type="button"
                        className="min-w-0 flex-1 truncate text-left text-sm hover:underline"
                        title={m.name}
                        onClick={() => onOpen(m.hash)}
                      >
                        {m.name}
                      </button>
                      {m.hash.startsWith("manual-") && (
                        <Badge variant="outline" className="text-muted-foreground">
                          no hash
                        </Badge>
                      )}
                      {t && <StatusBadge status={t.status} />}
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`Remove the ${tag} flag`}
                            disabled={unflag.isPending}
                            onClick={() => unflag.mutate([m.hash], { onSuccess: unflagDone })}
                          >
                            <TagX />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Remove the {tag} flag, on qBittorrent too</TooltipContent>
                      </Tooltip>
                    </li>
                  )
                })}
              </ul>
            </CardContent>
          )}
        </Card>
      )}

      <AlertDialog open={unflaggingAll} onOpenChange={setUnflaggingAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove the {tag} flag from {plural(data.notOnTracker.length, "torrent")}?</AlertDialogTitle>
            <AlertDialogDescription>
              They won't be considered your releases anymore: bookrr no longer reports them when they disappear from the
              clients. On the qBittorrent clients only the "{tag}" tag is removed: torrents and files stay where they are.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => unflag.mutate("all", { onSuccess: unflagDone })}>Remove flag</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={addingAll} onOpenChange={setAddingAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Add {plural(addable.length, "upload")} to bookrr?</AlertDialogTitle>
            <AlertDialogDescription>
              Each upload bookrr doesn't know becomes a torrent with its hash, name, size, .torrent file and the Personal
              Release flag. You can then set where it is (disk, adopter) from the torrent details.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => add.mutate("all", { onSuccess: (r) => toast.success(`${plural(r.added, "upload")} added to bookrr`) })}
            >
              Add all
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
