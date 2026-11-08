package syncer

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/wayfinderr"
)

// Wayfinderr hands the .torrent files of the plan over to the Wayfinderr of
// the clients that have one: the ones complete on such a client are sent as
// the source .torrent of their film (Wayfinderr copies it into its upload as
// soon as the rip is done), and once the film is released (found on the
// tracker) bookrr drops its own copy, after checking Wayfinderr has it.
// Failures are kept on the torrent and tried again at the next sync.
func Wayfinderr(ctx context.Context, st *store.Store) (sent, dropped int, err error) {
	sends, err := st.PlanTorrentsForWayfinderr(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, w := range sends {
		wctx, cancel := context.WithTimeout(ctx, time.Minute)
		sendErr := wayfinderr.New(w.Client.WayfinderrURL, w.Client.WayfinderrAPIKey).AddSourceTorrent(wctx, wayfinderr.SourceTorrent{
			FileName: FileName(w.Name, w.Hash), Data: w.Data, TmdbID: w.TmdbID, Label: w.Label,
		})
		cancel()
		if ctx.Err() != nil {
			return sent, dropped, ctx.Err()
		}
		if err := st.SetPlanTorrentWayfinderr(ctx, w.PlanID, w.Position, w.Client.ID, sendErr); err != nil {
			return sent, dropped, err
		}
		if sendErr != nil {
			slog.Warn("sending a plan torrent to Wayfinderr failed", "plan", w.PlanID, "position", w.Position,
				"client", w.Client.Name, "err", sendErr)
			continue
		}
		sent++
		slog.Info("plan torrent sent to Wayfinderr", "plan", w.PlanID, "position", w.Position, "hash", w.Hash,
			"client", w.Client.Name)
	}

	drops, err := st.PlanTorrentsToDrop(ctx)
	if err != nil {
		return sent, dropped, err
	}
	for _, d := range drops {
		if !d.Client.HasWayfinderr() {
			// The client no longer has a Wayfinderr to check with: kept.
			continue
		}
		wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		has, checkErr := wayfinderr.New(d.Client.WayfinderrURL, d.Client.WayfinderrAPIKey).HasSourceTorrent(wctx, d.Hash)
		cancel()
		if ctx.Err() != nil {
			return sent, dropped, ctx.Err()
		}
		if checkErr != nil || !has {
			// Not there (or not reachable): kept, checked again at the next sync.
			slog.Warn("plan torrent kept: Wayfinderr does not confirm it has it", "plan", d.PlanID, "position", d.Position,
				"client", d.Client.Name, "found", has, "err", checkErr)
			continue
		}
		if err := st.DropPlanTorrentFile(ctx, d.PlanID, d.Position); err != nil {
			return sent, dropped, err
		}
		dropped++
		slog.Info("plan torrent file dropped: the film is released, the file is on Wayfinderr", "plan", d.PlanID,
			"position", d.Position, "hash", d.Hash, "client", d.Client.Name)
	}
	return sent, dropped, nil
}

// FileName is the name of the .torrent file of a torrent, without slashes.
func FileName(name, hash string) string {
	name = strings.NewReplacer("/", "_", "\\", "_").Replace(name)
	if name == "" {
		name = hash
	}
	return name + ".torrent"
}
