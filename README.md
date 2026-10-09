# bookrr

Inventario dei tuoi torrent: **dove sono** (su quale client qBittorrent, su quale disco d'archivio, chi li ha adottati) e **cosa è sparito** dai client.

- 📋 **Elenco unico** di tutti i torrent letti da uno o più client qBittorrent, con ricerca e filtri.
- 🖥️ **Su quale client** si trova ogni torrent (percorso, stato, ratio).
- 🧬 **Duplicati**: segnala lo stesso torrent (stesso hash) caricato su più client.
- 💽 **Archivi offline**: dischi con nome, tipo, **numero di serie**, modello, capacità e posizione fisica; per ogni torrent puoi registrare su quale disco/cartella si trova.
- 🤝 **Adozioni**: chi ha adottato una tua release.
- 🔔 **Rimozioni delle Personal Release**: quando un torrent con tag `Personal Release` sparisce da tutti i client (rilevato dalla sincronizzazione o ricevuto via webhook) viene evidenziato e bookrr ti chiede **dove è stato spostato** (disco, adottato, eliminato, altro).
- ✍️ **Dati manuali**: aggiungi torrent che non sono su nessun client, note, adozioni e archivi.
- 🐳 Un solo container, ~9 MB, per **amd64, 386 (32 bit), arm64, armv7, armv6** (Raspberry Pi incluso).

Interfaccia web in italiano basata su [shadcn/ui](https://ui.shadcn.com), con tema chiaro/scuro.

## Installazione con Docker Compose

```bash
mkdir bookrr && cd bookrr
curl -O https://raw.githubusercontent.com/TehMaat/bookrr/main/docker-compose.yml
docker compose up -d
```

Apri `http://IP-DEL-SERVER:8080`, vai su **Client** e aggiungi i tuoi qBittorrent (URL della Web UI, utente e password). Il database SQLite viene salvato in `./data`.

L'immagine `ghcr.io/tehmaat/bookrr` è multi-architettura: Docker scarica automaticamente quella giusta per la tua macchina. In alternativa puoi compilarla in locale dal repository:

```bash
git clone https://github.com/TehMaat/bookrr.git && cd bookrr
docker compose up -d --build
```

> La compilazione locale richiede un'immagine `node` per la tua piattaforma: su macchine x86 a 32 bit usa l'immagine pubblicata oppure compila da un'altra macchina con `docker buildx build --platform linux/386`.

### Configurazione

Tutte le opzioni sono variabili d'ambiente (vedi `docker-compose.yml`):

| Variabile | Default | Descrizione |
|---|---|---|
| `BOOKRR_SYNC_INTERVAL` | `5m` | Ogni quanto leggere i client (minimo `30s`). |
| `BOOKRR_RELEASE_TAG` | `Personal Release` | Tag qBittorrent che identifica le tue release (maiuscole/minuscole indifferenti). |
| `BOOKRR_WEBHOOK_TOKEN` | — | Se impostato, il webhook richiede il token (`?token=…`, header `X-Bookrr-Token` o `Authorization: Bearer …`). |
| `BOOKRR_AUTH_USER` / `BOOKRR_AUTH_PASSWORD` | — | Se impostati, l'interfaccia e le API richiedono login (HTTP Basic). |
| `BOOKRR_LISTEN` | `:8080` | Indirizzo di ascolto. |
| `BOOKRR_DATA_DIR` | `/data` | Cartella del database. |
| `TZ` | — | Fuso orario dei log (es. `Europe/Rome`). |

> Le password dei client qBittorrent sono salvate in chiaro nel database in `./data`: proteggi quella cartella.

## Come funziona

### Sincronizzazione

A ogni intervallo bookrr legge `/api/v2/torrents/info` da ogni client attivo e confronta il risultato con la lettura precedente:

- i torrent presenti su più client vengono marcati come **duplicati**;
- se un torrent con il tag delle release sparisce da **tutti** i client, viene aperta una **segnalazione** ("Da localizzare") e la riga viene evidenziata;
- se poi ricompare su un client, la segnalazione si chiude da sola;
- i torrent senza tag e senza dati tuoi (archivi, adozione, note) vengono semplicemente dimenticati quando spariscono.

Un client irraggiungibile **non** genera segnalazioni: i suoi torrent restano com'erano fino alla prossima lettura riuscita.

### Webhook di rimozione

Per avere la segnalazione subito, senza aspettare la sincronizzazione, fai chiamare questo endpoint quando un torrent viene eliminato:

```
POST /api/webhook/qbit
```

Accetta JSON, form o query string. Campi riconosciuti:

| Campo | Obbligatorio | Note |
|---|---|---|
| `hash` | sì | info hash (anche `infohash`, `info_hash`, `hash_v1`) |
| `name` | | nome del torrent |
| `tags` | | lista separata da virgole o array JSON |
| `category`, `size` | | |
| `client` | | nome del client **come configurato in bookrr**: permette di sapere da quale client è stato tolto |

Esempi:

```bash
# form
curl -X POST "http://bookrr:8080/api/webhook/qbit?token=SEGRETO" \
  -d hash="%I" -d name="%N" -d tags="%G" -d category="%L" -d client="seedbox"

# JSON
curl -X POST http://bookrr:8080/api/webhook/qbit \
  -H "Content-Type: application/json" -H "X-Bookrr-Token: SEGRETO" \
  -d '{"hash":"0123…","name":"La.Mia.Release","tags":["Personal Release"],"client":"seedbox"}'
```

Viene aperta una segnalazione solo se il torrent ha il tag delle release (nel payload o già noto a bookrr) e non è presente su altri client. qBittorrent non offre un'opzione nativa "esegui programma alla rimozione": il webhook è pensato per script, qbit_manage, n8n, Home Assistant e simili. La sincronizzazione periodica rileva comunque tutte le rimozioni.

## API

Tutto ciò che fa l'interfaccia è disponibile via REST (`/api/torrents`, `/api/disks`, `/api/clients`, `/api/alerts`, `/api/sync`, …). Vedi `internal/api/server.go`.

## Sviluppo

Requisiti: Go 1.24+, Node 22+.

```bash
# backend (crea ./data/bookrr.db)
BOOKRR_DATA_DIR=./data BOOKRR_LISTEN=:8080 go run ./cmd/bookrr

# frontend con hot reload su http://localhost:5173 (proxy verso :8080)
cd web && npm install && npm run dev

# build completa: il frontend viene incorporato nel binario
cd web && npm run build && cd .. && go build -o bookrr ./cmd/bookrr

go test ./...
```

Struttura:

```
cmd/bookrr        entrypoint
internal/api      API REST, webhook, file statici
internal/qbit     client minimale per la Web API di qBittorrent
internal/store    SQLite (modernc.org/sqlite, puro Go, niente CGO)
internal/syncer   sincronizzazione periodica
web/              frontend React + Vite + Tailwind + shadcn/ui
```
