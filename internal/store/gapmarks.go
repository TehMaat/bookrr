package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// GapThreshold is the lowest quality a part of a collection counts as
// present in: a type ("Remux", "Encode"…), a resolution ("1080p") and an
// audio language ("ITA"), each "" for any.
type GapThreshold struct {
	Type       string `json:"type"`
	Resolution string `json:"resolution"`
	Lang       string `json:"lang"`
}

// GapPrefs are the user's options for one kind of gaps check.
type GapPrefs struct {
	GapThreshold
	// What others uploaded counts as present.
	OthersCount bool `json:"othersCount"`
}

// GapMark is a choice of the user on a result of the gaps checks (see the
// gap_marks table).
type GapMark struct {
	Kind      string `json:"kind"`
	Group     string `json:"group"`
	Action    string `json:"action"`
	Quality   string `json:"quality"`
	Unit      int64  `json:"unit"`
	Torrent   int64  `json:"torrent"`
	Value     string `json:"value"`
	CreatedAt string `json:"createdAt"`
}

func gapPrefKey(kind, name string) string { return "gaps_" + kind + "_" + name }

// GapPrefs returns the options of a kind of check, empty if never saved.
func (s *Store) GapPrefs(ctx context.Context, kind string) (GapPrefs, error) {
	var p GapPrefs
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings WHERE key LIKE ?`, gapPrefKey(kind, "%"))
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return p, err
		}
		switch k {
		case gapPrefKey(kind, "min_type"):
			p.Type = v
		case gapPrefKey(kind, "min_resolution"):
			p.Resolution = v
		case gapPrefKey(kind, "lang"):
			p.Lang = v
		case gapPrefKey(kind, "others_count"):
			p.OthersCount = v == "1"
		}
	}
	return p, rows.Err()
}

// SaveGapPrefs stores the options of a kind of check.
func (s *Store) SaveGapPrefs(ctx context.Context, kind string, p GapPrefs) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	others := "0"
	if p.OthersCount {
		others = "1"
	}
	for k, v := range map[string]string{
		gapPrefKey(kind, "min_type"):       p.Type,
		gapPrefKey(kind, "min_resolution"): p.Resolution,
		gapPrefKey(kind, "lang"):           p.Lang,
		gapPrefKey(kind, "others_count"):   others,
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GapMarks returns the user's choices on a kind of check.
func (s *Store) GapMarks(ctx context.Context, kind string) ([]GapMark, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT kind, group_key, action, quality, unit, torrent, value, created_at
FROM gap_marks WHERE kind = ? ORDER BY group_key, action, quality, unit`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GapMark{}
	for rows.Next() {
		var m GapMark
		if err := rows.Scan(&m.Kind, &m.Group, &m.Action, &m.Quality, &m.Unit, &m.Torrent, &m.Value, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetGapMark stores a choice, replacing the same one. Accepting a torrent
// for a part and ignoring it exclude each other.
func (s *Store) SetGapMark(ctx context.Context, m GapMark) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	other := map[string]string{"accept": "ignore", "ignore": "accept"}[m.Action]
	if other != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM gap_marks WHERE kind = ? AND group_key = ? AND action = ? AND quality = ? AND unit = ?`,
			m.Kind, m.Group, other, m.Quality, m.Unit); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO gap_marks (kind, group_key, action, quality, unit, torrent, value, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (kind, group_key, action, quality, unit) DO UPDATE SET torrent = excluded.torrent, value = excluded.value,
	created_at = excluded.created_at`,
		m.Kind, m.Group, m.Action, m.Quality, m.Unit, m.Torrent, m.Value, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteGapMark removes a choice; removing one that is not there is fine.
func (s *Store) DeleteGapMark(ctx context.Context, m GapMark) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM gap_marks WHERE kind = ? AND group_key = ? AND action = ? AND quality = ? AND unit = ?`,
		m.Kind, m.Group, m.Action, m.Quality, m.Unit)
	return err
}

// GapGroup returns the stored result of one collection.
func (s *Store) GapGroup(ctx context.Context, kind, key string) (GapGroup, error) {
	g := GapGroup{Key: key}
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT title, missing, data, checked_at FROM gap_groups WHERE kind = ? AND key = ?`, kind, key).
		Scan(&g.Title, &g.Missing, &data, &g.CheckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrNotFound
	}
	g.Data = []byte(data)
	return g, err
}

const keyBannedGroups = "gaps_banned_groups"

// BannedGroups returns the release groups the user does not want, for
// every kind of check, as saved.
func (s *Store) BannedGroups(ctx context.Context) ([]string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, keyBannedGroups).Scan(&v)
	out := []string{}
	if errors.Is(err, sql.ErrNoRows) || v == "" {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, json.Unmarshal([]byte(v), &out)
}

// SaveBannedGroups replaces the release groups the user does not want.
func (s *Store) SaveBannedGroups(ctx context.Context, groups []string) error {
	if groups == nil {
		groups = []string{}
	}
	data, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, keyBannedGroups, string(data))
	return err
}
