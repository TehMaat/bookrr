# bookrr

Inventario dei tuoi torrent: **dove sono** (su quale client qBittorrent, su quale disco d'archivio, chi li ha adottati) e **cosa è sparito** dai client.

- 📋 **Elenco unico** di tutti i torrent letti da uno o più client qBittorrent, con ricerca e filtri.
- 🖥️ **Su quale client** si trova ogni torrent (percorso, stato, ratio).
- 🧬 **Duplicati**: segnala lo stesso torrent (stesso hash) caricato su più client e su quali è ancora **in download**; quando è completato ovunque puoi **toglierlo da uno dei client** (con o senza i file) direttamente da bookrr.
- 💽 **Archivi offline**: dischi con nome, tipo, **numero di serie**, modello, capacità e posizione fisica; per ogni torrent registri lo **spostamento** su un disco/cartella.
- 🩺 **Dati SMART**: crea o aggiorna un disco incollando l'output di `smartctl` (testo o JSON) o di CrystalDiskInfo: modello, seriale, capacità, firmware, stato di salute e ore di accensione.
- 🤝 **Adottatori**: elenco delle persone che hanno adottato le tue release; ogni torrent può avere uno o più adottatori, scelti da un menu.
- 🔔 **Rimozioni delle Personal Release**: quando un torrent con tag `Personal Release` sparisce da tutti i client (rilevato dalla sincronizzazione o ricevuto via webhook) viene evidenziato e bookrr ti chiede **dove è stato spostato** (disco, adottato, eliminato, altro).
- ✍️ **Dati manuali**: aggiungi torrent che non sono su nessun client, note, adozioni e archivi.
- 📥 **Import degli archiviati** da CSV/Excel: ogni riga viene salvata esattamente come se l'avessi inserita con "Aggiungi".
- 🔎 **Hash automatico da UNIT3D**: i torrent aggiunti senza info hash vengono cercati per nome sul tuo tracker UNIT3D e ricevono l'hash vero.
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
| `BOOKRR_UNIT3D_URL` / `BOOKRR_UNIT3D_API_KEY` | — | Indirizzo del tracker UNIT3D (es. `https://tracker.example`) e la tua API key (Impostazioni → API key): servono a trovare l'hash dei torrent aggiunti senza. |
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
- i torrent senza tag e senza dati tuoi (spostamenti, adozioni, note) vengono semplicemente dimenticati quando spariscono.

### Spostamenti e adozioni

Registrare che un torrent è stato **spostato su un disco** o **adottato** da qualcuno risponde alla domanda "dove è finito?":

- se c'è una segnalazione aperta, viene chiusa con la destinazione indicata;
- se lo registri mentre il torrent è ancora sul client, quando lo togli dal client bookrr non apre una segnalazione (resta nello storico come già gestita).

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

### Dati SMART dei dischi

Nella pagina **Dischi**, *Importa da SMART* legge il rapporto SMART di un disco e crea il disco o aggiorna quello con lo stesso numero di serie (il pulsante sulla scheda di un disco aggiorna proprio quello). Formati accettati:

- `smartctl -a /dev/sdX` o `smartctl -x /dev/sdX` (SATA, NVMe, SAS);
- `smartctl -a -j /dev/sdX` (JSON);
- il testo copiato da CrystalDiskInfo (*Modifica → Copia*).

Vengono copiati modello, numero di serie, capacità, tipo (HDD/SSD/NVMe), firmware, stato SMART e ore di accensione, con la data della lettura; nome, posizione e note restano come li hai impostati, e un tipo come `USB` o `NAS` non viene sovrascritto.

Lo stesso si può fare da script, inviando il rapporto così com'è:

```bash
sudo smartctl -a -j /dev/sdb | curl -X POST --data-binary @- -H "Content-Type: text/plain" \
  "http://bookrr:8080/api/disks/smart?label=Archivio%2003"
```

| Parametro | Note |
|---|---|
| `id` | aggiorna il disco con questo id invece di cercarlo per numero di serie |
| `new=1` | crea sempre un disco nuovo |
| `label` | nome del disco se viene creato (default: il modello) |
| `dryRun=1` | mostra il risultato senza salvare |

Senza `id` né `new=1` un rapporto privo di numero di serie viene rifiutato.
### Import degli archiviati

In **Torrent → Importa** puoi caricare (o incollare) un file CSV con molti torrent già spostati su disco. Separatore `;`, `,` o tabulazione, prima riga con le intestazioni (c'è un modello da scaricare):

| Colonna | Note |
|---|---|
| `Nome` | obbligatoria |
| `Hash` | facoltativo, 40 o 64 caratteri esadecimali |
| `Dimensione` | `12,5 GB`, `1.5 TB`, byte… |
| `Tag` | separati da virgola |
| `Personal Release` | sì/no; se vuota vale l'interruttore nella finestra |
| `Note` | |
| `Disco` | nome o numero di serie di un disco già registrato; se vuota vale il disco scelto nella finestra |
| `Percorso`, `Note archivio` | serve almeno il disco o il percorso |
| `Adottato da` | nome di un adottatore già registrato |

Prima dell'import ogni riga viene verificata (anche dal server: hash già presente, ripetuto nel file, …). L'import è tutto-o-niente: se una riga non va bene non viene scritto nulla. Il form e l'import usano lo stesso codice (`POST /api/torrents` e `POST /api/torrents/import` con gli stessi campi), quindi i dati salvati sono identici.

### Hash automatico da UNIT3D

Se imposti `BOOKRR_UNIT3D_URL` e `BOOKRR_UNIT3D_API_KEY`, i torrent aggiunti a mano o importati **senza hash** vengono cercati in background sul tracker:

1. bookrr chiama `GET /api/torrents/filter?name=…` e sceglie il torrent con **lo stesso nome** (maiuscole, punti e underscore non contano); se più torrent hanno lo stesso nome usa la dimensione per scegliere, altrimenti non indovina;
2. l'hash viene letto da `info_hash` o dal magnet link quando il tracker li fornisce, altrimenti bookrr scarica il file `.torrent` (con il link di download della tua API) e lo calcola;
3. il torrent prende l'hash vero, con spostamenti, adozioni e note. Se bookrr conosce già quell'hash (per esempio perché il torrent è ancora su un client) i due vengono uniti in uno solo.

La ricerca parte subito dopo l'import e poi ogni ora. UNIT3D permette 30 richieste API al minuto, quindi bookrr ne fa al massimo una ogni 2,5 secondi (una o due per torrent, circa 12–24 torrent al minuto): un import grande viene completato in background mentre continui a usare bookrr. I torrent non trovati (nome diverso, più risultati) mostrano il motivo nella scheda del torrent e vengono ricercati dopo 24 ore; dalla scheda puoi anche lanciare subito la ricerca (`POST /api/torrents/{hash}/lookup-hash`).

> UNIT3D aggiunge il nome del tracker al file .torrent, quindi lo stesso contenuto ha un hash diverso su ogni tracker: bookrr usa quello del tracker configurato, che è lo stesso caricato nel tuo client.

## API

Tutto ciò che fa l'interfaccia è disponibile via REST (`/api/torrents`, `/api/disks`, `/api/adopters`, `/api/clients`, `/api/alerts`, `/api/sync`, …). Vedi `internal/api/server.go`.

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
internal/smart    lettura dei rapporti SMART (smartctl, CrystalDiskInfo)
internal/store    SQLite (modernc.org/sqlite, puro Go, niente CGO)
internal/syncer   sincronizzazione periodica
internal/unit3d   ricerca dell'hash sul tracker UNIT3D
web/              frontend React + Vite + Tailwind + shadcn/ui
```
