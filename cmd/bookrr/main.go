package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tehmaat/bookrr/internal/api"
	"github.com/tehmaat/bookrr/internal/bucket"
	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
	"github.com/tehmaat/bookrr/internal/unit3d"
	"github.com/tehmaat/bookrr/web"
)

var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "verifica che il server risponda ed esce")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configurazione", "err", err)
		os.Exit(1)
	}
	if *healthcheck {
		os.Exit(runHealthcheck(cfg))
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("cartella dati", "dir", cfg.DataDir, "err", err)
		os.Exit(1)
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		slog.Error("database", "path", cfg.DBPath(), "err", err)
		os.Exit(1)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sy := syncer.New(st, cfg.ReleaseTag, cfg.SyncInterval)
	go sy.Run(ctx)

	buckets := bucket.New(st, cfg.ReleaseTag, cfg.S3ScanInterval)
	go buckets.Run(ctx)

	var hashes *unit3d.Resolver
	if cfg.Unit3DURL != "" {
		hashes = unit3d.NewResolver(st, unit3d.New(cfg.Unit3DURL, cfg.Unit3DAPIKey))
		go hashes.Run(ctx)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(cfg, st, sy, buckets, hashes, web.FS(), version).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	slog.Info("bookrr avviato", "version", version, "listen", cfg.Listen, "data", cfg.DataDir,
		"sync", cfg.SyncInterval.String(), "s3Scan", cfg.S3ScanInterval.String(), "releaseTag", cfg.ReleaseTag, "unit3d", cfg.Unit3DURL)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

// runHealthcheck lets the distroless-style image check itself without curl.
func runHealthcheck(cfg config.Config) int {
	addr := cfg.Listen
	if addr != "" && addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(fmt.Sprintf("http://%s/api/health", addr))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
