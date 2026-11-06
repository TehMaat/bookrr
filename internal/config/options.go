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
	{Key: "BOOKRR_RELEASE_TAG", Group: GroupGeneral, Label: "Tag of your releases", Default: "Personal Release",
		Help: "qBittorrent tag that marks your releases (case insensitive)."},
	{Key: "BOOKRR_SYNC_INTERVAL", Group: GroupGeneral, Label: "Client reads", Default: "5m",
		Help: "How often to read the qBittorrent clients (minimum 30s)."},
	{Key: "BOOKRR_S3_SCAN_INTERVAL", Group: GroupGeneral, Label: "S3 bucket reads", Default: "1h",
		Help: "How often to read the buckets of the cloud archives (minimum 1m)."},
	{Key: "BOOKRR_LOG_RETENTION", Group: GroupGeneral, Label: "Log retention", Default: "30d",
		Help: "How many days to keep the log files in the data folder (logs/): minimum 1d, 0 forever."},
	{Key: "BOOKRR_LOG_MAX_SIZE", Group: GroupGeneral, Label: "Maximum size of a log file", Default: "50MB",
		Help: "Past this size the day goes on in a new file (bookrr-YYYY-MM-DD.2.log); 0 for no limit."},
	{Key: "BOOKRR_LOG_LEVEL", Group: GroupGeneral, Label: "Log level", Default: "info",
		Help: "Minimum level of the messages written: debug, info, warn or error."},

	{Key: "BOOKRR_UNIT3D_URL", Group: GroupUnit3D, Label: "Tracker address",
		Help: "E.g. https://tracker.example: used to find the hash of torrents added without one and for the history of your releases."},
	{Key: "BOOKRR_UNIT3D_API_KEY", Group: GroupUnit3D, Label: "API key", Secret: true,
		Help: "On the tracker: Settings → API key."},
	{Key: "BOOKRR_UNIT3D_RELEASES_INTERVAL", Group: GroupUnit3D, Label: "Your uploads reads", Default: "6h",
		Help: "How often to read your uploads again for the release history (minimum 15m)."},

	{Key: "BOOKRR_TMDB_API_KEY", Group: GroupMissing, Label: "TMDB key", Secret: true,
		Help: "TMDB API key (v3) or read access token (v4): with the tracker it enables missing seasons and saga films."},
	{Key: "BOOKRR_MDBLIST_API_KEY", Group: GroupMissing, Label: "MDBList key", Secret: true,
		Help: "Free, in your MDBList account preferences: with the tracker and TMDB it enables collections."},
	{Key: "BOOKRR_MISSING_CHECK_AT", Group: GroupMissing, Label: "Nightly check time", Default: "04:00",
		Help: "Local time of the missing seasons and films check."},

	{Key: "BOOKRR_PROWLARR_URL", Group: GroupProwlarr, Label: "Prowlarr address",
		Help: "E.g. http://prowlarr:9696: with the tracker it enables the max resolution of your uploaded films."},
	{Key: "BOOKRR_PROWLARR_API_KEY", Group: GroupProwlarr, Label: "API key", Secret: true,
		Help: "In Prowlarr: Settings → General → API Key."},
	{Key: "BOOKRR_PROWLARR_RECHECK", Group: GroupProwlarr, Label: "New search of each film", Default: "7d",
		Help: "How often to search each film again (minimum 1d; hours work too, e.g. 36h)."},
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
