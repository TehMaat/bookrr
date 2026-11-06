package store

import "context"

// BestResolution is the best release found with Prowlarr for a film (by
// type, then resolution), whatever the language.
type BestResolution struct {
	Film string `json:"film"` // "tmdb:603", "imdb:133093"
	// "" when Prowlarr found no release of the film with a resolution.
	Resolution string `json:"resolution"`
	// Type of the release, from its name ("Remux", "WEB-DL"), "" if unknown.
	Type    string `json:"type"`
	Name    string `json:"name"`
	Indexer string `json:"indexer"`
	URL     string `json:"url"`
	// Releases of the film found, of any resolution.
	Results int `json:"results"`
	// "" when it was not searched: the user already has the film in a
	// 2160p Remux.
	CheckedAt string `json:"checkedAt"`
	// Version of the order that chose the release (see prowlarr.Ranking).
	Ranking int `json:"-"`

	// Set when read: resolution and type of the best of the user's uploads
	// of the film, and whether the one found is better.
	Mine     string `json:"mine"`
	MineType string `json:"mineType"`
	Better   bool   `json:"better"`
}

// SaveBestResolution records the outcome of the search of a film.
func (s *Store) SaveBestResolution(ctx context.Context, b BestResolution) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO best_resolutions (film, resolution, name, indexer, url, results, checked_at, ranking) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (film) DO UPDATE SET resolution = excluded.resolution, name = excluded.name, indexer = excluded.indexer,
	url = excluded.url, results = excluded.results, checked_at = excluded.checked_at, ranking = excluded.ranking`,
		b.Film, b.Resolution, b.Name, b.Indexer, b.URL, b.Results, b.CheckedAt, b.Ranking)
	return err
}

// BestResolutions returns the outcome of the last search of every film, by film.
func (s *Store) BestResolutions(ctx context.Context) (map[string]BestResolution, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT film, resolution, name, indexer, url, results, checked_at, ranking FROM best_resolutions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]BestResolution{}
	for rows.Next() {
		var b BestResolution
		if err := rows.Scan(&b.Film, &b.Resolution, &b.Name, &b.Indexer, &b.URL, &b.Results, &b.CheckedAt, &b.Ranking); err != nil {
			return nil, err
		}
		out[b.Film] = b
	}
	return out, rows.Err()
}
