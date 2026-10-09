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
import { formatRelative } from "@/lib/format"
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
    client ? "Client aggiornato" : "Client aggiunto, sincronizzazione avviata"
  )

  const test = async () => {
    setTesting(true)
    try {
      const r = await api.testClient(payload())
      if (r.ok) toast.success(`Connesso a qBittorrent ${r.version}`, { description: `${r.torrents} torrent trovati` })
      else toast.error("Connessione fallita", { description: r.error })
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
            <DialogTitle>{client ? "Modifica client" : "Nuovo client qBittorrent"}</DialogTitle>
            <DialogDescription>Indirizzo della Web UI di qBittorrent.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="c-name">Nome *</Label>
            <Input id="c-name" value={form.name} onChange={set("name")} placeholder="seedbox" required autoFocus />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="c-url">URL *</Label>
            <Input id="c-url" value={form.url} onChange={set("url")} placeholder="http://192.168.1.10:8080" required />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="c-user">Utente</Label>
              <Input id="c-user" value={form.username} onChange={set("username")} autoComplete="off" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="c-pass">Password</Label>
              <Input
                id="c-pass"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={client?.hasPassword ? "•••••• (invariata)" : ""}
                autoComplete="new-password"
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-6">
            <div className="flex items-center gap-2">
              <Switch id="c-enabled" checked={form.enabled} onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))} />
              <Label htmlFor="c-enabled">Attivo</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="c-tls"
                checked={form.skipTlsVerify}
                onCheckedChange={(v) => setForm((f) => ({ ...f, skipTlsVerify: v }))}
              />
              <Label htmlFor="c-tls">Ignora certificato TLS</Label>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={test} disabled={testing || !form.url}>
              {testing ? "Verifica…" : "Prova connessione"}
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

function WebhookCard() {
  const info = useInfo()
  const origin = window.location.origin
  const token = info.data?.webhookTokenRequired ? "?token=IL_TUO_TOKEN" : ""
  const example = `curl -s -X POST "${origin}/api/webhook/qbit${token}" \\
  -d hash="%I" -d name="%N" -d tags="%G" -d category="%L" -d client="NOME_CLIENT"`
  return (
    <Card className="gap-3">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Webhook className="size-4" /> Webhook rimozione
        </CardTitle>
        <CardDescription>
          bookrr rileva da solo le rimozioni a ogni sincronizzazione (ogni {info.data?.syncInterval ?? "5m"}). Per una
          segnalazione immediata, fai chiamare questo endpoint quando elimini un torrent (script, qbit_manage, n8n…).
          Vengono segnalati solo i torrent con il tag <b>{info.data?.releaseTag ?? "Personal Release"}</b>.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <pre className="bg-muted overflow-x-auto rounded-md p-3 text-xs">{example}</pre>
        <p className="text-muted-foreground mt-2 text-xs">
          Accetta JSON, form o query string. Campi: <code>hash</code> (obbligatorio), <code>name</code>,{" "}
          <code>tags</code>, <code>category</code>, <code>size</code>, <code>client</code> (nome del client in bookrr).
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
  const del = useAction((id: number) => api.deleteClient(id), "Client rimosso")

  return (
    <div className="grid gap-4">
      <div className="flex items-center gap-2">
        <p className="text-muted-foreground text-sm">I client qBittorrent da cui bookrr legge i torrent.</p>
        <Button
          className="ml-auto"
          onClick={() => {
            setEditing(null)
            setOpen(true)
          }}
        >
          <Plus /> Nuovo client
        </Button>
      </div>

      {clients.isLoading && <Skeleton className="h-32 w-full" />}
      {clients.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-6 text-center text-sm">
            Nessun client configurato. Aggiungi il tuo primo qBittorrent.
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
                  <Badge variant="outline">Disattivato</Badge>
                ) : c.lastError ? (
                  <Badge variant="destructive">
                    <CircleX /> Errore
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
                  aria-label="Modifica"
                  onClick={() => {
                    setEditing(c)
                    setOpen(true)
                  }}
                >
                  <Pencil />
                </Button>
                <Button variant="ghost" size="icon-sm" aria-label="Elimina" onClick={() => setDeleting(c)}>
                  <Trash2 />
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent className="grid gap-1 text-sm">
              <div>
                <span className="font-medium">{c.torrentCount}</span> torrent
              </div>
              <div className="text-muted-foreground text-xs">Ultima sincronizzazione: {formatRelative(c.lastSyncAt)}</div>
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
            <AlertDialogTitle>Rimuovere il client "{deleting?.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              bookrr smetterà di leggerlo. I torrent rimossi in questo modo non generano segnalazioni; quelli senza
              altri dati (archivi, adozione, note) verranno dimenticati.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Annulla</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive hover:bg-destructive/90 text-white"
              onClick={() => deleting && del.mutate(deleting.id)}
            >
              Rimuovi
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
