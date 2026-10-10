import { useEffect, useState } from "react"
import { Cloud } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { api, type Disk, type DiskInput, type S3Input } from "@/lib/api"
import { formatBytes, parseBytes } from "@/lib/format"
import { useAction, useScanDisk } from "@/lib/queries"

export const diskKinds = ["HDD", "SSD", "NVMe", "USB", "NAS", "Nastro", "Cloud", "Altro"]

/** A disk you can hold in your hand, so it can carry a QR label. */
export const isPhysicalDisk = (kind: string) => kind !== "Cloud"

const scalewayRegions = [
  { id: "fr-par", label: "Parigi (fr-par)" },
  { id: "nl-ams", label: "Amsterdam (nl-ams)" },
  { id: "pl-waw", label: "Varsavia (pl-waw)" },
]
const scalewayEndpoint = (region: string) => `https://s3.${region}.scw.cloud`

type Provider = "scaleway" | "other"
type BucketForm = { provider: Provider; endpoint: string; region: string; bucket: string; prefix: string; accessKey: string }

const emptyBucket: BucketForm = { provider: "scaleway", endpoint: "", region: "fr-par", bucket: "", prefix: "", accessKey: "" }

function bucketForm(d: Disk | null | undefined): BucketForm {
  const b = d?.s3
  if (!b) return emptyBucket
  const scw = scalewayRegions.some((r) => r.id === b.region && scalewayEndpoint(r.id) === b.endpoint)
  return { provider: scw ? "scaleway" : "other", endpoint: b.endpoint, region: b.region, bucket: b.bucket, prefix: b.prefix, accessKey: b.accessKey }
}

export function DiskFormDialog({
  open,
  disk,
  onOpenChange,
  onSaved,
}: {
  open: boolean
  disk?: Disk | null
  onOpenChange: (open: boolean) => void
  onSaved?: (d: Disk) => void
}) {
  const [form, setForm] = useState({ label: "", kind: "HDD", serial: "", model: "", capacity: "", place: "", notes: "" })
  const [linked, setLinked] = useState(false)
  const [bucket, setBucket] = useState<BucketForm>(emptyBucket)
  const [secret, setSecret] = useState("")
  const [testing, setTesting] = useState(false)
  useEffect(() => {
    if (!open) return
    setForm({
      label: disk?.label ?? "",
      kind: disk?.kind || "HDD",
      serial: disk?.serial ?? "",
      model: disk?.model ?? "",
      capacity: disk?.capacity ? formatBytes(disk.capacity) : "",
      place: disk?.place ?? "",
      notes: disk?.notes ?? "",
    })
    setLinked(!!disk?.s3)
    setBucket(bucketForm(disk))
    setSecret("")
  }, [open, disk])

  const save = useAction(
    (d: DiskInput) => (disk ? api.updateDisk(disk.id, d) : api.createDisk(d)),
    disk ? "Disco aggiornato" : "Disco aggiunto"
  )
  const scan = useScanDisk()
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))
  const setB = (k: keyof BucketForm) => (e: React.ChangeEvent<HTMLInputElement>) => setBucket((b) => ({ ...b, [k]: e.target.value }))

  const withBucket = form.kind === "Cloud" && linked
  const s3Payload = (): S3Input => ({
    endpoint: bucket.provider === "scaleway" ? scalewayEndpoint(bucket.region) : bucket.endpoint,
    region: bucket.region,
    bucket: bucket.bucket,
    prefix: bucket.prefix,
    accessKey: bucket.accessKey,
    // Empty secret while editing keeps the stored one.
    secretKey: disk?.s3 && secret === "" ? undefined : secret,
  })

  const test = async () => {
    setTesting(true)
    try {
      const r = await api.testS3({ ...s3Payload(), id: disk?.id })
      if (!r.ok) toast.error("Bucket non leggibile", { description: r.error })
      else if (r.objects === 0) toast.success("Bucket raggiungibile", { description: "La cartella indicata è vuota." })
      else toast.success("Bucket raggiungibile", { description: `${r.objects}${r.more ? "+" : ""} file nella cartella indicata` })
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setTesting(false)
    }
  }

  const submit = (e: React.FormEvent) => {
    // The dialog can be opened from inside another form (e.g. the resolve dialog).
    e.preventDefault()
    e.stopPropagation()
    save.mutate(
      { ...form, capacity: parseBytes(form.capacity), s3: withBucket ? s3Payload() : null },
      {
        onSuccess: (d) => {
          onSaved?.(d)
          onOpenChange(false)
          if (d.s3) scan.mutate(d.id)
        },
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{disk ? "Modifica disco" : "Nuovo disco"}</DialogTitle>
            <DialogDescription>Un disco o un altro archivio dove conservi i torrent offline.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-label">Nome *</Label>
              <Input id="disk-label" value={form.label} onChange={set("label")} placeholder="Archivio 01" required autoFocus />
            </div>
            <div className="grid gap-2">
              <Label>Tipo</Label>
              <Select value={form.kind} onValueChange={(v) => setForm((f) => ({ ...f, kind: v }))}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {diskKinds.map((k) => (
                    <SelectItem key={k} value={k}>
                      {k}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="disk-serial">Numero di serie</Label>
              <Input id="disk-serial" value={form.serial} onChange={set("serial")} placeholder="WD-WCC4E1234567" className="font-mono" />
            </div>
            {form.kind === "Cloud" && (
              <div className="grid gap-3 rounded-lg border p-3 sm:col-span-2">
                <div className="flex items-center gap-2">
                  <Switch id="disk-s3" checked={linked} onCheckedChange={setLinked} />
                  <Label htmlFor="disk-s3" className="flex items-center gap-1.5">
                    <Cloud className="size-4" /> Collega un bucket S3 (sola lettura)
                  </Label>
                </div>
                {linked && <BucketFields bucket={bucket} setBucket={setBucket} setB={setB} secret={secret} setSecret={setSecret} hasSecret={!!disk?.s3?.hasSecret} />}
                {linked && (
                  <Button type="button" variant="outline" size="sm" className="w-fit" onClick={test} disabled={testing || !bucket.bucket}>
                    {testing ? "Verifica…" : "Prova connessione"}
                  </Button>
                )}
              </div>
            )}
            <div className="grid gap-2">
              <Label htmlFor="disk-model">Modello</Label>
              <Input id="disk-model" value={form.model} onChange={set("model")} placeholder={form.kind === "Cloud" ? "Scaleway Object Storage" : "WD Red 4TB"} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="disk-capacity">Capacità</Label>
              <Input id="disk-capacity" value={form.capacity} onChange={set("capacity")} placeholder="4 TB" />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-place">Dove si trova</Label>
              <Input id="disk-place" value={form.place} onChange={set("place")} placeholder="Cassetto ufficio, slot 2 del NAS…" />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-notes">Note</Label>
              <Textarea id="disk-notes" value={form.notes} onChange={set("notes")} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Annulla
            </Button>
            <Button type="submit" disabled={save.isPending}>
              Salva
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function BucketFields({
  bucket,
  setBucket,
  setB,
  secret,
  setSecret,
  hasSecret,
}: {
  bucket: BucketForm
  setBucket: React.Dispatch<React.SetStateAction<BucketForm>>
  setB: (k: keyof BucketForm) => (e: React.ChangeEvent<HTMLInputElement>) => void
  secret: string
  setSecret: (s: string) => void
  hasSecret: boolean
}) {
  const scw = bucket.provider === "scaleway"
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="grid gap-2">
        <Label>Servizio</Label>
        <Select
          value={bucket.provider}
          onValueChange={(v) =>
            setBucket((b) =>
              v === "scaleway"
                ? { ...b, provider: "scaleway", region: scalewayRegions.some((r) => r.id === b.region) ? b.region : "fr-par" }
                : { ...b, provider: "other", endpoint: b.endpoint || scalewayEndpoint(b.region) }
            )
          }
        >
          <SelectTrigger className="w-full min-w-0">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="scaleway">Scaleway Object Storage</SelectItem>
            <SelectItem value="other">Altro servizio S3</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="s3-region">Regione</Label>
        {scw ? (
          <Select value={bucket.region} onValueChange={(v) => setBucket((b) => ({ ...b, region: v }))}>
            <SelectTrigger id="s3-region" className="w-full min-w-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {scalewayRegions.map((r) => (
                <SelectItem key={r.id} value={r.id}>
                  {r.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Input id="s3-region" value={bucket.region} onChange={setB("region")} placeholder="us-east-1" className="font-mono" />
        )}
      </div>
      {!scw && (
        <div className="grid gap-2 sm:col-span-2">
          <Label htmlFor="s3-endpoint">Endpoint *</Label>
          <Input id="s3-endpoint" value={bucket.endpoint} onChange={setB("endpoint")} placeholder="https://s3.example.com" className="font-mono" required />
        </div>
      )}
      <div className="grid gap-2">
        <Label htmlFor="s3-bucket">Bucket *</Label>
        <Input id="s3-bucket" value={bucket.bucket} onChange={setB("bucket")} placeholder="mie-release" className="font-mono" required />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="s3-prefix">Cartella</Label>
        <Input id="s3-prefix" value={bucket.prefix} onChange={setB("prefix")} placeholder="tutto il bucket" className="font-mono" />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="s3-access">Access key *</Label>
        <Input id="s3-access" value={bucket.accessKey} onChange={setB("accessKey")} placeholder="SCW…" className="font-mono" autoComplete="off" required />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="s3-secret">Secret key *</Label>
        <Input
          id="s3-secret"
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          placeholder={hasSecret ? "•••••• (invariata)" : ""}
          autoComplete="new-password"
          required={!hasSecret}
        />
      </div>
      <p className="text-muted-foreground text-xs sm:col-span-2">
        bookrr legge solo l'elenco dei file: non scarica, non scrive e non cancella nulla nel bucket. Le cartelle e i file con lo
        stesso nome di un torrent vengono registrati come spostati qui.
        {scw && (
          <>
            {" "}
            Crea una chiave API con il permesso <b>ObjectStorageReadOnly</b> e con il progetto del bucket come progetto preferito.
          </>
        )}
      </p>
    </div>
  )
}
