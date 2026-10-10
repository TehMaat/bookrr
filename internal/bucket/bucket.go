// Package bucket reads the S3 buckets behind cloud archives and records
// which torrents are stored there. It only lists the buckets: nothing is
// ever written or deleted in them.
package bucket

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/s3"
	"github.com/tehmaat/bookrr/internal/store"
)

type Scanner struct {
	store      *store.Store
	releaseTag string
	interval   time.Duration
	mu         sync.Mutex // serializes scans
}

func New(s *store.Store, releaseTag string, interval time.Duration) *Scanner {
	return &Scanner{store: s, releaseTag: releaseTag, interval: interval}
}

// Run scans every bucket immediately and then on every tick until ctx is done.
func (s *Scanner) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		s.ScanAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ScanAll scans every archive backed by a bucket. Failures are recorded on
// the archive and leave what was found before untouched.
func (s *Scanner) ScanAll(ctx context.Context) {
	disks, err := s.store.BucketDisks(ctx)
	if err != nil {
		slog.Error("lettura archivi S3", "err", err)
		return
	}
	for _, d := range disks {
		if _, err := s.Scan(ctx, d); err != nil && ctx.Err() == nil {
			slog.Warn("bucket non leggibile", "disk", d.Label, "bucket", d.S3.Bucket, "err", err)
		}
	}
}

// Scan lists the disk's bucket and records the torrents found there.
func (s *Scanner) Scan(ctx context.Context, d store.Disk) (store.BucketResult, error) {
	if d.S3 == nil {
		return store.BucketResult{}, fmt.Errorf("%s non è collegato a un bucket", d.Label)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.scan(ctx, d)
	if err != nil && !errors.Is(err, store.ErrBucketChanged) && !errors.Is(err, store.ErrNotFound) {
		if serr := s.store.SetBucketError(context.WithoutCancel(ctx), d.ID, err); serr != nil {
			slog.Error("salvataggio errore bucket", "err", serr)
		}
	}
	return res, err
}

func (s *Scanner) scan(ctx context.Context, d store.Disk) (store.BucketResult, error) {
	c, err := Client(d.S3)
	if err != nil {
		return store.BucketResult{}, err
	}
	lctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	objs, err := c.List(lctx, d.S3.Prefix)
	if err != nil {
		return store.BucketResult{}, err
	}
	names, err := s.store.TorrentNames(ctx)
	if err != nil {
		return store.BucketResult{}, err
	}
	scan := Match(d.S3.Bucket, d.S3.Prefix, objs, names)
	res, err := s.store.ApplyBucketScan(ctx, d, scan, s.releaseTag)
	if err != nil {
		return res, err
	}
	slog.Info("bucket letto", "disk", d.Label, "bucket", d.S3.Bucket, "objects", scan.Objects,
		"torrents", len(scan.Matches), "unmatched", len(scan.Unmatched), "added", res.Added, "removed", res.Removed, "alerts", res.Alerts)
	return res, nil
}

// Client returns a read-only client for the bucket.
func Client(b *store.DiskS3) (*s3.Client, error) {
	return s3.New(s3.Config{Endpoint: b.Endpoint, Region: b.Region, Bucket: b.Bucket, AccessKey: b.AccessKey, SecretKey: b.SecretKey})
}

// NormalizePrefix turns "/torrent" or "torrent/" into "torrent/", the folder
// whose content is scanned; "" is the whole bucket.
func NormalizePrefix(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

type node struct {
	name     string
	path     string // full key, folders end with "/"
	size     int64
	files    int
	modified time.Time
	children map[string]*node
	contains *bool // a torrent was found in this subtree
}

func (n *node) child(name, path string) *node {
	if c, ok := n.children[name]; ok {
		return c
	}
	c := &node{name: name, path: path, children: map[string]*node{}}
	n.children[name] = c
	return c
}

func (n *node) sorted() []*node {
	out := make([]*node, 0, len(n.children))
	for _, c := range n.children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// Match finds the torrents stored under prefix. A torrent is a folder or a
// file with the torrent's name (as qBittorrent names its content), at any
// depth: Film/Nome.Release/ is found as well as Nome.Release/. Whatever
// holds no torrent is returned as unmatched, as high up as possible: a
// folder with no torrent inside is one entry, not a list of its files.
func Match(bucketName, prefix string, objs []s3.Object, names map[string][]string) store.BucketScan {
	root := &node{children: map[string]*node{}}
	var scan store.BucketScan
	for _, o := range objs {
		scan.Objects++
		scan.Size += o.Size
		rel := strings.TrimPrefix(o.Key, prefix)
		if rel == "" || !strings.HasPrefix(o.Key, prefix) {
			continue
		}
		parts := strings.Split(rel, "/")
		n := root
		path := prefix
		// Size, file count and date go to the object and every folder above it.
		file := !strings.HasSuffix(o.Key, "/")
		for i, p := range parts {
			if p == "" {
				continue // folder marker ("Nome/") or double slash
			}
			if i == len(parts)-1 {
				n = n.child(p, path+p)
			} else {
				path += p + "/"
				n = n.child(p, path)
			}
			n.size += o.Size
			if file {
				n.files++
			}
			if o.LastModified.After(n.modified) {
				n.modified = o.LastModified
			}
		}
	}

	hashes := func(n *node) []string { return names[strings.ToLower(strings.TrimSpace(n.name))] }
	var contains func(n *node) bool
	contains = func(n *node) bool {
		if n.contains != nil {
			return *n.contains
		}
		v := len(hashes(n)) > 0
		for _, c := range n.children {
			if contains(c) {
				v = true
			}
		}
		n.contains = &v
		return v
	}
	var walk func(n *node)
	walk = func(n *node) {
		if hs := hashes(n); len(hs) > 0 {
			for _, h := range hs {
				scan.Matches = append(scan.Matches, store.BucketMatch{Hash: h, Path: "s3://" + bucketName + "/" + n.path})
			}
			return
		}
		if !contains(n) {
			e := store.BucketEntry{Path: n.path, Name: n.name, Size: n.size, Files: n.files}
			if !n.modified.IsZero() {
				m := n.modified.UTC().Format(time.RFC3339)
				e.ModifiedAt = &m
			}
			scan.Unmatched = append(scan.Unmatched, e)
			return
		}
		for _, c := range n.sorted() {
			walk(c)
		}
	}
	for _, c := range root.sorted() {
		walk(c)
	}
	return scan
}
