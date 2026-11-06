# API

[Documentazione](README.md) · [English](../en/api.md)

Tutto ciò che fa l'interfaccia è disponibile come API REST in JSON sotto `/api`, protetta dallo stesso login dell'interfaccia (`BOOKRR_AUTH_USER` / `BOOKRR_AUTH_PASSWORD`, HTTP Basic). Gli errori rispondono `{"error": "…"}` con uno stato 4xx o 5xx. Le rotte sono in [`internal/api/server.go`](../../internal/api/server.go).

| Area | Rotte |
|---|---|
| Info e sincronizzazione | `GET /api/info`, `GET /api/sync`, `POST /api/sync` (legge subito i client) |
| Torrent | `GET`/`POST /api/torrents`, `POST /api/torrents/import` (`{"items": […], "dryRun": true}`), `PATCH`/`DELETE /api/torrents/{hash}`, `POST /api/torrents/{hash}/archives`, `POST /api/torrents/{hash}/adoptions`, `DELETE /api/torrents/{hash}/locations/{clientId}?deleteFiles=1`, `POST /api/torrents/never-archive` |
| Tracker | `POST /api/torrents/{hash}/lookup-hash`, `GET /api/torrents/{hash}/tracker-candidates?q=…`, `POST /api/torrents/{hash}/tracker-match`, `GET /api/torrents/{hash}/torrent-file` |
| Release | `GET /api/releases`, `POST /api/releases/sync`, `POST /api/releases/add` (`{"ids": [123]}` o `{"all": true}`), `POST /api/releases/unflag` (`{"hashes": ["…"]}` o `{"all": true}`) |
| Segnalazioni | `GET /api/alerts?open=1`, `POST /api/alerts/{id}/resolve`, `POST /api/alerts/resolve` (più di una insieme) |
| Dischi | `GET`/`POST /api/disks`, `PUT`/`DELETE /api/disks/{id}`, `POST /api/disks/smart`, `POST /api/disks/test-s3`, `POST /api/disks/{id}/scan`, `GET /api/disks/{id}/bucket-entries` |
| Archivi e adozioni | `PUT`/`DELETE /api/archives/{id}`, `DELETE /api/adoptions/{id}` |
| Adottatori | `GET`/`POST /api/adopters`, `PUT`/`DELETE /api/adopters/{id}` |
| Client | `GET`/`POST /api/clients`, `PUT`/`DELETE /api/clients/{id}`, `POST /api/clients/test` |
| Mancanti | vedi [Stagioni e film mancanti](mancanti.md#quando-gira) |
| Piano | vedi [Piano release](prowlarr-e-piano.md#piano-release) |
| Log | vedi [Log](log.md) |
| Impostazioni | `GET`/`PUT /api/settings` (soglie delle archiviabili, `archiveMinSeeders`, `archiveMinCompleted`, `archiveDays`, e l'indirizzo usato dai QR code, `publicUrl`) |
| Configurazione | `GET /api/config` (le opzioni, senza le chiavi), `PUT /api/config` con `{"values": {"BOOKRR_UNIT3D_URL": "https://…", "BOOKRR_UNIT3D_API_KEY": null}}` (`null` lascia il valore com'è, `""` torna al default), `POST /api/config/test` con `{"service": "unit3d" \| "tmdb" \| "mdblist" \| "prowlarr", "values": {…}}` |
| Webhook | `POST /api/webhook/qbit`, vedi [Client e segnalazioni](client-e-segnalazioni.md#webhook-di-rimozione) |

Ogni torrent e ogni release hanno il campo `archivable` (`null` se non è su un client), con `neverArchive` per quelli da non archiviare.
