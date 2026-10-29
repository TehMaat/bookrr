<p align="center"><img src="web/public/favicon.svg" alt="Logo di bookrr" width="128"></p>

# bookrr

Inventario dei tuoi torrent: **dove sono** (su quale client qBittorrent, su quale disco d'archivio, chi li ha adottati) e **cosa è sparito** dai client.

- 📋 **Elenco unico** di tutti i torrent letti da uno o più client qBittorrent, con ricerca e **filtri su ogni colonna** (icona a imbuto accanto al titolo: testo, intervalli di dimensione, date e numeri, scelta di client/dischi/adottatori e stati); lo stesso nelle tabelle Release e Segnalazioni.
- 🖥️ **Su quale client** si trova ogni torrent (percorso, stato, ratio).
- 🧬 **Duplicati**: segnala lo stesso torrent (stesso hash) caricato su più client e su quali è ancora **in download**; quando è completato ovunque puoi **toglierlo da uno dei client** (con o senza i file) direttamente da bookrr.
- 💽 **Archivi offline**: dischi con nome, tipo, **numero di serie**, modello, capacità e posizione fisica; per ogni torrent registri lo **spostamento** su un disco/cartella.
- ☁️ **Archivi su cloud S3** (Scaleway e altri servizi compatibili), collegati in **sola lettura**: bookrr legge l'elenco dei file del bucket e riconosce da solo quali torrent ci hai spostato.
- 📱 **QR code sui dischi**: ogni disco fisico ha la sua pagina con i torrent archiviati sopra; stampa il QR code da attaccare al disco e inquadrandolo con il telefono apri quella pagina. L'indirizzo di bookrr nella tua rete (es. `http://192.168.1.10:8080`, utile se il container è in una rete Docker) si imposta dalla finestra del QR code.
- 🩺 **Dati SMART**: crea o aggiorna un disco incollando l'output di `smartctl` (testo o JSON) o di CrystalDiskInfo: modello, seriale, capacità, firmware, stato di salute e ore di accensione.
- 🤝 **Adottatori**: elenco delle persone che hanno adottato le tue release; ogni torrent può avere uno o più adottatori, scelti da un menu.
- 🔔 **Rimozioni delle Personal Release**: quando un torrent con tag `Personal Release` sparisce da tutti i client (rilevato dalla sincronizzazione o ricevuto via webhook) viene evidenziato e bookrr ti chiede **dove è stato spostato** (disco, adottato, eliminato, altro).
- ✍️ **Dati manuali**: aggiungi torrent che non sono su nessun client, note, adozioni e archivi.
- 📥 **Import degli archiviati** da CSV/Excel: ogni riga viene salvata esattamente come se l'avessi inserita con "Aggiungi".
- 🔎 **Hash automatico da UNIT3D**: i torrent aggiunti senza info hash vengono cercati per nome sul tuo tracker UNIT3D e ricevono l'hash vero.
- 🚀 **Storico release**: tutti i torrent che hai caricato sul tracker UNIT3D (anche anonimi), con seeder, leecher e completati, collegati al torrent di bookrr corrispondente e al suo stato (su client, archiviato, adottato); le release che bookrr non conosce si aggiungono con un clic, e le tue Personal Release sparite dal tracker vengono evidenziate (e puoi togliere loro il flag, anche su qBittorrent).
- 🚀 **Storico release**: tutti i torrent che hai caricato sul tracker UNIT3D (anche anonimi), con seeder, leecher e completati, collegati al torrent di bookrr corrispondente e al suo stato (su client, archiviato, adottato); le release che bookrr non conosce si aggiungono con un clic, e le tue Personal Release sparite dal tracker vengono evidenziate.
- 🧩 **Stagioni mancanti**: per ogni serie TV di cui hai caricato almeno un season pack, una griglia delle stagioni per ognuna delle tue qualità (tipo, risoluzione, HDR, lingue audio): quali sono tue, quali ha già caricato qualcun altro nella stessa qualità, quali esistono solo in un'altra qualità e quali mancano del tutto dal tracker. Lo stesso per le **saghe di film**: per ogni film caricato che fa parte di una saga, gli altri film della saga nella stessa qualità. E per le **collezioni** che segui (liste di film su MDBList: l'MCU, i film Pixar, i classici Disney), anche se non ne hai caricato nessuno. Vista **per stagione / per film** con la versione migliore di ognuna contro una **qualità minima** (es. «almeno Remux»), saghe e qualità **nascondibili**, saghe **segnate complete**, release di altri **accettate**, parti **ignorate** e **gruppi bannati** (le loro release non bastano: la parte resta mancante, in rosa e con meno priorità). Controllo ogni notte, stagioni e saghe da TMDB.
- 📺 **Risoluzione massima dei film**: con [Prowlarr](https://prowlarr.com), per ogni film che hai caricato bookrr cerca su tutti i tuoi indexer la risoluzione migliore che esiste (in qualsiasi lingua) e ti segnala i film che esistono in 2160p mentre tu li hai in 1080p, da rifare con un mux.
- 📦 **Archiviabili**: le tue release ancora su un client che altri stanno già seminando (soglie di seeder, completati e giorni scelte nelle impostazioni), da togliere dai client e archiviare.
- 🐳 Un solo container, ~9 MB, per **amd64, 386 (32 bit), arm64, armv7, armv6** (Raspberry Pi incluso).

Interfaccia web in italiano basata su [shadcn/ui](https://ui.shadcn.com), con tema chiaro/scuro e **colore a scelta** (icona 🎨 in alto a destra): il logo, l'icona della scheda e il colore dei pulsanti seguono il colore della spada laser che scegli. La scelta viene ricordata dal browser.

<p align="center">
  <img src="docs/logo/bookrr-viola.svg" alt="Viola" width="56">
  <img src="docs/logo/bookrr-blu.svg" alt="Blu" width="56">
  <img src="docs/logo/bookrr-verde.svg" alt="Verde" width="56">
  <img src="docs/logo/bookrr-giallo.svg" alt="Giallo" width="56">
  <img src="docs/logo/bookrr-rosso.svg" alt="Rosso" width="56">
  <img src="docs/logo/bookrr-darksaber.svg" alt="Nero" width="56">
</p>

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

### Versioni

La versione in uso è mostrata accanto al nome, in alto. Ogni versione è pubblicata anche come immagine: `ghcr.io/tehmaat/bookrr:0.8.0` blocca una versione precisa, `:0.8` riceve solo le correzioni della 0.8, `:latest` segue sempre `main`.

### Configurazione

Tutte le opzioni sono variabili d'ambiente (vedi `docker-compose.yml`):

| Variabile | Default | Descrizione |
|---|---|---|
| `BOOKRR_SYNC_INTERVAL` | `5m` | Ogni quanto leggere i client (minimo `30s`). |
| `BOOKRR_RELEASE_TAG` | `Personal Release` | Tag qBittorrent che identifica le tue release (maiuscole/minuscole indifferenti). |
| `BOOKRR_WEBHOOK_TOKEN` | — | Se impostato, il webhook richiede il token (`?token=…`, header `X-Bookrr-Token` o `Authorization: Bearer …`). |
| `BOOKRR_AUTH_USER` / `BOOKRR_AUTH_PASSWORD` | — | Se impostati, l'interfaccia e le API richiedono login (HTTP Basic). |
| `BOOKRR_UNIT3D_URL` / `BOOKRR_UNIT3D_API_KEY` | — | Indirizzo del tracker UNIT3D (es. `https://tracker.example`) e la tua API key (Impostazioni → API key): servono a trovare l'hash dei torrent aggiunti senza e per lo storico delle tue release. |
| `BOOKRR_UNIT3D_RELEASES_INTERVAL` | `6h` | Ogni quanto rileggere i tuoi upload sul tracker per lo storico release (minimo `15m`). |
| `BOOKRR_TMDB_API_KEY` | — | Chiave API (v3) o token di lettura (v4) di [TMDB](https://www.themoviedb.org/settings/api): con il tracker UNIT3D attiva il controllo delle **stagioni mancanti** e dei **film mancanti delle saghe**. |
| `BOOKRR_MDBLIST_API_KEY` | — | Chiave API di [MDBList](https://mdblist.com) (gratuita, nelle preferenze del tuo account): con il tracker e TMDB attiva le **collezioni** (liste di film da seguire oltre le saghe). |
| `BOOKRR_PROWLARR_URL` / `BOOKRR_PROWLARR_API_KEY` | — | Indirizzo di [Prowlarr](https://prowlarr.com) (es. `http://prowlarr:9696`) e la sua API key (Settings → General): con il tracker UNIT3D attiva la **risoluzione massima** dei film caricati. |
| `BOOKRR_PROWLARR_RECHECK` | `7d` | Ogni quanto cercare di nuovo ogni film su Prowlarr (minimo `1d`; anche in ore, es. `36h`). |
| `BOOKRR_MISSING_CHECK_AT` | `04:00` | Ora del controllo notturno delle stagioni e dei film mancanti (ora locale, `TZ`). |
| `BOOKRR_S3_SCAN_INTERVAL` | `1h` | Ogni quanto leggere i bucket S3 degli archivi cloud (minimo `1m`). |
| `BOOKRR_LISTEN` | `:8080` | Indirizzo di ascolto. |
| `BOOKRR_DATA_DIR` | `/data` | Cartella del database. |
| `TZ` | — | Fuso orario dei log (es. `Europe/Rome`). |

> Le password dei client qBittorrent e le chiavi dei bucket S3 sono salvate in chiaro nel database in `./data`: proteggi quella cartella.

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
- nella pagina **Segnalazioni** puoi selezionarne più di una (anche a intervalli, con Maiusc+clic) e indicare per tutte la stessa destinazione in un colpo solo;
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

### Archivi su cloud S3 (Scaleway)

Un archivio di tipo **Cloud** può essere collegato a un bucket S3: in **Dischi → Nuovo disco** scegli il tipo *Cloud*, attiva *Collega un bucket S3* e indica:

| Campo | Note |
|---|---|
| Servizio | *Scaleway Object Storage* (scegli la regione: `fr-par`, `nl-ams`, `pl-waw`) o *Altro servizio S3* con endpoint e regione (AWS, MinIO, Backblaze B2, Cloudflare R2, Wasabi, …) |
| Bucket | solo il nome, es. `mie-release` |
| Cartella | facoltativa: legge solo questa cartella del bucket (es. `torrent/`) |
| Access key / Secret key | la chiave API; la secret key non viene mai mostrata di nuovo |

Il collegamento è in **sola lettura**: bookrr usa soltanto l'elenco dei file (`ListObjectsV2`), non scarica, non scrive e non cancella nulla. Per sicurezza usa comunque una chiave che possa solo leggere: su Scaleway crea un'applicazione IAM con il permesso **ObjectStorageReadOnly** sul progetto del bucket e genera la chiave API con quel progetto come *progetto preferito*. *Prova connessione* verifica la chiave leggendo la prima pagina del bucket.

Dopo il salvataggio, ogni ora (`BOOKRR_S3_SCAN_INTERVAL`) o con il pulsante *Leggi il bucket*:

- ogni **cartella o file con lo stesso nome di un torrent** (il nome che gli dà qBittorrent), a qualunque profondità (`Film/Nome.Release/` come `Nome.Release/`), viene registrato come **spostato sull'archivio**, con il percorso `s3://bucket/…`; se il torrent aveva una segnalazione aperta, viene chiusa;
- se ci sposti una release prima di toglierla dal client, la rimozione non apre segnalazioni (come per gli spostamenti registrati a mano);
- se una release sparisce dal bucket e non è su nessun client, su nessun altro disco e non è adottata, viene aperta una segnalazione ("Rimosso da *nome archivio*");
- ciò che non corrisponde a nessun torrent compare nella pagina dell'archivio, in *Non riconosciuti nel bucket*: con *Aggiungi* diventa un torrent (con il nome del bucket) archiviato lì, e se è configurato UNIT3D bookrr ne cerca l'hash sul tracker.

Gli spostamenti trovati nel bucket si aggiornano da soli e non si cancellano a mano; se registri a mano lo stesso torrent sullo stesso archivio vale la tua registrazione. Se cambi bucket, endpoint o cartella, ciò che era stato trovato nel vecchio bucket viene dimenticato senza segnalazioni. Un bucket non raggiungibile non cambia nulla: l'errore è mostrato sulla scheda dell'archivio.

Via API: `POST /api/disks/{id}/scan` legge subito il bucket, `GET /api/disks/{id}/bucket-entries` restituisce gli elementi non riconosciuti.

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
2. se il nome è scritto diversamente (`Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1…DV.HDR10.H.265-MaTiTa` contro `Criminal Minds S19 2160p UHD DSNP WEB-DL DD+ 5.1 … DV HDR10+ H.265-MaTiTa`) bookrr confronta le caratteristiche della release, scritte in qualunque forma (`DDP5.1` = `DD+ 5.1`, `HDR10` = `HDR10+`, `H.265` = `x265` = `HEVC`): titolo, anno, stagione/episodio, risoluzione, sorgente (REMUX, WEB-DL, …), servizio (DSNP, NF, …), HDR/DV, codec video e **gruppo** dopo il `-` devono coincidere; l'audio e le lingue del nome del file devono comparire nel titolo del tracker. Ciò che c'è in uno solo dei due nomi non conta, tranne HDR e stagione. Se più torrent corrispondono (o la dimensione, se nota, differisce più del 5%) non indovina;
3. bookrr scarica il file `.torrent` (con il link di download della tua API), ne calcola l'hash e lo **salva**: lo scarichi dalla scheda del torrent, nella sezione *Spostato su disco*. Se il download non riesce usa l'hash che alcune versioni di UNIT3D forniscono (`info_hash` o magnet link);
4. il torrent prende l'hash vero, con spostamenti, adozioni e note, e il link alla sua pagina sul tracker. Se bookrr conosce già quell'hash (per esempio perché il torrent è ancora su un client) i due vengono uniti in uno solo.

La ricerca parte subito dopo l'import e poi ogni ora. UNIT3D permette 30 richieste API al minuto, quindi bookrr ne fa al massimo una ogni 2,5 secondi (una o due per torrent, circa 12–24 torrent al minuto): un import grande viene completato in background mentre continui a usare bookrr. I torrent non trovati mostrano il motivo nella scheda del torrent e vengono ricercati dopo 24 ore. Dalla scheda puoi:

- **Cerca ora sul tracker**: ripete subito la ricerca automatica (`POST /api/torrents/{hash}/lookup-hash`);
- **Scegli dal tracker**: mostra i torrent simili, ordinati per parole in comune, con dimensione, data e link alla pagina; puoi cambiare la ricerca (es. solo `Ad Astra 2019`) e con *Usa questo* il torrent prende hash e file `.torrent` di quello scelto (`GET /api/torrents/{hash}/tracker-candidates?q=…`, `POST /api/torrents/{hash}/tracker-match` con `{"id": "123"}`).

Il file `.torrent` salvato si scarica anche da `GET /api/torrents/{hash}/torrent-file`.

I **tag** dei torrent che non sono su un client si cambiano dalla scheda, in *Dati personali*; per quelli su un client fanno fede i tag di qBittorrent.

> UNIT3D aggiunge il nome del tracker al file .torrent, quindi lo stesso contenuto ha un hash diverso su ogni tracker: bookrr usa quello del tracker configurato, che è lo stesso caricato nel tuo client.

### Storico release

Con il tracker UNIT3D configurato, la scheda **Release** mostra **tutti i torrent che hai caricato** (non solo quelli con il flag Personal Release del tracker, e anche quelli anonimi), dal più recente: nome, tipo (WEB-DL, Remux, Encode…), risoluzione, data di caricamento, seeder, leecher e completati, link alla pagina sul tracker e lo **stato** del torrent di bookrr collegato: *su client*, *da localizzare*, *archiviato*, *adottato*, *posizione ignota*, oppure **sconosciuta** se bookrr non conosce nessun torrent con quell'hash. Puoi cercare, filtrare per stato e filtrare ogni colonna (nome, tipo e risoluzione, data di caricamento, seeder e completati, stato); cliccando una release collegata si apre la scheda del torrent.

Come funziona:

1. bookrr chiede al tracker il tuo nome utente (`GET /api/user`) e poi i tuoi upload (`GET /api/torrents/filter?uploader=…`, seguendo tutte le pagine). Il tracker tiene in cache queste risposte per qualche minuto, condivise tra gli utenti: bookrr aggiunge alla richiesta un parametro con il tuo nome (ignorato dal tracker) per non ricevere la risposta preparata per qualcun altro, che non conterrebbe i tuoi upload anonimi;
2. l'API non fornisce l'info hash: bookrr scarica il file `.torrent` di ogni release **una sola volta** (l'hash non cambia), ne calcola l'hash e lo conserva. Il primo giro con molti upload richiede qualche minuto (al massimo una richiesta ogni 2,5 secondi, condivise con la ricerca degli hash);
3. ogni release è collegata al torrent di bookrr con **lo stesso hash**, che riceve anche il file `.torrent`, il link alla pagina sul tracker e, se mancava, la dimensione. I torrent aggiunti **senza hash** vengono confrontati per nome con le tue release (stesse regole della ricerca automatica, vedi sopra) e, se ne corrisponde una sola, prendono il suo hash;
4. a ogni giro seeder, leecher e completati vengono aggiornati; le release che non compaiono più tra i tuoi upload restano nello storico, segnate come **rimosse**.

L'aggiornamento parte all'avvio, poi ogni `BOOKRR_UNIT3D_RELEASES_INTERVAL` (6 ore) e con **Aggiorna ora**. Le release **sconosciute** non vengono aggiunte da sole: con **Aggiungi a bookrr** (o **Aggiungi tutte**) diventano torrent con hash, nome, dimensione, file `.torrent` e flag Personal Release, e dalla loro scheda indichi dove si trovano.

In cima alla pagina sono evidenziate le **Personal Release di bookrr che non risultano tra i tuoi upload**: cancellate o sostituite sul tracker, caricate su un altro tracker o aggiunte senza hash (i torrent su un client che annunciano a un altro dominio non vengono elencati).

Se non sono tue release, **togli il flag** a una (icona accanto al nome) o a tutte (**Togli il flag a tutte**): bookrr toglie il tag `Personal Release` anche dai torrent sui client qBittorrent (solo il tag: torrent e file restano dove sono) e non le segnala più quando spariscono. Se un client non risponde, i torrent che ha restano Personal Release e puoi riprovare.

API: `GET /api/releases`, `POST /api/releases/sync` (aggiorna ora), `POST /api/releases/add` con `{"ids": [123]}` o `{"all": true}`, `POST /api/releases/unflag` con `{"hashes": ["…"]}` o `{"all": true}` (toglie il flag).

### Risoluzione massima dei film

Con `BOOKRR_PROWLARR_URL` e `BOOKRR_PROWLARR_API_KEY` (e il tracker UNIT3D, da cui vengono i film), la scheda **Release** ha la colonna **Max**: la risoluzione migliore che esiste di ogni film che hai caricato, **in qualsiasi lingua** (l'audio italiano lo aggiungi tu con un mux). Mostra:

- **↑ 2160p** (in giallo): il film esiste in una risoluzione migliore della tua migliore release di quel film; il tooltip dice quale release e su quale indexer, e cliccando si apre la sua pagina. La casella **Migliorabili** in alto filtra questi film;
- **✓**: hai già la risoluzione migliore che esiste (i film che hai già in 2160p non vengono cercati);
- **Non trovata**: Prowlarr non ha trovato release del film con una risoluzione nota;
- **—**: non è un film (le serie non vengono cercate) o non è ancora stato cercato.

Come funziona:

1. una volta al giorno (la prima 15 minuti dopo l'avvio) bookrr prende i tuoi upload sul tracker che sono **film** con un id TMDB o IMDb, tranne quelli che hai già in 2160p, e cerca quelli mai cercati o cercati più di `BOOKRR_PROWLARR_RECHECK` fa (7 giorni);
2. per ogni film chiede a Prowlarr `GET /api/v1/search?type=movie&query={ImdbId:tt…}{TmdbId:…}` nella categoria Film (2000): Prowlarr lo cerca su **tutti i tuoi indexer**. Le ricerche sono lente apposta: una alla volta, con 30 secondi di pausa tra una e l'altra, per non pesare sui tracker (i limiti di richieste che hai impostato in Prowlarr per ogni indexer valgono comunque);
3. dei risultati contano solo quelli del film: con lo stesso id TMDB o IMDb oppure, se l'indexer non li indica, con lo stesso titolo e anno (±1) di una tua release. Gli **upscale** non contano. La risoluzione è letta dal nome della release; vince la più alta, a parità la più seminata.

Se Prowlarr non risponde o rifiuta la chiave, la ricerca si ferma (l'errore compare in cima alla scheda) e i film rimasti vengono cercati il giorno dopo.

### Stagioni mancanti e saghe

Con il tracker UNIT3D e `BOOKRR_TMDB_API_KEY` configurati, la scheda **Mancanti** dice, nella sottoscheda **Serie TV**, per ogni serie TV di cui hai caricato almeno una stagione, **quali stagioni mancano nella stessa qualità** e se sul tracker le hanno già caricate altri; nella sottoscheda **Saghe** lo stesso per i **film delle saghe** (vedi [Saghe di film](#saghe-di-film)). Senza una delle due chiavi la funzione è disattivata e la pagina spiega cosa manca.

Regole:

- contano solo i **season pack** (`S03`, `Stagione 3`) e i **pack di più stagioni o della serie intera** (`S01-S05`, `S01-05`, `S01.S02.S03`, `Season 1-3`, `Stagioni 1 e 2`, `Complete`); gli **episodi singoli** (`S01E01`, `S01E01-E10`) si ignorano, tuoi e degli altri. Un pack `Complete` senza numeri di stagione vale per le stagioni già uscite quando è stato caricato;
- **stessa qualità** = stesso tipo (WEB-DL, Remux, Encode… come lo indica il tracker) + stessa risoluzione + stesso insieme di formati HDR (DV, HDR, HLG) + stesse lingue audio. Quindi `2160p WEB-DL DV HDR ITA ENG` è diverso da `2160p WEB-DL DV HDR ITA` e da `2160p WEB-DL HDR ITA ENG`. HDR e lingue si leggono dal nome; se il nome non indica le lingue (o dice solo `MULTI`) si usano le tracce audio del *media info* del torrent;
- le **stagioni della serie** vengono da TMDB (`GET /tv/{id}`): esclusi gli speciali (stagione 0) e le stagioni non ancora uscite. Se sul tracker la release non ha l'id TMDB, bookrr lo cerca dall'id TVDB (`GET /find/{id}`); se non ci riesce la serie compare tra le **non verificabili**;
- sul tracker bookrr fa **una ricerca per serie** (`GET /api/torrents/filter?tmdbId=…`, seguendo tutte le pagine, con le stesse precauzioni dello storico: al massimo una richiesta ogni 2,5 secondi e il parametro che evita la cache condivisa). TMDB numera film e serie separatamente, quindi vengono considerati solo i torrent nelle categorie dei tuoi upload della serie. Il numero di stagione si ricava dal nome.

Per ogni serie e ogni tua qualità la griglia mostra ogni stagione come:

| Stato | Significato |
|---|---|
| **Mia** | l'hai caricata tu in questa qualità |
| **C'è, di altri** | caricata da altri nella stessa qualità |
| **Solo gruppi bannati** | caricata da altri nella stessa qualità, ma solo da [gruppi bannati](#gruppi-bannati) |
| **Solo altra qualità** | sul tracker c'è, ma solo in altre qualità (elencate nel dettaglio) |
| **Assente** | non c'è sul tracker in nessuna qualità |

Cliccando una stagione si aprono i torrent trovati, con il link alla pagina sul tracker. La lista mostra per ogni serie il numero di stagioni mancanti (la somma sulle tue qualità), i link alla serie sul tracker e su TMDB; si può cercare per nome, filtrare per qualità (risoluzione e tipo) e mostrare **solo le serie con mancanti**.

#### Saghe di film

Per ogni film che hai caricato e che su TMDB fa parte di una **saga** (una *collection*: «Il Signore degli Anelli - La collezione»), la sottoscheda **Saghe** mostra gli altri film della saga nella stessa qualità, con gli stessi quattro stati delle stagioni. Regole:

- contano solo le release della **categoria film** (una categoria del tracker che contiene `Movie` o `Film`; senza categoria, un nome senza stagioni né episodi e senza id TVDB): l'id TMDB di un film non è quello di una serie;
- la saga del film viene da TMDB (`GET /movie/{id}`, campo `belongs_to_collection`); i film senza saga non compaiono. Se sul tracker la release non ha l'id TMDB, bookrr lo cerca dall'id IMDb (`GET /find/tt…`); se non ci riesce il film compare tra i **non verificabili**;
- i **film della saga** vengono da TMDB (`GET /collection/{id}`), in ordine di uscita, **esclusi quelli non ancora usciti** (o senza data), tranne quelli che hai caricato tu;
- sul tracker bookrr fa **una ricerca per saga** (`GET /api/torrents/filter?collectionId=…`, seguendo tutte le pagine, con le stesse precauzioni delle serie). Se non restituisce nulla, o restituisce torrent di film che non sono della saga (un tracker che non conosce il filtro), cerca **film per film** (`tmdbId=…`). Valgono solo i torrent nelle categorie film; **stessa qualità** ha la stessa definizione delle serie.

Nelle celle il titolo del film è scritto senza la parte comune a tutta la saga («La compagnia dell'anello (2001)»); cliccando si aprono i torrent trovati e il link al film su TMDB. Si può cercare per nome della saga o di un film.

#### Collezioni

Una saga di TMDB è Iron Man 1, 2 e 3; l'MCU, i film Pixar o i classici Disney non sono saghe. Per questi c'è la sottoscheda **Collezioni**: con `BOOKRR_MDBLIST_API_KEY` incolli il link di una lista pubblica di [MDBList](https://mdblist.com) (es. `https://mdblist.com/lists/utente/nome-lista`, anche una lista di IMDb o Trakt copiata da MDBList) e bookrr la segue. Regole:

- i film della collezione sono quelli della lista **nell'ordine della lista**, letti a ogni controllo (`GET /lists/{utente}/{lista}/items/movie` dell'API di MDBList), quindi se la lista cambia cambia anche la collezione; titolo e data di uscita vengono da TMDB, e un film senza id TMDB si cerca dall'id IMDb. I film **non ancora usciti** non contano, tranne quelli che hai caricato tu;
- una collezione si vede **sempre**, anche se non hai caricato nessuno dei suoi film: nella vista per film ogni film è tuo, di altri, sotto la soglia o assente; la vista per qualità ha una riga per ogni qualità in cui hai caricato almeno un film della collezione;
- il tracker non conosce le liste: bookrr cerca **film per film** (`tmdbId=…`), una volta sola per film anche se è in più collezioni;
- le collezioni sono **indipendenti dalle saghe**: Iron Man compare sia nella saga sia nell'MCU, e le scelte (accettata, ignorata, completa, nascosta, qualità minima) fatte in una non valgono nell'altra;
- una lista cancellata o resa privata su MDBList compare tra le **liste non trovate**. **Smetti di seguire** (cestino accanto alla collezione) toglie la collezione con i suoi risultati e le scelte fatte sui suoi film; la lista su MDBList non viene toccata.

Aggiunta una collezione, bookrr la controlla subito; le altre si aggiornano con il controllo notturno. MDBList gratuito permette 1000 richieste al giorno: bookrr ne usa una per collezione a ogni controllo (più una quando la aggiungi).

#### Vista per stagione / per film e qualità minima

La griglia per qualità diventa difficile da leggere quando hai caricato una saga in più qualità «massime» (Alien in 2160p Remux DV, Alien³ che esiste solo in 1080p Remux): Alien³ risulta mancante in tutte le righe 2160p. Per questo ogni sottoscheda ha due viste, e il browser ricorda l'ultima scelta (la prima volta è quella per parte):

- **Per stagione / Per film**: una sola riga per serie o saga, una cella per stagione o film con la **versione migliore** (la tua, se ne hai una che raggiunge la qualità minima; altrimenti la migliore degli altri che la raggiunge; altrimenti la migliore in assoluto). «Migliore» vuol dire, in ordine: tipo (Full Disc > Remux > Encode > WEB-DL > WEBRip > HDTV), risoluzione, HDR (DV vale più degli altri), numero di lingue audio;
- **Per qualità**: la griglia descritta sopra, una riga per ogni tua qualità.

La **qualità minima** (pulsante «Minima», una per le serie, una per le saghe e una per le collezioni; ognuna può averne una propria dal menu ⋯) è fatta di tipo minimo, risoluzione minima e lingua audio richiesta, ognuno facoltativo. Una parte è **Tua** se l'hai caricata almeno in quella qualità, **C'è, di altri** se l'ha caricata qualcun altro almeno in quella qualità, **Sotto la soglia** se sul tracker c'è solo in qualità inferiori, **Assente** se non c'è. Le release di cui non si conoscono le lingue (o solo `MULTI`) contano come se avessero quella richiesta. Esempio: con minima *Encode*, «300 - L'alba di un impero» in 1080p Encode di altri va bene; con minima *Remux* è sotto la soglia.

Le tue scelte restano anche dopo i controlli successivi:

| Scelta | Dove | Effetto |
|---|---|---|
| **Conta quelle di altri** | interruttore sopra la lista | quello che hanno caricato altri (nella stessa qualità, o sopra la soglia) non conta come mancante |
| **Va bene questa** | dettaglio di una cella, su una release di altri | la cella diventa «Va bene così» e non conta come mancante; se il torrent sparisce dal tracker torna mancante |
| **Ignora** | dettaglio di una cella | la cella non conta come mancante (es. «Alien³ non esiste in 4K») |
| **Segna come completa** | menu ⋯ della serie o saga | le parti uscite finora non contano come mancanti; una stagione o un film usciti dopo sì |
| **Nascondi** | menu ⋯ (serie o saga), icona accanto a una qualità | sparisce dai numeri e dalla lista; si ripristina da «Nascoste» o dal link sotto la griglia |

«Va bene questa» e «Ignora» dalla vista per parte valgono per tutte le qualità; dalla vista per qualità solo per quella riga.

#### Gruppi bannati

Il **release group** è quello dopo l'ultimo `-` del nome (`…H.265-MaTiTa` → `MaTiTa`) e compare accanto a ogni torrent nel dettaglio di una cella. Con **Gruppi bannati** (sopra la lista, oppure l'icona del divieto accanto a una release di altri) indichi i gruppi le cui release non ti bastano; la lista è una sola per serie, saghe e collezioni e le maiuscole non contano. Se una stagione o un film c'è sul tracker **solo** da gruppi bannati:

- resta **mancante**, anche con «Conta quelle di altri», ma è in **rosa** («Solo gruppi bannati») invece che verde, per dargli meno priorità;
- nella vista per parte una release di un gruppo bannato vale solo se non ce n'è una di altri gruppi che raggiunge la qualità minima;
- il conteggio «mancanti» della serie o saga e il numero sulla sottoscheda non lo contano: è mostrato a parte («N mancanti, solo bannati»), e le serie o saghe a cui manca solo questo vanno in fondo alla lista.

Le tue release non contano mai come bannate. Un torrent accettato con «Va bene questa» resta accettato anche se il suo gruppo è bannato.

#### Quando gira

Il controllo, serie, saghe e collezioni insieme, gira **ogni notte** alle `BOOKRR_MISSING_CHECK_AT` (04:00), con **Controlla ora** e, se l'ultimo ha più di un giorno (bookrr era spento), 10 minuti dopo l'avvio. Usa i tuoi upload letti dallo storico release; i risultati restano nel database con la data dell'ultimo controllo, e un controllo non riuscito lascia quelli precedenti (se fallisce solo una delle parti, le altre vengono comunque aggiornate).

API: `GET /api/missing/series`, `GET /api/missing/sagas`, `GET /api/missing/lists` (con le collezioni seguite in `sources`), `POST /api/missing/check` (controlla ora, serie, saghe e collezioni), `PUT /api/missing/{series|sagas|lists}/prefs` (qualità minima e «conta quelle di altri»), `POST`/`DELETE /api/missing/{series|sagas|lists}/marks` (le scelte: `hide`, `complete`, `accept`, `ignore`, `threshold`), `PUT /api/missing/banned-groups` con `{"groups": ["FLUX"]}` (i gruppi bannati, in `bannedGroups` nelle risposte), `POST /api/missing/lists/sources` con `{"url": "https://mdblist.com/lists/…"}` (segui una collezione) e `DELETE /api/missing/lists/sources/{id}`.

I risultati di un controllo fatto con una versione precedente non hanno le versioni delle singole parti: fino al controllo successivo (che parte da solo 10 minuti dopo l'avvio) la vista per parte li mostra per qualità.

### Release archiviabili

Una tua release che è ancora su almeno un client è **archiviabile** quando altri la stanno già seminando: puoi toglierla dai client e archiviarla (o lasciarla agli adottatori). Il numero di seeder del tracker comprende anche te, una volta per ogni client che la semina, quindi:

> **altri seeder** = seeder del tracker − tuoi client in bookrr che la hanno **completa (100%)** e **in seed**

Contano come in seed gli stati *In seed*, *In seed (fermo)* e *Seed forzato*; un client in pausa, fermato, in coda, in verifica o con la copia incompleta non conta (non viene annunciato al tracker come seeder). Se il tracker è indietro rispetto ai client gli altri seeder non scendono mai sotto zero.

La release è archiviabile quando **altri seeder ≥ soglia** e **completati ≥ soglia** da almeno **N giorni consecutivi**. bookrr ricorda da quando le due soglie sono rispettate (*archiviabile dal*) e azzera la data appena smettono di esserlo: il conteggio dei giorni riparte da capo. La condizione viene rivalutata a ogni sincronizzazione dei client, a ogni giro dello storico release, a ogni rimozione da un client e quando cambi le soglie. Le release già archiviate o adottate (su nessun client) e quelle rimosse dal tracker non vengono valutate.

Le soglie si scelgono dall'icona **Impostazioni** (ingranaggio) in alto:

| Impostazione | Predefinito | Significato |
|---|---|---|
| Altri seeder (min.) | `1` | seeder diversi dai tuoi client |
| Completati (min.) | `1` | download completati sul tracker |
| Giorni di fila (min.) | `0` | da quanto devono valere le due soglie; `0` = subito |

Nella lista **Torrent** la colonna **Archiviabile** mostra *Sì · da N giorni* (vuota altrimenti) con, nel tooltip, seeder totali, tuoi, altri, completati, regola e data dell'ultimo aggiornamento dal tracker; la casella **Archiviabili** filtra la lista. La stessa informazione è nella scheda del torrent (anche quando le soglie sono rispettate ma mancano ancora dei giorni, o perché non lo è) e nella pagina **Release**, con il suo filtro. Non ci sono notifiche.

**Da non archiviare**: i film che vuoi tenere sui client si possono escludere. Nella vista **Archiviabili** (Torrent o Release) compare una casella su ogni riga: spunta quelli che non archivierai mai (o tutti con la casella in intestazione) e premi **Non segnare da archiviare**; per uno solo c'è anche l'interruttore nella scheda del torrent. Non compaiono più tra gli archiviabili e mostrano *Da non archiviare*; per ritrovarli usa il link *N da non archiviare · mostra* o il filtro della colonna Archiviabile (Stato nelle Release), da cui puoi selezionarli e premere **Segnala di nuovo quando archiviabile**. bookrr continua a contare i giorni in cui le soglie sono rispettate, quindi se annulli la scelta torna subito archiviabile se lo era già.

API: `GET`/`PUT /api/settings` con `archiveMinSeeders`, `archiveMinCompleted`, `archiveDays` (le opzioni non inviate restano invariate); ogni torrent e ogni release hanno il campo `archivable` (`null` se non su un client), con `neverArchive` per quelli da non archiviare. `POST /api/torrents/never-archive` con `{"hashes": ["…"], "never": true}` li segna da non archiviare (`false` annulla) e risponde `{"updated": N}`.

## API

Tutto ciò che fa l'interfaccia è disponibile via REST (`/api/torrents`, `/api/disks`, `/api/adopters`, `/api/clients`, `/api/alerts`, `/api/releases`, `/api/missing`, `/api/sync`, …). Vedi `internal/api/server.go`.

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

Il logo e i suoi colori sono definiti in `web/src/lib/accent.ts`; dopo averli cambiati, `cd web && npm run logos` rigenera `docs/logo/` e `web/public/favicon.svg`.

Struttura:

```
cmd/bookrr        entrypoint
internal/api      API REST, webhook, file statici
internal/bucket   lettura dei bucket S3 degli archivi cloud
internal/qbit     client minimale per la Web API di qBittorrent
internal/s3       client S3 minimale in sola lettura (ListObjectsV2, firma SigV4)
internal/smart    lettura dei rapporti SMART (smartctl, CrystalDiskInfo)
internal/store    SQLite (modernc.org/sqlite, puro Go, niente CGO)
internal/gaps     controllo delle stagioni mancanti delle serie e dei film mancanti delle saghe e delle collezioni
internal/mdblist  client minimale per le liste di MDBList
internal/syncer   sincronizzazione periodica
internal/tmdb     client minimale per The Movie Database
internal/unit3d   tracker UNIT3D: ricerca dell'hash, storico delle release, qualità
web/              frontend React + Vite + Tailwind + shadcn/ui
```
