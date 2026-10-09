import { useState } from "react"
import { Plus } from "lucide-react"

import { AdopterFormDialog } from "@/components/adopter-form-dialog"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useAdopters } from "@/lib/queries"

/** Dropdown of the known adopters, with a button to add a new one. */
export function AdopterSelect({
  value,
  onChange,
  exclude = [],
  id,
}: {
  value: number | null
  onChange: (id: number | null) => void
  /** Adopters that can't be picked, e.g. the ones that already adopted the torrent. */
  exclude?: number[]
  id?: string
}) {
  const adopters = useAdopters()
  const [creating, setCreating] = useState(false)
  const options = adopters.data?.filter((a) => !exclude.includes(a.id) || a.id === value) ?? []

  return (
    <div className="flex gap-2">
      <Select value={value ? String(value) : "none"} onValueChange={(v) => onChange(v === "none" ? null : Number(v))}>
        <SelectTrigger id={id} className="w-full min-w-0">
          <SelectValue placeholder="Scegli un adottatore" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="none">
            <span className="text-muted-foreground">Nessuno</span>
          </SelectItem>
          {options.map((a) => (
            <SelectItem key={a.id} value={String(a.id)}>
              {a.name}
              {a.contact && <span className="text-muted-foreground text-xs">{a.contact}</span>}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button type="button" variant="outline" size="icon" onClick={() => setCreating(true)} aria-label="Nuovo adottatore">
        <Plus />
      </Button>
      <AdopterFormDialog open={creating} onOpenChange={setCreating} onSaved={(a) => onChange(a.id)} />
    </div>
  )
}
