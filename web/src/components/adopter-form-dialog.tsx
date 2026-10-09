import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { api, type Adopter } from "@/lib/api"
import { useAction } from "@/lib/queries"

export function AdopterFormDialog({
  open,
  adopter,
  onOpenChange,
  onSaved,
}: {
  open: boolean
  adopter?: Adopter | null
  onOpenChange: (open: boolean) => void
  onSaved?: (a: Adopter) => void
}) {
  const [form, setForm] = useState({ name: "", contact: "", notes: "" })
  useEffect(() => {
    if (!open) return
    setForm({ name: adopter?.name ?? "", contact: adopter?.contact ?? "", notes: adopter?.notes ?? "" })
  }, [open, adopter])

  const save = useAction(
    (a: Partial<Adopter>) => (adopter ? api.updateAdopter(adopter.id, a) : api.createAdopter(a)),
    adopter ? "Adottatore aggiornato" : "Adottatore aggiunto"
  )
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))

  const submit = (e: React.FormEvent) => {
    // The dialog can be opened from inside another form (e.g. the resolve dialog).
    e.preventDefault()
    e.stopPropagation()
    save.mutate(form, {
      onSuccess: (a) => {
        onSaved?.(a)
        onOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{adopter ? "Modifica adottatore" : "Nuovo adottatore"}</DialogTitle>
            <DialogDescription>Una persona che ha adottato (e continua a condividere) le tue release.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="adopter-name">Nome *</Label>
              <Input id="adopter-name" value={form.name} onChange={set("name")} placeholder="Nome utente" required autoFocus />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="adopter-contact">Contatto</Label>
              <Input
                id="adopter-contact"
                value={form.contact}
                onChange={set("contact")}
                placeholder="Profilo sul tracker, Telegram, e-mail…"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="adopter-notes">Note</Label>
              <Textarea id="adopter-notes" value={form.notes} onChange={set("notes")} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Annulla
            </Button>
            <Button type="submit" disabled={!form.name.trim() || save.isPending}>
              Salva
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
