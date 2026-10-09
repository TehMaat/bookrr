package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Location struct {
	ClientID   int64   `json:"clientId"`
	ClientName string  `json:"clientName"`
	SavePath   string  `json:"savePath"`
	State      string  `json:"state"`
	Progress   float64 `json:"progress"`
	Ratio      float64 `json:"ratio"`
	Tags       string  `json:"tags"`
	Category   string  `json:"category"`
	AddedOn    int64   `json:"addedOn"`
	SeenAt     string  `json:"seenAt"`
}

type Archive struct {
	ID         int64  `json:"id"`
	DiskID     *int64 `json:"diskId"`
	DiskLabel  string `json:"diskLabel"`
	DiskSerial string `json:"diskSerial"`
	DiskKind   string `json:"diskKind"`
	Path       string `json:"path"`
	Notes      string `json:"notes"`
	CreatedAt  string `json:"createdAt"`
	hash       string
}

type Alert struct {
	ID          int64   `json:"id"`
	Hash        string  `json:"hash"`
	TorrentName string  `json:"torrentName"`
	Source      string  `json:"source"`
	ClientName  string  `json:"clientName"`
	Message     string  `json:"message"`
	CreatedAt   string  `json:"createdAt"`
	ResolvedAt  *string `json:"resolvedAt"`
	Resolution  string  `json:"resolution"`
}

type Torrent struct {
	Hash            string     `json:"hash"`
	Name            string     `json:"name"`
	Size            int64      `json:"size"`
	Tags            []string   `json:"tags"`
	Category        string     `json:"category"`
	Tracker         string     `json:"tracker"`
	PersonalRelease bool       `json:"personalRelease"`
	Manual          bool       `json:"manual"`
	AdoptedBy       string     `json:"adoptedBy"`
	Notes           string     `json:"notes"`
	FirstSeenAt     string     `json:"firstSeenAt"`
	LastSeenAt      *string    `json:"lastSeenAt"`
	UpdatedAt       string     `json:"updatedAt"`
	Locations       []Location `json:"locations"`
	Archives        []Archive  `json:"archives"`
	OpenAlert       *Alert     `json:"openAlert"`
	Duplicate       bool       `json:"duplicate"`
	Status          string     `json:"status"`
}

// Status values exposed to the UI.
const (
	StatusClient   = "client"
	StatusMissing  = "missing"
	StatusArchived = "archived"
	StatusAdopted  = "adopted"
	StatusUnknown  = "unknown"
)

func (t *Torrent) finalize() {
	t.Duplicate = len(t.Locations) > 1
	switch {
	case len(t.Locations) > 0:
		t.Status = StatusClient
	case t.OpenAlert != nil:
		t.Status = StatusMissing
	case len(t.Archives) > 0:
		t.Status = StatusArchived
	case t.AdoptedBy != "":
		t.Status = StatusAdopted
	default:
		t.Status = StatusUnknown
	}
}

const torrentCols = `hash, name, size, tags, category, tracker, personal_release, manual, adopted_by, notes, first_seen_at, last_seen_at, updated_at`

func scanTorrent(row interface{ Scan(...any) error }) (Torrent, error) {
	var t Torrent
	var tags string
	var last sql.NullString
	err := row.Scan(&t.Hash, &t.Name, &t.Size, &tags, &t.Category, &t.Tracker, &t.PersonalRelease, &t.Manual, &t.AdoptedBy, &t.Notes, &t.FirstSeenAt, &last, &t.UpdatedAt)
	t.Tags = SplitTags(tags)
	t.LastSeenAt = nullStr(last)
	t.Locations = []Location{}
	t.Archives = []Archive{}
	return t, err
}

// ListTorrents returns every known torrent with its locations, archives and open alert.
func (s *Store) ListTorrents(ctx context.Context) ([]Torrent, error) {
	return s.loadTorrents(ctx, "")
}

func (s *Store) GetTorrent(ctx context.Context, hash string) (Torrent, error) {
	ts, err := s.loadTorrents(ctx, hash)
	if err != nil {
		return Torrent{}, err
	}
	if len(ts) == 0 {
		return Torrent{}, ErrNotFound
	}
	return ts[0], nil
}

func (s *Store) loadTorrents(ctx context.Context, hash string) ([]Torrent, error) {
	where, args := "", []any{}
	if hash != "" {
		where, args = " WHERE hash = ?", []any{hash}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+torrentCols+` FROM torrents`+where+` ORDER BY name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	out := []Torrent{}
	idx := map[string]int{}
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		idx[t.Hash] = len(out)
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	lwhere := ""
	if hash != "" {
		lwhere = " WHERE l.hash = ?"
	}
	rows, err = s.db.QueryContext(ctx, `
SELECT l.hash, l.client_id, c.name, l.save_path, l.state, l.progress, l.ratio, l.tags, l.category, l.added_on, l.seen_at
FROM locations l JOIN clients c ON c.id = l.client_id`+lwhere+` ORDER BY c.name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h string
		var l Location
		if err := rows.Scan(&h, &l.ClientID, &l.ClientName, &l.SavePath, &l.State, &l.Progress, &l.Ratio, &l.Tags, &l.Category, &l.AddedOn, &l.SeenAt); err != nil {
			rows.Close()
			return nil, err
		}
		if i, ok := idx[h]; ok {
			out[i].Locations = append(out[i].Locations, l)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	archives, err := s.listArchives(ctx, hash)
	if err != nil {
		return nil, err
	}
	for _, a := range archives {
		if i, ok := idx[a.hash]; ok {
			out[i].Archives = append(out[i].Archives, a)
		}
	}

	alerts, err := s.ListAlerts(ctx, true, hash)
	if err != nil {
		return nil, err
	}
	for _, a := range alerts {
		if i, ok := idx[a.Hash]; ok && out[i].OpenAlert == nil {
			a := a
			out[i].OpenAlert = &a
		}
	}

	for i := range out {
		out[i].finalize()
	}
	return out, nil
}

func (s *Store) listArchives(ctx context.Context, hash string) ([]Archive, error) {
	where, args := "", []any{}
	if hash != "" {
		where, args = " WHERE a.hash = ?", []any{hash}
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.hash, a.disk_id, COALESCE(d.label, ''), COALESCE(d.serial, ''), COALESCE(d.kind, ''), a.path, a.notes, a.created_at
FROM archives a LEFT JOIN disks d ON d.id = a.disk_id`+where+` ORDER BY a.created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Archive{}
	for rows.Next() {
		var a Archive
		var disk sql.NullInt64
		if err := rows.Scan(&a.ID, &a.hash, &disk, &a.DiskLabel, &a.DiskSerial, &a.DiskKind, &a.Path, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		if disk.Valid {
			a.DiskID = &disk.Int64
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type TorrentInput struct {
	Hash            string   `json:"hash"`
	Name            string   `json:"name"`
	Size            int64    `json:"size"`
	Tags            []string `json:"tags"`
	Category        string   `json:"category"`
	Tracker         string   `json:"tracker"`
	PersonalRelease bool     `json:"personalRelease"`
	AdoptedBy       string   `json:"adoptedBy"`
	Notes           string   `json:"notes"`
}

var ErrExists = errors.New("esiste già")

func (s *Store) CreateManualTorrent(ctx context.Context, in TorrentInput) (Torrent, error) {
	ts := now()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO torrents (hash, name, size, tags, category, tracker, personal_release, manual, adopted_by, notes, first_seen_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
		in.Hash, in.Name, in.Size, strings.Join(in.Tags, ","), in.Category, in.Tracker, boolInt(in.PersonalRelease), in.AdoptedBy, in.Notes, ts, ts)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Torrent{}, ErrExists
		}
		return Torrent{}, err
	}
	return s.GetTorrent(ctx, in.Hash)
}

// TorrentPatch holds user-editable fields; nil means "unchanged".
type TorrentPatch struct {
	Name            *string   `json:"name"`
	Size            *int64    `json:"size"`
	Tags            *[]string `json:"tags"`
	Category        *string   `json:"category"`
	PersonalRelease *bool     `json:"personalRelease"`
	AdoptedBy       *string   `json:"adoptedBy"`
	Notes           *string   `json:"notes"`
}

func (s *Store) UpdateTorrent(ctx context.Context, hash string, p TorrentPatch) (Torrent, error) {
	sets, args := []string{}, []any{}
	add := func(col string, v any) { sets = append(sets, col+" = ?"); args = append(args, v) }
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.Size != nil {
		add("size", *p.Size)
	}
	if p.Tags != nil {
		add("tags", strings.Join(*p.Tags, ","))
	}
	if p.Category != nil {
		add("category", *p.Category)
	}
	if p.PersonalRelease != nil {
		add("personal_release", boolInt(*p.PersonalRelease))
	}
	if p.AdoptedBy != nil {
		add("adopted_by", strings.TrimSpace(*p.AdoptedBy))
	}
	if p.Notes != nil {
		add("notes", *p.Notes)
	}
	add("updated_at", now())
	args = append(args, hash)
	res, err := s.db.ExecContext(ctx, `UPDATE torrents SET `+strings.Join(sets, ", ")+` WHERE hash = ?`, args...)
	if err != nil {
		return Torrent{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Torrent{}, ErrNotFound
	}
	return s.GetTorrent(ctx, hash)
}

var ErrOnClient = errors.New("il torrent è ancora presente su un client")

func (s *Store) DeleteTorrent(ctx context.Context, hash string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM locations WHERE hash = ?`, hash).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrOnClient
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM torrents WHERE hash = ?`, hash)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type ArchiveInput struct {
	DiskID *int64 `json:"diskId"`
	Path   string `json:"path"`
	Notes  string `json:"notes"`
}

func (s *Store) AddArchive(ctx context.Context, hash string, in ArchiveInput) error {
	return addArchive(ctx, s.db, hash, in)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func addArchive(ctx context.Context, db execer, hash string, in ArchiveInput) error {
	_, err := db.ExecContext(ctx, `INSERT INTO archives (hash, disk_id, path, notes, created_at) VALUES (?, ?, ?, ?, ?)`,
		hash, in.DiskID, in.Path, in.Notes, now())
	if err != nil && strings.Contains(err.Error(), "FOREIGN KEY") {
		return ErrNotFound
	}
	return err
}

func (s *Store) UpdateArchive(ctx context.Context, id int64, in ArchiveInput) error {
	res, err := s.db.ExecContext(ctx, `UPDATE archives SET disk_id = ?, path = ?, notes = ? WHERE id = ?`, in.DiskID, in.Path, in.Notes, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteArchive(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM archives WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
