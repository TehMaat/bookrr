import { useEffect, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Check, ExternalLink, Search } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { api, type Torrent } from "@/lib/api"
import { formatBytes, formatDate } from "@/lib/format"
import { useAction } from "@/lib/queries"

/**
 * Lets the user pick on the tracker the torrent a hand-entered one refers
 * to, when its name differs too much to be matched automatically.
 */
export function TrackerPickDialog({
  torrent,
  open,
  onOpenChange,
  onPicked,
}: {
  torrent: Torrent
  open: boolean
  onOpenChange: (o: boolean) => void
  onPicked: (hash: string) => void
}) {
  const [text, setText] = useState("")
  const [query, setQuery] = useState("")
  useEffect(() => {
    if (!open) return
    setText("")
    setQuery("")
  }, [open])

  const candidates = useQuery({
    queryKey: ["tracker-candidates", torrent.hash, query],
    queryFn: () => api.trackerCandidates(torrent.hash, query),
    enabled: open,
    staleTime: 5 * 60_000,
    retry: false,
  })
  const choose = useAction(async (id: string) => {
    const t = await api.trackerMatch(torrent.hash, id)
    onPicked(t.hash)
    onOpenChange(false)
    return t
  }, "Hash e file .torrent salvati")

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Scegli il torrent sul tracker</DialogTitle>
          <DialogDescription className="break-all">
            Torrent in bookrr: <span className="text-foreground font-medium">{torrent.name}</span>
            {torrent.size > 0 && <> · {formatBytes(torrent.size)}</>}
          </DialogDescription>
        </DialogHeader>

        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            setQuery(text.trim())
          }}
        >
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Cerca altro sul tracker (vuoto = per nome, es. Ad Astra 2019)"
          />
          <Button type="submit" variant="outline">
            <Search /> Cerca
          </Button>
        </form>

        <div className="max-h-96 overflow-auto">
          {candidates.isFetching ? (
            <p className="text-muted-foreground py-6 text-center text-sm">Ricerca sul tracker…</p>
          ) : candidates.isError ? (
            <p className="text-destructive py-6 text-center text-sm">{candidates.error.message}</p>
          ) : candidates.data?.length === 0 ? (
            <p className="text-muted-foreground py-6 text-center text-sm">
              Nessun torrent trovato: prova a cercare solo titolo e anno.
            </p>
          ) : (
            <ul className="grid gap-2">
              {candidates.data?.map((c) => (
                <li key={c.id} className="flex items-start gap-3 rounded-lg border p-3 text-sm">
                  <div className="min-w-0 flex-1">
                    <div className="font-medium break-all">{c.name}</div>
                    <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                      <Badge variant={c.score >= 50 ? "success" : "secondary"}>{c.score}% parole in comune</Badge>
                      <span>{formatBytes(c.size)}</span>
                      {c.createdAt && <span>caricato il {formatDate(c.createdAt)}</span>}
                      {c.detailsLink && (
                        <a
                          href={c.detailsLink}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 underline-offset-2 hover:underline"
                        >
                          <ExternalLink className="size-3" /> Apri sul tracker
                        </a>
                      )}
                    </div>
                  </div>
                  <Button size="sm" disabled={choose.isPending} onClick={() => choose.mutate(c.id)}>
                    <Check /> Usa questo
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
