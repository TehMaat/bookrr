import { useRef, useState } from "react"
import { CircleCheck, TriangleAlert, X } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Alert } from "@/lib/api"
import { formatDate } from "@/lib/format"
import { useAlerts } from "@/lib/queries"
import { cn } from "@/lib/utils"

export function AlertsPage({ onResolve }: { onResolve: (as: Alert[]) => void }) {
  const [show, setShow] = useState<"open" | "all">("open")
  const alerts = useAlerts(show === "open")
  const [checked, setChecked] = useState<Set<number>>(new Set())
  const lastClicked = useRef<number | null>(null)

  // Only open alerts can be selected; resolved ones drop out on the next refetch.
  const selectable = (alerts.data ?? []).filter((a) => !a.resolvedAt)
  const selected = selectable.filter((a) => checked.has(a.id))
  const allSelected = selectable.length > 0 && selected.length === selectable.length

  const toggle = (a: Alert, shift: boolean) => {
    const next = new Set(checked)
    const on = !checked.has(a.id)
    const from = selectable.findIndex((x) => x.id === lastClicked.current)
    const to = selectable.findIndex((x) => x.id === a.id)
    // Shift-click applies the same state to the whole range from the last clicked row.
    const range = shift && from !== -1 ? selectable.slice(Math.min(from, to), Math.max(from, to) + 1) : [a]
    for (const x of range) {
      if (on) next.add(x.id)
      else next.delete(x.id)
    }
    lastClicked.current = a.id
    setChecked(next)
  }
  const toggleAll = () => setChecked(allSelected ? new Set() : new Set(selectable.map((a) => a.id)))

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-muted-foreground text-sm">
          Personal Release rimosse da un client e non più presenti altrove.
        </p>
        <Tabs value={show} onValueChange={(v) => setShow(v as "open" | "all")} className="ml-auto">
          <TabsList>
            <TabsTrigger value="open">Da gestire</TabsTrigger>
            <TabsTrigger value="all">Storico</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      {selected.length > 0 && (
        <div className="bg-muted/60 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
          <span className="font-medium">
            {selected.length === 1 ? "1 selezionata" : `${selected.length} selezionate`}
          </span>
          <Button size="sm" className="ml-auto h-7" onClick={() => onResolve(selected)}>
            <TriangleAlert /> Indica posizione
          </Button>
          <Button size="sm" variant="ghost" className="h-7" onClick={() => setChecked(new Set())}>
            <X /> Deseleziona
          </Button>
        </div>
      )}
      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="w-8">
                <input
                  type="checkbox"
                  aria-label="Seleziona tutte"
                  className="accent-primary size-4 cursor-pointer align-middle disabled:cursor-default"
                  checked={allSelected}
                  ref={(el) => {
                    if (el) el.indeterminate = selected.length > 0 && !allSelected
                  }}
                  disabled={selectable.length === 0}
                  onChange={toggleAll}
                />
              </TableHead>
              <TableHead className="w-full">Torrent</TableHead>
              <TableHead>Evento</TableHead>
              <TableHead>Quando</TableHead>
              <TableHead>Esito</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {alerts.isLoading && (
              <TableRow>
                <TableCell colSpan={5}>
                  <Skeleton className="h-6 w-full" />
                </TableCell>
              </TableRow>
            )}
            {alerts.data?.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-muted-foreground py-10 text-center">
                  <CircleCheck className="mx-auto mb-2 size-6 text-emerald-500" />
                  Nessuna segnalazione {show === "open" ? "da gestire" : ""}.
                </TableCell>
              </TableRow>
            )}
            {alerts.data?.map((a) => (
              <TableRow
                key={a.id}
                data-state={checked.has(a.id) && !a.resolvedAt ? "selected" : undefined}
                className={cn(!a.resolvedAt && "bg-warning/10 hover:bg-warning/20 data-[state=selected]:bg-warning/30")}
              >
                <TableCell>
                  {!a.resolvedAt && (
                    <input
                      type="checkbox"
                      aria-label={`Seleziona ${a.torrentName}`}
                      className="accent-primary size-4 cursor-pointer align-middle"
                      checked={checked.has(a.id)}
                      onChange={() => {}}
                      onClick={(e) => toggle(a, e.shiftKey)}
                    />
                  )}
                </TableCell>
                <TableCell className="max-w-0 min-w-[240px]">
                  <div className="truncate font-medium" title={a.torrentName}>
                    {a.torrentName}
                  </div>
                  <div className="text-muted-foreground truncate font-mono text-xs">{a.hash}</div>
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-1.5">
                    {a.message}
                    <Badge variant="outline">{a.source === "webhook" ? "webhook" : "sync"}</Badge>
                  </div>
                </TableCell>
                <TableCell className="text-muted-foreground">{formatDate(a.createdAt)}</TableCell>
                <TableCell className="whitespace-normal">
                  {a.resolvedAt ? (
                    <div className="min-w-[200px] text-sm">
                      <span className="text-emerald-600 dark:text-emerald-400">✓ </span>
                      {a.resolution || "Gestita"}
                      <div className="text-muted-foreground text-xs">{formatDate(a.resolvedAt)}</div>
                    </div>
                  ) : (
                    <Button size="sm" variant="outline" className="h-7" onClick={() => onResolve([a])}>
                      <TriangleAlert className="text-amber-600" /> Indica posizione
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  )
}
