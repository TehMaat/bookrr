import { useState } from "react"
import { Columns3 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

/** The columns hidden by the user, remembered in this browser under key. Every column is visible at first. */
export function useHiddenColumns<K extends string>(key: string) {
  const [hidden, setHidden] = useState<Set<K>>(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(key) ?? "[]")
      return new Set(Array.isArray(saved) ? (saved as K[]) : [])
    } catch {
      return new Set()
    }
  })
  const toggle = (col: K, visible: boolean) =>
    setHidden((h) => {
      const next = new Set(h)
      if (visible) next.delete(col)
      else next.add(col)
      try {
        localStorage.setItem(key, JSON.stringify([...next]))
      } catch {
        // Not remembered: the choice lasts until the page is reloaded.
      }
      return next
    })
  return { shows: (col: K) => !hidden.has(col), toggle }
}

/** The button that shows or hides the optional columns of a table. */
export function ColumnPicker<K extends string>({
  columns,
  shows,
  onToggle,
}: {
  columns: { id: K; label: string }[]
  shows: (col: K) => boolean
  onToggle: (col: K, visible: boolean) => void
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <Columns3 /> Columns
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Columns to show</DropdownMenuLabel>
        {columns.map((c) => (
          <DropdownMenuCheckboxItem
            key={c.id}
            checked={shows(c.id)}
            onCheckedChange={(on) => onToggle(c.id, on)}
            // Keeps the menu open to pick several columns.
            onSelect={(e) => e.preventDefault()}
          >
            {c.label}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
