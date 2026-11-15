import { useEffect, useRef } from "react"
import { useQuery } from "@tanstack/react-query"
import { Disc3, ExternalLink, Loader2, RotateCcw, TriangleAlert, Upload } from "lucide-react"

import { MAX_PLAN_TORRENTS } from "@/components/plan-torrents"
import { Alert as AlertBox, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { api, type PlanItem } from "@/lib/api"
import { formatBytes, formatRelative } from "@/lib/format"
import { keys, useAction } from "@/lib/queries"

/**
 * The full discs of a film of the plan found with Prowlarr, with the button to load the .torrent of one on the
 * film. A film never searched is searched when the dialog opens; the search can take minutes.
 */
export function PlanDiscsDialog({ item, onClose }: { item: PlanItem | null; onClose: () => void }) {
  const id = item?.id ?? 0
  const discs = useQuery({
    queryKey: keys.planDiscs(id),
    queryFn: () => api.planDiscs(id),
    enabled: item !== null,
    refetchInterval: (q) => (q.state.data?.searching ? 2_000 : false),
  })
  const search = useAction((id: number) => api.searchPlanDiscs(id))
  const load = useAction(
    ({ id, disc }: { id: number; disc: string }) => api.loadPlanDisc(id, disc),
    "Full disc loaded on the plan",
  )

  const d = discs.data
  // Searched by itself once per film, when it never was.
  const started = useRef(new Set<number>())
  useEffect(() => {
    if (!item || !d || d.searching || d.checkedAt || d.error || started.current.has(item.id)) return
    started.current.add(item.id)
    search.mutate(item.id)
  }, [item, d, search])
  const searching = !!d?.searching || search.isPending
  const full = (item?.torrents.length ?? 0) >= MAX_PLAN_TORRENTS
  // Loaded on the film already: its .torrent is named as the release, mostly.
  const loaded = (title: string) => !!item?.torrents.some((t) => t.name === title)
  const loadLabel = item && item.torrents.length > 0 ? "Load as second (merge)" : "Load into plan"

  return (
    <Dialog open={item !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Full discs of "{item?.title}"</DialogTitle>
          <DialogDescription>
            The Blu-ray and UHD Blu-ray discs found with Prowlarr, best first. Load one and its .torrent goes on the
            film, as with "Load .torrent".
          </DialogDescription>
        </DialogHeader>

        {d?.error && (
          <AlertBox variant="destructive">
            <TriangleAlert />
            <AlertTitle>Last search failed</AlertTitle>
            <AlertDescription>{d.error}</AlertDescription>
          </AlertBox>
        )}

        {searching && (
          <p className="text-muted-foreground inline-flex items-center gap-2 text-sm">
            <Loader2 className="size-4 animate-spin" /> Searching Prowlarr… it can take a few minutes: every indexer
            is asked.
          </p>
        )}

        {d && d.checkedAt && d.discs.length === 0 && !searching && (
          <p className="text-muted-foreground inline-flex items-center gap-2 text-sm">
            <Disc3 className="size-4" /> No full disc of this film found.
          </p>
        )}

        {d && d.discs.length > 0 && (
          <div className="max-h-[60vh] overflow-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Release</TableHead>
                  <TableHead>Indexer</TableHead>
                  <TableHead className="text-right">Size</TableHead>
                  <TableHead className="text-right">Seeders</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {d.discs.map((disc) => (
                  <TableRow key={disc.id}>
                    <TableCell className="max-w-md">
                      <Badge variant="violet">{disc.quality || "Full Disc"}</Badge>
                      <div className="mt-1 text-xs break-all">
                        {disc.url ? (
                          <a href={disc.url} target="_blank" rel="noreferrer" className="hover:underline">
                            {disc.title} <ExternalLink className="inline size-3" />
                          </a>
                        ) : (
                          disc.title
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs">{disc.indexer}</TableCell>
                    <TableCell className="text-right text-xs whitespace-nowrap">
                      {disc.size > 0 ? formatBytes(disc.size) : "—"}
                    </TableCell>
                    <TableCell className="text-right text-xs">{disc.seeders}</TableCell>
                    <TableCell className="text-right">
                      {loaded(disc.title) ? (
                        <Badge variant="success">Loaded</Badge>
                      ) : (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            {/* A span: the tooltip shows on a disabled button too. */}
                            <span>
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={!disc.loadable || full || load.isPending}
                                onClick={() => item && load.mutate({ id: item.id, disc: disc.id })}
                              >
                                {load.isPending && load.variables?.disc === disc.id ? (
                                  <Loader2 className="animate-spin" />
                                ) : (
                                  <Upload />
                                )}
                                {loadLabel}
                              </Button>
                            </span>
                          </TooltipTrigger>
                          <TooltipContent>
                            {!disc.loadable
                              ? "Prowlarr has only a magnet link of this release"
                              : full
                                ? `The film has ${MAX_PLAN_TORRENTS} .torrent files already`
                                : "Downloads the .torrent through Prowlarr and loads it on the film"}
                          </TooltipContent>
                        </Tooltip>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <DialogFooter className="items-center sm:justify-between">
          <span className="text-muted-foreground text-xs">
            {d?.checkedAt ? `Searched ${formatRelative(d.checkedAt)}` : ""}
          </span>
          <div className="flex gap-2">
            <Button variant="outline" disabled={searching} onClick={() => item && search.mutate(item.id)}>
              <RotateCcw /> Search again
            </Button>
            <Button onClick={onClose}>Close</Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
