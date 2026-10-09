import { TriangleAlert } from "lucide-react"

import { Alert as AlertBox, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import type { Alert } from "@/lib/api"
import { formatRelative } from "@/lib/format"

export function AlertsBanner({
  alerts,
  onResolve,
  onShowAll,
}: {
  alerts: Alert[]
  onResolve: (a: Alert) => void
  onShowAll: () => void
}) {
  if (alerts.length === 0) return null
  const shown = alerts.slice(0, 3)
  return (
    <AlertBox variant="warning">
      <TriangleAlert />
      <AlertTitle>
        {alerts.length === 1
          ? "1 Personal Release è stata rimossa: dove l'hai spostata?"
          : `${alerts.length} Personal Release sono state rimosse: dove le hai spostate?`}
      </AlertTitle>
      <AlertDescription>
        <ul className="w-full space-y-1.5 pt-1">
          {shown.map((a) => (
            <li key={a.id} className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="text-foreground min-w-0 flex-1 truncate font-medium">{a.torrentName}</span>
              <span className="text-xs">
                {a.message} · {formatRelative(a.createdAt)}
              </span>
              <Button size="sm" variant="outline" className="h-7" onClick={() => onResolve(a)}>
                Indica posizione
              </Button>
            </li>
          ))}
        </ul>
        {alerts.length > shown.length && (
          <Button variant="link" className="h-auto p-0 text-current" onClick={onShowAll}>
            Vedi tutte ({alerts.length})
          </Button>
        )}
      </AlertDescription>
    </AlertBox>
  )
}
