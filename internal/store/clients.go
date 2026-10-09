package store

import (
	"context"
	"database/sql"
	"errors"
)

type Client struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	URL           string  `json:"url"`
	Username      string  `json:"username"`
	Password      string  `json:"-"`
	HasPassword   bool    `json:"hasPassword"`
	SkipTLSVerify bool    `json:"skipTlsVerify"`
	Enabled       bool    `json:"enabled"`
	LastSyncAt    *string `json:"lastSyncAt"`
	LastError     string  `json:"lastError"`
	TorrentCount  int     `json:"torrentCount"`
	CreatedAt     string  `json:"createdAt"`
}

const clientCols = `id, name, url, username, password, skip_tls_verify, enabled, last_sync_at, last_error, torrent_count, created_at`

func scanClient(row interface{ Scan(...any) error }) (Client, error) {
	var c Client
	var last sql.NullString
	err := row.Scan(&c.ID, &c.Name, &c.URL, &c.Username, &c.Password, &c.SkipTLSVerify, &c.Enabled, &last, &c.LastError, &c.TorrentCount, &c.CreatedAt)
	c.LastSyncAt = nullStr(last)
	c.HasPassword = c.Password != ""
	return c, err
}

func (s *Store) ListClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+clientCols+` FROM clients ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetClient(ctx context.Context, id int64) (Client, error) {
	c, err := scanClient(s.db.QueryRowContext(ctx, `SELECT `+clientCols+` FROM clients WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateClient(ctx context.Context, c Client) (Client, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO clients (name, url, username, password, skip_tls_verify, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.URL, c.Username, c.Password, boolInt(c.SkipTLSVerify), boolInt(c.Enabled), now())
	if err != nil {
		return c, err
	}
	id, _ := res.LastInsertId()
	return s.GetClient(ctx, id)
}

func (s *Store) UpdateClient(ctx context.Context, c Client) (Client, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE clients SET name = ?, url = ?, username = ?, password = ?, skip_tls_verify = ?, enabled = ? WHERE id = ?`,
		c.Name, c.URL, c.Username, c.Password, boolInt(c.SkipTLSVerify), boolInt(c.Enabled), c.ID)
	if err != nil {
		return c, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return c, ErrNotFound
	}
	return s.GetClient(ctx, c.ID)
}

// DeleteClient removes the client and its locations. Torrents left with no
// location are cleaned up by the next sync.
func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetClientSyncResult(ctx context.Context, id int64, count int, syncErr error) error {
	if syncErr != nil {
		_, err := s.db.ExecContext(ctx, `UPDATE clients SET last_error = ? WHERE id = ?`, syncErr.Error(), id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE clients SET last_error = '', last_sync_at = ?, torrent_count = ? WHERE id = ?`, now(), count, id)
	return err
}

func (s *Store) ClientByName(ctx context.Context, name string) (Client, error) {
	c, err := scanClient(s.db.QueryRowContext(ctx, `SELECT `+clientCols+` FROM clients WHERE name = ? COLLATE NOCASE`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}
