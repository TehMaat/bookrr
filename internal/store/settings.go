package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// Settings are the preferences changed from the web UI (the options of the
// configuration set from it are apart, see ConfigValues).
type Settings struct {
	// Address of bookrr as seen from other devices (e.g. http://192.168.1.10:8080),
	// used in the QR codes of the disks. Empty means the address of the browser.
	PublicURL string `json:"publicUrl"`
	// When an upload of the user can be taken off the clients and archived
	// (see ArchiveRule).
	ArchiveMinSeeders   int64 `json:"archiveMinSeeders"`
	ArchiveMinCompleted int64 `json:"archiveMinCompleted"`
	ArchiveDays         int64 `json:"archiveDays"`
}

const (
	keyPublicURL           = "public_url"
	keyArchiveMinSeeders   = "archive_min_seeders"
	keyArchiveMinCompleted = "archive_min_completed"
	keyArchiveDays         = "archive_days"
)

// DefaultSettings are used for the keys never saved.
var DefaultSettings = Settings{ArchiveMinSeeders: 1, ArchiveMinCompleted: 1, ArchiveDays: 0}

// MaxArchiveDays keeps the days of the archive rule within a sane range.
const MaxArchiveDays = 3650

var ErrInvalidSettings = errors.New("soglie delle release archiviabili non valide: numeri da 0 in su, giorni al massimo 3650")

func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	out := DefaultSettings
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	num := func(v string, dst *int64) {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			*dst = n
		}
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case keyPublicURL:
			out.PublicURL = v
		case keyArchiveMinSeeders:
			num(v, &out.ArchiveMinSeeders)
		case keyArchiveMinCompleted:
			num(v, &out.ArchiveMinCompleted)
		case keyArchiveDays:
			num(v, &out.ArchiveDays)
		}
	}
	return out, rows.Err()
}

// SaveSettings stores every option. A change of the archive rule is applied
// at once to the uploads still on a client.
func (s *Store) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	if in.ArchiveMinSeeders < 0 || in.ArchiveMinCompleted < 0 || in.ArchiveDays < 0 || in.ArchiveDays > MaxArchiveDays {
		return in, ErrInvalidSettings
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return in, err
	}
	defer tx.Rollback()
	for k, v := range map[string]string{
		keyPublicURL:           in.PublicURL,
		keyArchiveMinSeeders:   strconv.FormatInt(in.ArchiveMinSeeders, 10),
		keyArchiveMinCompleted: strconv.FormatInt(in.ArchiveMinCompleted, 10),
		keyArchiveDays:         strconv.FormatInt(in.ArchiveDays, 10),
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return in, err
		}
	}
	if err := tx.Commit(); err != nil {
		return in, err
	}
	if err := s.RefreshArchivable(ctx); err != nil {
		return in, err
	}
	return s.GetSettings(ctx)
}

// configPrefix marks, among the settings, the options of the configuration
// set from the web UI: "env:BOOKRR_UNIT3D_URL".
const configPrefix = "env:"

// ConfigValues returns the options of the configuration set from the web
// UI, by environment variable.
func (s *Store) ConfigValues(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings WHERE substr(key, 1, length(?)) = ?`, configPrefix, configPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(k, configPrefix)] = v
	}
	return out, rows.Err()
}

// SaveConfigValues replaces the options of the configuration set from the
// web UI: an empty value goes back to the default.
func (s *Store) SaveConfigValues(ctx context.Context, values map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE substr(key, 1, length(?)) = ?`, configPrefix, configPrefix); err != nil {
		return err
	}
	for k, v := range values {
		if v == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)`, configPrefix+k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}
