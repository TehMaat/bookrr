package config

import "os"

// Option is a setting that can be changed from the web UI, unless its
// environment variable is set (the environment wins). The address, the
// data folder, the login and the webhook token stay in the environment
// alone: they are needed before the database is open, or protect the UI.
type Option struct {
	Key     string `json:"key"` // the environment variable
	Group   string `json:"group"`
	Label   string `json:"label"`
	Default string `json:"default"`
	Help    string `json:"help"`
	// Never sent to the browser: the UI only knows whether it is set.
	Secret bool `json:"secret"`
}

// Groups of the options, as shown in the UI.
const (
	GroupGeneral  = "general"
	GroupUnit3D   = "unit3d"
	GroupMissing  = "missing"
	GroupProwlarr = "prowlarr"
)

var Options = []Option{
	{Key: "BOOKRR_RELEASE_TAG", Group: GroupGeneral, Label: "Tag delle tue release", Default: "Personal Release",
		Help: "Tag qBittorrent che identifica le tue release (maiuscole/minuscole indifferenti)."},
	{Key: "BOOKRR_SYNC_INTERVAL", Group: GroupGeneral, Label: "Lettura dei client", Default: "5m",
		Help: "Ogni quanto leggere i client qBittorrent (minimo 30s)."},
	{Key: "BOOKRR_S3_SCAN_INTERVAL", Group: GroupGeneral, Label: "Lettura dei bucket S3", Default: "1h",
		Help: "Ogni quanto leggere i bucket degli archivi cloud (minimo 1m)."},

	{Key: "BOOKRR_UNIT3D_URL", Group: GroupUnit3D, Label: "Indirizzo del tracker",
		Help: "Es. https://tracker.example: serve a trovare l'hash dei torrent aggiunti senza e per lo storico delle tue release."},
	{Key: "BOOKRR_UNIT3D_API_KEY", Group: GroupUnit3D, Label: "API key", Secret: true,
		Help: "Sul tracker: Impostazioni → API key."},
	{Key: "BOOKRR_UNIT3D_RELEASES_INTERVAL", Group: GroupUnit3D, Label: "Lettura dei tuoi upload", Default: "6h",
		Help: "Ogni quanto rileggere i tuoi upload per lo storico release (minimo 15m)."},

	{Key: "BOOKRR_TMDB_API_KEY", Group: GroupMissing, Label: "Chiave TMDB", Secret: true,
		Help: "Chiave API (v3) o token di lettura (v4) di TMDB: con il tracker attiva le stagioni mancanti e i film delle saghe."},
	{Key: "BOOKRR_MDBLIST_API_KEY", Group: GroupMissing, Label: "Chiave MDBList", Secret: true,
		Help: "Gratuita, nelle preferenze del tuo account MDBList: con il tracker e TMDB attiva le collezioni."},
	{Key: "BOOKRR_MISSING_CHECK_AT", Group: GroupMissing, Label: "Ora del controllo notturno", Default: "04:00",
		Help: "Ora locale del controllo delle stagioni e dei film mancanti."},

	{Key: "BOOKRR_PROWLARR_URL", Group: GroupProwlarr, Label: "Indirizzo di Prowlarr",
		Help: "Es. http://prowlarr:9696: con il tracker attiva la risoluzione massima dei film caricati."},
	{Key: "BOOKRR_PROWLARR_API_KEY", Group: GroupProwlarr, Label: "API key", Secret: true,
		Help: "In Prowlarr: Settings → General → API Key."},
	{Key: "BOOKRR_PROWLARR_RECHECK", Group: GroupProwlarr, Label: "Nuova ricerca di ogni film", Default: "7d",
		Help: "Ogni quanto cercare di nuovo ogni film (minimo 1d; anche in ore, es. 36h)."},
}

// IsOption reports whether an environment variable can be set from the UI.
func IsOption(key string) bool {
	for _, o := range Options {
		if o.Key == key {
			return true
		}
	}
	return false
}

// Locked reports whether an option is set in the environment: the UI shows
// it but cannot change it.
func Locked(key string) bool { return os.Getenv(key) != "" }
