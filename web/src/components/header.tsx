import { useState, type Ref } from "react"
import { Check, Laptop, Menu, Moon, Palette, RefreshCw, Settings, Sun } from "lucide-react"
import { toast } from "sonner"

import { SettingsDialog } from "@/components/settings-dialog"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { accents, logoUrl } from "@/lib/accent"
import { api } from "@/lib/api"
import { formatRelative } from "@/lib/format"
import { useAction, useSyncState } from "@/lib/queries"
import { useTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

/**
 * The bar on top of the page: the open page's counters (slotRef, see NavbarStats),
 * or its title when it has none, then sync and preferences.
 */
export function Header({ title, slotRef, onMenu }: { title: React.ReactNode; slotRef: Ref<HTMLDivElement>; onMenu: () => void }) {
  const { setTheme, accent, setAccent } = useTheme()
  const sync = useSyncState()
  const syncNow = useAction(() => api.syncNow())
  const running = sync.data?.running || syncNow.isPending
  const [settingsOpen, setSettingsOpen] = useState(false)

  return (
    <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
      <div className="flex min-h-16 flex-wrap items-center gap-x-3 px-4 sm:px-6 md:flex-nowrap">
        <Button variant="ghost" size="icon-sm" aria-label="Menu" className="lg:hidden" onClick={onMenu}>
          <Menu />
        </Button>
        <div ref={slotRef} className="peer order-last -mx-1 flex min-w-0 basis-full items-center gap-2 overflow-x-auto px-1 pt-1 pb-2.5 empty:hidden md:order-none md:mx-0 md:flex-1 md:basis-auto md:px-0.5 md:py-1" />
        <div className="hidden h-16 flex-1 items-center gap-2 font-semibold peer-empty:flex [&_svg]:size-4">{title}</div>
        <div className="ml-auto flex h-16 shrink-0 items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-muted-foreground hidden text-xs xl:inline">
                Sync {formatRelative(sync.data?.lastRunAt)}
              </span>
            </TooltipTrigger>
            <TooltipContent>{sync.data?.lastResult || "Nessuna sincronizzazione ancora"}</TooltipContent>
          </Tooltip>
          <Button
            variant="outline"
            size="sm"
            disabled={running}
            onClick={() =>
              syncNow.mutate(undefined, { onSuccess: (r) => toast.success("Sincronizzazione completata", { description: r.result }) })
            }
          >
            <RefreshCw className={cn(running && "animate-spin")} />
            <span className="hidden sm:inline">Sincronizza</span>
          </Button>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Impostazioni" onClick={() => setSettingsOpen(true)}>
                <Settings />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Impostazioni</TooltipContent>
          </Tooltip>
          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon-sm" aria-label="Colore">
                    <Palette />
                  </Button>
                </DropdownMenuTrigger>
              </TooltipTrigger>
              <TooltipContent>Colore</TooltipContent>
            </Tooltip>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>Colore della spada</DropdownMenuLabel>
              {accents.map((a) => (
                <DropdownMenuItem key={a.id} onClick={() => setAccent(a.id)}>
                  <img src={logoUrl(a)} alt="" className="size-5" />
                  <span>{a.label}</span>
                  <span className="text-muted-foreground text-xs">{a.saber}</span>
                  {a.id === accent.id && <Check className="ml-auto" />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Tema">
                <Sun className="dark:hidden" />
                <Moon className="hidden dark:block" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setTheme("light")}>
                <Sun /> Chiaro
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setTheme("dark")}>
                <Moon /> Scuro
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setTheme("system")}>
                <Laptop /> Sistema
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </header>
  )
}
