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

export const diskKinds = ["HDD", "SSD", "NVMe", "USB", "NAS", "Tape", "Cloud", "Other"]

/** A disk you can hold in your hand, so it can carry a QR label. */
export const isPhysicalDisk = (kind: string) => kind !== "Cloud"

const scalewayRegions = [
  { id: "fr-par", label: "Paris (fr-par)" },
  { id: "nl-ams", label: "Amsterdam (nl-ams)" },
  { id: "pl-waw", label: "Warsaw (pl-waw)" },
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
    disk ? "Disk updated" : "Disk added"
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
      if (!r.ok) toast.error("Bucket not readable", { description: r.error })
      else if (r.objects === 0) toast.success("Bucket reachable", { description: "The folder is empty." })
      else toast.success("Bucket reachable", { description: `${r.objects}${r.more ? "+" : ""} files in the folder` })
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
            <DialogTitle>{disk ? "Edit disk" : "New disk"}</DialogTitle>
            <DialogDescription>A disk or other archive where you keep torrents offline.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-label">Name *</Label>
              <Input id="disk-label" value={form.label} onChange={set("label")} placeholder="Archive 01" required autoFocus />
            </div>
            <div className="grid gap-2">
              <Label>Kind</Label>
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
              <Label htmlFor="disk-serial">Serial number</Label>
              <Input id="disk-serial" value={form.serial} onChange={set("serial")} placeholder="WD-WCC4E1234567" className="font-mono" />
            </div>
            {form.kind === "Cloud" && (
              <div className="grid gap-3 rounded-lg border p-3 sm:col-span-2">
                <div className="flex items-center gap-2">
                  <Switch id="disk-s3" checked={linked} onCheckedChange={setLinked} />
                  <Label htmlFor="disk-s3" className="flex items-center gap-1.5">
                    <Cloud className="size-4" /> Link an S3 bucket (read-only)
                  </Label>
                </div>
                {linked && <BucketFields bucket={bucket} setBucket={setBucket} setB={setB} secret={secret} setSecret={setSecret} hasSecret={!!disk?.s3?.hasSecret} />}
                {linked && (
                  <Button type="button" variant="outline" size="sm" className="w-fit" onClick={test} disabled={testing || !bucket.bucket}>
                    {testing ? "Testing…" : "Test connection"}
                  </Button>
                )}
              </div>
            )}
            <div className="grid gap-2">
              <Label htmlFor="disk-model">Model</Label>
              <Input id="disk-model" value={form.model} onChange={set("model")} placeholder={form.kind === "Cloud" ? "Scaleway Object Storage" : "WD Red 4TB"} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="disk-capacity">Capacity</Label>
              <Input id="disk-capacity" value={form.capacity} onChange={set("capacity")} placeholder="4 TB" />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-place">Location</Label>
              <Input id="disk-place" value={form.place} onChange={set("place")} placeholder="Office drawer, NAS bay 2…" />
            </div>
            <div className="grid gap-2 sm:col-span-2">
              <Label htmlFor="disk-notes">Notes</Label>
              <Textarea id="disk-notes" value={form.notes} onChange={set("notes")} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={save.isPending}>
              Save
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
        <Label>Service</Label>
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
            <SelectItem value="other">Other S3 service</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-2">
        <Label htmlFor="s3-region">Region</Label>
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
        <Input id="s3-bucket" value={bucket.bucket} onChange={setB("bucket")} placeholder="my-releases" className="font-mono" required />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="s3-prefix">Folder</Label>
        <Input id="s3-prefix" value={bucket.prefix} onChange={setB("prefix")} placeholder="whole bucket" className="font-mono" />
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
          placeholder={hasSecret ? "•••••• (unchanged)" : ""}
          autoComplete="new-password"
          required={!hasSecret}
        />
      </div>
      <p className="text-muted-foreground text-xs sm:col-span-2">
        bookrr only reads the list of files: it doesn't download, write or delete anything in the bucket. Folders and files with
        the same name as a torrent are recorded as moved here.
        {scw && (
          <>
            {" "}
            Create an API key with the <b>ObjectStorageReadOnly</b> permission and the bucket's project as preferred project.
          </>
        )}
      </p>
    </div>
  )
}
