import type { Ref } from "react"
import { Menu } from "lucide-react"

import { Button } from "@/components/ui/button"

/** The bar on top of the page: the open page's counters (slotRef, see NavbarStats), or its title when it has none. */
export function Header({ title, slotRef, onMenu }: { title: React.ReactNode; slotRef: Ref<HTMLDivElement>; onMenu: () => void }) {
  return (
    <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
      <div className="flex h-16 items-center gap-3 px-4 sm:px-6">
        <Button variant="ghost" size="icon-sm" aria-label="Menu" className="lg:hidden" onClick={onMenu}>
          <Menu />
        </Button>
        <div ref={slotRef} className="peer flex min-w-0 flex-1 items-center gap-2 overflow-x-auto px-0.5 py-1 empty:hidden" />
        <div className="hidden flex-1 items-center gap-2 font-semibold peer-empty:flex [&_svg]:size-4">{title}</div>
      </div>
    </header>
  )
}
