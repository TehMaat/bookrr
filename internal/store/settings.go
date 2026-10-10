package store

import "context"

// Settings are the options changed from the web UI, as opposed to the
// environment variables read at startup.
type Settings struct {
	// Address of bookrr as seen from other devices (e.g. http://192.168.1.10:8080),
	// used in the QR codes of the disks. Empty means the address of the browser.
	PublicURL string `json:"publicUrl"`
}

func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	var out Settings
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case "public_url":
			out.PublicURL = v
		}
	}
	return out, rows.Err()
}

func (s *Store) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES ('public_url', ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		in.PublicURL)
	if err != nil {
		return in, err
	}
	return s.GetSettings(ctx)
}
