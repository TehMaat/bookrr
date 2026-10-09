import { useState } from "react"
import { CircleCheck, TriangleAlert } from "lucide-react"

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

export function AlertsPage({ onResolve }: { onResolve: (a: Alert) => void }) {
  const [show, setShow] = useState<"open" | "all">("open")
  const alerts = useAlerts(show === "open")

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
      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="w-full">Torrent</TableHead>
              <TableHead>Evento</TableHead>
              <TableHead>Quando</TableHead>
              <TableHead>Esito</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {alerts.isLoading && (
              <TableRow>
                <TableCell colSpan={4}>
                  <Skeleton className="h-6 w-full" />
                </TableCell>
              </TableRow>
            )}
            {alerts.data?.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="text-muted-foreground py-10 text-center">
                  <CircleCheck className="mx-auto mb-2 size-6 text-emerald-500" />
                  Nessuna segnalazione {show === "open" ? "da gestire" : ""}.
                </TableCell>
              </TableRow>
            )}
            {alerts.data?.map((a) => (
              <TableRow key={a.id} className={cn(!a.resolvedAt && "bg-warning/10 hover:bg-warning/20")}>
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
                    <Button size="sm" variant="outline" className="h-7" onClick={() => onResolve(a)}>
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
