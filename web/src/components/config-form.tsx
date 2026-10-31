import { useEffect, useState } from "react"
import { Lock, PlugZap, X } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { api, type ConfigGroup, type ConfigOption, type ConfigService, type ConfigValues } from "@/lib/api"
import { useAction, useConfig } from "@/lib/queries"

/** The connections each group can try, with the label of their button. */
const tests: Record<ConfigGroup, { service: ConfigService; label: string }[]> = {
  general: [],
  unit3d: [{ service: "unit3d", label: "Prova il tracker" }],
  missing: [
    { service: "tmdb", label: "Prova TMDB" },
    { service: "mdblist", label: "Prova MDBList" },
  ],
  prowlarr: [{ service: "prowlarr", label: "Prova Prowlarr" }],
}

/** What the form holds for an option: the value typed and, for a secret, whether it is to be removed. */
type Field = { value: string; clear: boolean }

const toForm = (opts: ConfigOption[]) =>
  Object.fromEntries(opts.map((o) => [o.key, { value: o.secret ? "" : o.value, clear: false }])) as Record<string, Field>

/** The options changed: a secret only when retyped or removed. */
function changes(opts: ConfigOption[], form: Record<string, Field>): ConfigValues {
  const out: ConfigValues = {}
  for (const o of opts) {
    const f = form[o.key]
    if (!f || o.locked) continue
    if (o.secret) {
      if (f.clear) out[o.key] = ""
      else if (f.value.trim()) out[o.key] = f.value.trim()
    } else if (f.value.trim() !== o.value) out[o.key] = f.value.trim()
  }
  return out
}

/** The options of the configuration of a group, with their connection tests. */
export function ConfigForm({ group, open }: { group: ConfigGroup; open: boolean }) {
  const config = useConfig(open)
  const opts = (config.data?.options ?? []).filter((o) => o.group === group)
  const [form, setForm] = useState<Record<string, Field>>({})
  useEffect(() => {
    if (open && config.data) setForm(toForm(config.data.options.filter((o) => o.group === group)))
  }, [open, config.data, group])

  const values = changes(opts, form)
  const changed = Object.keys(values).length > 0
  const save = useAction(() => api.saveConfig(values), "Configurazione salvata e applicata")
  const [testing, setTesting] = useState<ConfigService | null>(null)
  const test = async (service: ConfigService) => {
    setTesting(service)
    try {
      const r = await api.testConfig(service, values)
      if (r.ok) toast.success(r.message ?? "Connessione riuscita")
      else toast.error("Connessione non riuscita", { description: r.error })
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setTesting(null)
    }
  }
  const set = (key: string, f: Partial<Field>) => setForm((s) => ({ ...s, [key]: { ...s[key], ...f } }))

  if (config.isLoading) return <Skeleton className="h-40 w-full" />
  if (config.isError) return <p className="text-destructive text-sm">Errore nel caricamento: {config.error.message}</p>

  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (changed) save.mutate(undefined)
      }}
    >
      {opts.map((o) => {
        const f = form[o.key] ?? { value: "", clear: false }
        return (
          <div key={o.key} className="grid gap-1.5">
            <div className="flex flex-wrap items-baseline gap-x-2">
              <Label htmlFor={o.key}>{o.label}</Label>
              <code className="text-muted-foreground text-[11px]">{o.key}</code>
            </div>
            <div className="flex items-center gap-2">
              <Input
                id={o.key}
                type={o.secret ? "password" : "text"}
                autoComplete="off"
                disabled={o.locked || f.clear}
                value={o.locked && !o.secret ? o.value : f.value}
                placeholder={
                  o.secret
                    ? o.locked
                      ? "impostata dall'ambiente"
                      : f.clear
                        ? "verrà rimossa"
                        : o.set
                          ? "impostata: lascia vuoto per non cambiarla"
                          : ""
                    : o.default
                }
                onChange={(e) => set(o.key, { value: e.target.value })}
              />
              {o.secret && o.set && !o.locked && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={f.clear ? "Non rimuovere" : "Rimuovi"}
                  title={f.clear ? "Non rimuovere" : "Rimuovi"}
                  onClick={() => set(o.key, { clear: !f.clear, value: "" })}
                >
                  <X />
                </Button>
              )}
            </div>
            <p className="text-muted-foreground text-xs">
              {o.locked && (
                <span className="text-foreground mr-1 inline-flex items-center gap-1">
                  <Lock className="size-3" /> Impostata dalla variabile d'ambiente: per cambiarla toglila dal docker-compose.
                </span>
              )}
              {o.help}
            </p>
          </div>
        )
      })}
      <div className="flex flex-wrap items-center gap-2">
        {tests[group].map((t) => (
          <Button key={t.service} type="button" variant="outline" disabled={testing !== null} onClick={() => test(t.service)}>
            <PlugZap /> {testing === t.service ? "Provo…" : t.label}
          </Button>
        ))}
        <Button type="submit" className="ml-auto" disabled={!changed || save.isPending}>
          {save.isPending ? "Applico…" : "Salva"}
        </Button>
      </div>
    </form>
  )
}
