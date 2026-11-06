import { useEffect, useState } from "react"
import { CircleCheck, CircleX, Pencil, Plus, Server, Trash2, Webhook } from "lucide-react"
import { toast } from "sonner"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { api, type Client, type ClientInput } from "@/lib/api"
import { formatRelative, plural } from "@/lib/format"
import { useAction, useClients, useInfo } from "@/lib/queries"

function ClientDialog({ open, client, onOpenChange }: { open: boolean; client: Client | null; onOpenChange: (o: boolean) => void }) {
  const [form, setForm] = useState<ClientInput>({ name: "", url: "", username: "", skipTlsVerify: false, enabled: true })
  const [password, setPassword] = useState("")
  const [testing, setTesting] = useState(false)

  useEffect(() => {
    if (!open) return
    setForm({
      name: client?.name ?? "",
      url: client?.url ?? "",
      username: client?.username ?? "",
      skipTlsVerify: client?.skipTlsVerify ?? false,
      enabled: client?.enabled ?? true,
    })
    setPassword("")
  }, [open, client])

  const payload = (): ClientInput => ({
    ...form,
    id: client?.id,
    // Empty password while editing keeps the stored one.
    password: client && password === "" ? undefined : password,
  })

  const save = useAction(
    (c: ClientInput) => (client ? api.updateClient(client.id, c) : api.createClient(c)),
    client ? "Client updated" : "Client added, sync started"
  )

  const test = async () => {
    setTesting(true)
    try {
      const r = await api.testClient(payload())
      if (r.ok) toast.success(`Connected to qBittorrent ${r.version}`, { description: `${plural(r.torrents ?? 0, "torrent")} found` })
      else toast.error("Connection failed", { description: r.error })
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setTesting(false)
    }
  }

  const set = (k: "name" | "url" | "username") => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate(payload(), { onSuccess: () => onOpenChange(false) })
          }}
        >
          <DialogHeader>
            <DialogTitle>{client ? "Edit client" : "New qBittorrent client"}</DialogTitle>
            <DialogDescription>Address of the qBittorrent Web UI.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="c-name">Name *</Label>
            <Input id="c-name" value={form.name} onChange={set("name")} placeholder="seedbox" required autoFocus />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="c-url">URL *</Label>
            <Input id="c-url" value={form.url} onChange={set("url")} placeholder="http://192.168.1.10:8080" required />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="c-user">Username</Label>
              <Input id="c-user" value={form.username} onChange={set("username")} autoComplete="off" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="c-pass">Password</Label>
              <Input
                id="c-pass"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={client?.hasPassword ? "•••••• (unchanged)" : ""}
                autoComplete="new-password"
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-6">
            <div className="flex items-center gap-2">
              <Switch id="c-enabled" checked={form.enabled} onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))} />
              <Label htmlFor="c-enabled">Enabled</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="c-tls"
                checked={form.skipTlsVerify}
                onCheckedChange={(v) => setForm((f) => ({ ...f, skipTlsVerify: v }))}
              />
              <Label htmlFor="c-tls">Skip TLS certificate check</Label>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={test} disabled={testing || !form.url}>
              {testing ? "Testing…" : "Test connection"}
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

function WebhookCard() {
  const info = useInfo()
  const origin = window.location.origin
  const token = info.data?.webhookTokenRequired ? "?token=YOUR_TOKEN" : ""
  const example = `curl -s -X POST "${origin}/api/webhook/qbit${token}" \\
  -d hash="%I" -d name="%N" -d tags="%G" -d category="%L" -d client="CLIENT_NAME"`
  return (
    <Card className="gap-3">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Webhook className="size-4" /> Removal webhook
        </CardTitle>
        <CardDescription>
          bookrr detects removals by itself on every sync (every {info.data?.syncInterval ?? "5m"}). For an immediate
          alert, have this endpoint called when you delete a torrent (script, qbit_manage, n8n…). Only torrents with
          the <b>{info.data?.releaseTag ?? "Personal Release"}</b> tag are reported.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <pre className="bg-muted overflow-x-auto rounded-md p-3 text-xs">{example}</pre>
        <p className="text-muted-foreground mt-2 text-xs">
          Accepts JSON, form or query string. Fields: <code>hash</code> (required), <code>name</code>,{" "}
          <code>tags</code>, <code>category</code>, <code>size</code>, <code>client</code> (client name in bookrr).
        </p>
      </CardContent>
    </Card>
  )
}

export function ClientsPage() {
  const clients = useClients()
  const [editing, setEditing] = useState<Client | null>(null)
  const [open, setOpen] = useState(false)
  const [deleting, setDeleting] = useState<Client | null>(null)
  const del = useAction((id: number) => api.deleteClient(id), "Client removed")

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4">
      <div className="flex items-center gap-2">
        <p className="text-muted-foreground text-sm">The qBittorrent clients bookrr reads torrents from.</p>
        <Button
          className="ml-auto"
          onClick={() => {
            setEditing(null)
            setOpen(true)
          }}
        >
          <Plus /> New client
        </Button>
      </div>

      {clients.isLoading && <Skeleton className="h-32 w-full" />}
      {clients.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-6 text-center text-sm">
            No clients configured. Add your first qBittorrent.
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {clients.data?.map((c) => (
          <Card key={c.id} className="gap-3">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Server className="size-4 text-sky-600" />
                {c.name}
                {!c.enabled ? (
                  <Badge variant="outline">Disabled</Badge>
                ) : c.lastError ? (
                  <Badge variant="destructive">
                    <CircleX /> Error
                  </Badge>
                ) : c.lastSyncAt ? (
                  <Badge variant="success">
                    <CircleCheck /> OK
                  </Badge>
                ) : null}
              </CardTitle>
              <CardDescription className="truncate font-mono text-xs">{c.url}</CardDescription>
              <CardAction className="flex gap-1">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Edit"
                  onClick={() => {
                    setEditing(c)
                    setOpen(true)
                  }}
                >
                  <Pencil />
                </Button>
                <Button variant="ghost" size="icon-sm" aria-label="Delete" onClick={() => setDeleting(c)}>
                  <Trash2 />
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent className="grid gap-1 text-sm">
              <div>
                <span className="font-medium">{c.torrentCount}</span> {c.torrentCount === 1 ? "torrent" : "torrents"}
              </div>
              <div className="text-muted-foreground text-xs">Last sync: {formatRelative(c.lastSyncAt)}</div>
              {c.lastError && <div className="text-destructive text-xs break-all">{c.lastError}</div>}
            </CardContent>
          </Card>
        ))}
      </div>

      <WebhookCard />

      <ClientDialog open={open} client={editing} onOpenChange={setOpen} />
      <AlertDialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove the client "{deleting?.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              bookrr will stop reading it. Torrents removed this way don't raise alerts; those without other data
              (archives, adoption, notes) will be forgotten.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => deleting && del.mutate(deleting.id)}
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
