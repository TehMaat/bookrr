import { useEffect, useState } from "react"
import { BellRing, CloudUpload, HandHeart, HardDrive, ListTree, Puzzle, Server } from "lucide-react"

import { AdoptersPage } from "@/components/adopters-page"
import { AlertsBanner } from "@/components/alerts-banner"
import { AlertsPage } from "@/components/alerts-page"
import { ClientsPage } from "@/components/clients-page"
import { DisksPage } from "@/components/disks-page"
import { Header } from "@/components/header"
import { MissingPage, type MissingView } from "@/components/missing-page"
import { NavbarSlot } from "@/components/navbar-stats"
import { ReleasesPage } from "@/components/releases-page"
import { ResolveDialog } from "@/components/resolve-dialog"
import { Sidebar, type NavItem } from "@/components/sidebar"
import { TorrentsPage } from "@/components/torrents-page"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import type { Alert } from "@/lib/api"
import { useAlerts } from "@/lib/queries"

const tabs = ["torrents", "releases", "missing", "alerts", "disks", "adopters", "clients"] as const
type Tab = (typeof tabs)[number]

type Route = { tab: Tab; diskId: number | null; missingView: MissingView }

/**
 * "#disks" or "#disks/3" (the torrents archived on disk 3, the target of its QR code);
 * "#missing" (the series) or "#missing/sagas".
 */
function routeFromHash(): Route {
  const [h, id] = window.location.hash.replace("#", "").split("/")
  const tab = (tabs as readonly string[]).includes(h) ? (h as Tab) : "torrents"
  const diskId = tab === "disks" && /^\d+$/.test(id ?? "") ? Number(id) : null
  const missingView = tab === "missing" && id === "sagas" ? "sagas" : "series"
  return { tab, diskId, missingView }
}

export default function App() {
  const [{ tab, diskId, missingView }, setRoute] = useState<Route>(routeFromHash)
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
    { id: "torrents", label: "Torrent", icon: ListTree },
    { id: "releases", label: "Release", icon: CloudUpload },
    { id: "missing", label: "Mancanti", icon: Puzzle },
    { id: "alerts", label: "Segnalazioni", icon: BellRing, count: openCount },
    { id: "disks", label: "Dischi", icon: HardDrive },
    { id: "adopters", label: "Adottatori", icon: HandHeart },
    { id: "clients", label: "Client", icon: Server },
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
          <main className="mx-auto w-full max-w-[1600px] space-y-4 px-4 py-4 sm:px-6">
            <AlertsBanner alerts={alerts.data ?? []} onResolve={resolveOne} onShowAll={() => go("alerts")} />
            {tab === "torrents" && <TorrentsPage onResolve={resolveOne} />}
            {tab === "releases" && <ReleasesPage onResolve={resolveOne} />}
            {tab === "missing" && (
              <MissingPage view={missingView} onView={(v) => go(v === "sagas" ? "missing/sagas" : "missing")} />
            )}
            {tab === "alerts" && <AlertsPage onResolve={setResolving} />}
            {tab === "disks" && (
              <DisksPage diskId={diskId} onOpenDisk={(id) => go(id === null ? "disks" : `disks/${id}`)} onResolve={resolveOne} />
            )}
            {tab === "adopters" && <AdoptersPage onResolve={resolveOne} />}
            {tab === "clients" && <ClientsPage />}
          </main>
        </NavbarSlot.Provider>
      </div>
      <ResolveDialog alerts={resolving} onClose={() => setResolving([])} />
    </div>
  )
}
