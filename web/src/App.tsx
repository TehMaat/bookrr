import { useEffect, useState } from "react"
import { BellRing, BookCheck, HandHeart, HardDrive, ListTree, Server } from "lucide-react"

import { AdoptersPage } from "@/components/adopters-page"
import { AlertsBanner } from "@/components/alerts-banner"
import { AlertsPage } from "@/components/alerts-page"
import { ClientsPage } from "@/components/clients-page"
import { DisksPage } from "@/components/disks-page"
import { Header } from "@/components/header"
import { ResolveDialog } from "@/components/resolve-dialog"
import { TorrentsPage } from "@/components/torrents-page"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Alert } from "@/lib/api"
import { useAlerts } from "@/lib/queries"

const tabs = ["torrents", "alerts", "disks", "adopters", "clients"] as const
type Tab = (typeof tabs)[number]

type Route = { tab: Tab; diskId: number | null }

/** "#disks" or "#disks/3" (the torrents archived on disk 3, the target of its QR code). */
function routeFromHash(): Route {
  const [h, id] = window.location.hash.replace("#", "").split("/")
  const tab = (tabs as readonly string[]).includes(h) ? (h as Tab) : "torrents"
  const diskId = tab === "disks" && /^\d+$/.test(id ?? "") ? Number(id) : null
  return { tab, diskId }
}

export default function App() {
  const [{ tab, diskId }, setRoute] = useState<Route>(routeFromHash)
  const [resolving, setResolving] = useState<Alert | null>(null)
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

  return (
    <div className="min-h-screen">
      <Header />
      <main className="mx-auto max-w-[1600px] space-y-4 px-4 py-4 sm:px-6">
        <AlertsBanner alerts={alerts.data ?? []} onResolve={setResolving} onShowAll={() => go("alerts")} />
        <Tabs value={tab} onValueChange={go}>
          <TabsList className="w-full justify-start overflow-x-auto sm:w-fit">
            <TabsTrigger value="torrents">
              <ListTree /> Torrent
            </TabsTrigger>
            <TabsTrigger value="alerts">
              <BellRing /> Segnalazioni
              {openCount > 0 && (
                <Badge variant="warning" className="ml-1 px-1.5">
                  {openCount}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="disks">
              <HardDrive /> Dischi
            </TabsTrigger>
            <TabsTrigger value="adopters">
              <HandHeart /> Adottatori
            </TabsTrigger>
            <TabsTrigger value="clients">
              <Server /> Client
            </TabsTrigger>
          </TabsList>
          <TabsContent value="torrents">
            <TorrentsPage onResolve={setResolving} />
          </TabsContent>
          <TabsContent value="alerts">
            <AlertsPage onResolve={setResolving} />
          </TabsContent>
          <TabsContent value="disks">
            <DisksPage diskId={diskId} onOpenDisk={(id) => go(id === null ? "disks" : `disks/${id}`)} onResolve={setResolving} />
          </TabsContent>
          <TabsContent value="adopters">
            <AdoptersPage onResolve={setResolving} />
          </TabsContent>
          <TabsContent value="clients">
            <ClientsPage />
          </TabsContent>
        </Tabs>
      </main>
      <ResolveDialog alert={resolving} onClose={() => setResolving(null)} />
      <footer className="text-muted-foreground flex items-center justify-center gap-1.5 py-6 text-xs">
        <BookCheck className="size-3.5" /> bookrr
      </footer>
    </div>
  )
}
