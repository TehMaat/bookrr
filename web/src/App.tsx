import { useEffect, useState } from "react"
import { BellRing, HandHeart, HardDrive, ListTodo, ListTree, Puzzle, ScrollText, Server } from "lucide-react"

import { AdoptersPage } from "@/components/adopters-page"
import { AlertsBanner } from "@/components/alerts-banner"
import { AlertsPage } from "@/components/alerts-page"
import { ClientsPage } from "@/components/clients-page"
import { DisksPage } from "@/components/disks-page"
import { Header } from "@/components/header"
import { LogsPage } from "@/components/logs-page"
import { MissingPage, type MissingView } from "@/components/missing-page"
import { NavbarSlot } from "@/components/navbar-stats"
import { PlanPage } from "@/components/plan-page"
import { ResolveDialog } from "@/components/resolve-dialog"
import { Sidebar, type NavItem } from "@/components/sidebar"
import { TorrentsPage, torrentViews } from "@/components/torrents-page"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import type { Alert } from "@/lib/api"
import { useAlerts } from "@/lib/queries"

const tabs = ["torrents", "missing", "plan", "alerts", "disks", "adopters", "clients", "logs"] as const
type Tab = (typeof tabs)[number]

type Route = { tab: Tab; diskId: number | null; missingView: MissingView; torrentView: string }

/** The pages Releases and Dead of the earlier versions, now counters of the torrents. */
const oldTabs: Record<string, string> = { releases: "uploads", dead: "dead" }

/**
 * "#torrents" or "#torrents/dead" (the torrents with that counter chosen, or a filter of the earlier counters);
 * "#disks" or "#disks/3" (the torrents archived on disk 3, the target of its QR code);
 * "#missing" (the series), "#missing/sagas" or "#missing/lists".
 */
function routeFromHash(): Route {
  let [h, id] = window.location.hash.replace("#", "").split("/")
  if (h in oldTabs) {
    id = oldTabs[h]
    h = "torrents"
    window.history.replaceState(null, "", `#torrents/${id}`)
  }
  const tab = (tabs as readonly string[]).includes(h) ? (h as Tab) : "torrents"
  const diskId = tab === "disks" && /^\d+$/.test(id ?? "") ? Number(id) : null
  const missingView = tab === "missing" && (id === "sagas" || id === "lists") ? id : "series"
  const torrentView = tab === "torrents" && torrentViews.includes(id) ? id : "all"
  return { tab, diskId, missingView, torrentView }
}

export default function App() {
  const [{ tab, diskId, missingView, torrentView }, setRoute] = useState<Route>(routeFromHash)
  const [resolving, setResolving] = useState<Alert[]>([])
  const [menuOpen, setMenuOpen] = useState(false)
  const [slot, setSlot] = useState<HTMLDivElement | null>(null)
  const resolveOne = (a: Alert) => setResolving([a])
  const alerts = useAlerts(true)
  const openCount = alerts.data?.length ?? 0

  useEffect(() => {
    const onHash = () => setRoute(routeFromHash())
    window.addEventListener("hashchange", onHash)
    return () => window.removeEventListener("hashchange", onHash)
  }, [])

  const go = (t: string) => {
    window.location.hash = t
    setRoute(routeFromHash())
  }

  const nav: (NavItem & { id: Tab })[] = [
    { id: "torrents", label: "Torrents", icon: ListTree },
    { id: "missing", label: "Missing", icon: Puzzle },
    { id: "plan", label: "Plan", icon: ListTodo },
    { id: "alerts", label: "Alerts", icon: BellRing, count: openCount },
    { id: "disks", label: "Disks", icon: HardDrive },
    { id: "adopters", label: "Adopters", icon: HandHeart },
    { id: "clients", label: "Clients", icon: Server },
    { id: "logs", label: "Logs", icon: ScrollText },
  ]
  const current = nav.find((n) => n.id === tab)!
  const sidebar = (
    <Sidebar
      items={nav}
      active={tab}
      onSelect={(id) => {
        setMenuOpen(false)
        go(id)
      }}
    />
  )

  return (
    <div className="flex min-h-screen">
      <aside className="bg-sidebar sticky top-0 hidden h-screen w-56 shrink-0 border-r lg:block">{sidebar}</aside>
      <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
        <SheetContent side="left" className="bg-sidebar w-64 gap-0 sm:max-w-64">
          <SheetTitle className="sr-only">Menu</SheetTitle>
          {sidebar}
        </SheetContent>
      </Sheet>
      <div className="flex min-w-0 flex-1 flex-col">
        <Header
          slotRef={setSlot}
          onMenu={() => setMenuOpen(true)}
          title={
            <>
              <current.icon /> {current.label}
            </>
          }
        />
        <NavbarSlot.Provider value={slot}>
          <main className="w-full space-y-4 px-4 py-4 sm:px-6">
            <AlertsBanner alerts={alerts.data ?? []} onResolve={resolveOne} onShowAll={() => go("alerts")} />
            {tab === "torrents" && (
              <TorrentsPage
                key={torrentView}
                view={torrentView}
                onResolve={resolveOne}
                // Keeps the counter in the address without opening the page again.
                onFilter={(f) => window.history.replaceState(null, "", f === "all" ? "#torrents" : `#torrents/${f}`)}
              />
            )}
            {tab === "missing" && (
              <MissingPage view={missingView} onView={(v) => go(v === "series" ? "missing" : `missing/${v}`)} />
            )}
            {tab === "plan" && <PlanPage onResolve={resolveOne} />}
            {tab === "alerts" && <AlertsPage onResolve={setResolving} />}
            {tab === "disks" && (
              <DisksPage diskId={diskId} onOpenDisk={(id) => go(id === null ? "disks" : `disks/${id}`)} onResolve={resolveOne} />
            )}
            {tab === "adopters" && <AdoptersPage onResolve={resolveOne} />}
            {tab === "clients" && <ClientsPage />}
            {tab === "logs" && <LogsPage />}
          </main>
        </NavbarSlot.Provider>
      </div>
      <ResolveDialog alerts={resolving} onClose={() => setResolving([])} />
    </div>
  )
}
