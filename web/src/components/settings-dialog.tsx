import { useEffect, useState } from "react"
import { Archive } from "lucide-react"

import { ConfigForm } from "@/components/config-form"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { api, type ConfigGroup, type Settings } from "@/lib/api"
import { useAction, useInfo, useSettings } from "@/lib/queries"

type ArchiveFields = Pick<Settings, "archiveMinSeeders" | "archiveMinCompleted" | "archiveDays">

const MAX_DAYS = 3650

const fields: { key: keyof ArchiveFields; label: string; hint: string; max?: number }[] = [
  { key: "archiveMinSeeders", label: "Altri seeder", hint: "Seeder del tracker meno i tuoi client che la seminano completa." },
  { key: "archiveMinCompleted", label: "Completati", hint: "Download completati sul tracker." },
  { key: "archiveDays", label: "Giorni di fila", hint: "Da quanto devono valere le due soglie; 0 = subito.", max: MAX_DAYS },
]

/** The groups of the options of the configuration, after the archive rule. */
const groups: { group: ConfigGroup; label: string; intro: string }[] = [
  { group: "general", label: "Generale", intro: "Le release e ogni quanto leggere client e bucket." },
  { group: "unit3d", label: "Tracker", intro: "Il tracker UNIT3D: hash dei torrent aggiunti senza, storico release, archiviabili." },
  { group: "missing", label: "Mancanti", intro: "TMDB e MDBList, per le stagioni, le saghe e le collezioni mancanti." },
  { group: "prowlarr", label: "Prowlarr", intro: "La risoluzione massima che esiste dei film caricati." },
]

const toForm = (s: ArchiveFields) => ({
  archiveMinSeeders: String(s.archiveMinSeeders),
  archiveMinCompleted: String(s.archiveMinCompleted),
  archiveDays: String(s.archiveDays),
})

/** A whole number from 0 to max, or null. */
function parse(v: string, max = Number.MAX_SAFE_INTEGER) {
  if (!/^\d+$/.test(v.trim())) return null
  const n = Number(v)
  return n <= max ? n : null
}

export function SettingsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const settings = useSettings()
  const info = useInfo()
  const [form, setForm] = useState<Record<keyof ArchiveFields, string>>(() => toForm(settings.data ?? defaults))
  useEffect(() => {
    if (open && settings.data) setForm(toForm(settings.data))
  }, [open, settings.data])

  const values = Object.fromEntries(fields.map((f) => [f.key, parse(form[f.key], f.max)])) as Record<
    keyof ArchiveFields,
    number | null
  >
  const valid = fields.every((f) => values[f.key] !== null)
  const changed = !!settings.data && fields.some((f) => values[f.key] !== settings.data[f.key])
  const save = useAction(() => api.saveSettings(values as ArchiveFields), "Impostazioni salvate")

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Impostazioni</DialogTitle>
          <DialogDescription>
            Valgono per tutta l'istanza di bookrr e si applicano subito, senza riavviare. Le opzioni impostate come variabili
            d'ambiente nel docker-compose hanno la precedenza e qui non si possono cambiare.
          </DialogDescription>
        </DialogHeader>
        <Tabs defaultValue="archive">
          <TabsList className="h-auto w-full flex-wrap">
            <TabsTrigger value="archive">Archiviabili</TabsTrigger>
            {groups.map((g) => (
              <TabsTrigger key={g.group} value={g.group}>
                {g.label}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="archive" className="pt-2">
            <form
              className="grid gap-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (valid && changed) save.mutate(undefined)
              }}
            >
              <section className="grid gap-3">
                <div className="flex items-center gap-2">
                  <Archive className="text-muted-foreground size-4" />
                  <h3 className="text-sm font-semibold">Release archiviabili</h3>
                </div>
                <p className="text-muted-foreground text-sm">
                  Una tua release ancora su un client è archiviabile quando altri la stanno già seminando: puoi toglierla dai client
                  e archiviarla. La trovi con il filtro "Archiviabili" nei Torrent e nelle Release.
                  {info.data && !info.data.unit3d && " Serve un tracker UNIT3D configurato."}
                </p>
                <div className="grid gap-3 sm:grid-cols-3">
                  {fields.map((f) => (
                    <div key={f.key} className="grid content-start gap-1.5">
                      <Label htmlFor={f.key}>{f.label} (min.)</Label>
                      <Input
                        id={f.key}
                        type="number"
                        inputMode="numeric"
                        min={0}
                        max={f.max}
                        step={1}
                        value={form[f.key]}
                        onChange={(e) => setForm((s) => ({ ...s, [f.key]: e.target.value }))}
                        aria-invalid={values[f.key] === null}
                        className="tabular-nums"
                      />
                      <p className="text-muted-foreground text-xs">{f.hint}</p>
                    </div>
                  ))}
                </div>
              </section>
              <div className="flex justify-end">
                <Button type="submit" disabled={!valid || !changed || save.isPending}>
                  Salva
                </Button>
              </div>
            </form>
          </TabsContent>
          {groups.map((g) => (
            <TabsContent key={g.group} value={g.group} className="grid gap-4 pt-2">
              <p className="text-muted-foreground text-sm">{g.intro}</p>
              <ConfigForm group={g.group} open={open} />
            </TabsContent>
          ))}
        </Tabs>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Chiudi
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

const defaults: ArchiveFields = { archiveMinSeeders: 1, archiveMinCompleted: 1, archiveDays: 0 }
