package store

import (
	"context"
	"testing"
)

func TestBucketScanKeepsArchivesInSync(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustClient(t, s, "a")
	gone := SnapshotTorrent{Hash: "h1", Name: "Gone", Tags: tag}
	kept := SnapshotTorrent{Hash: "h2", Name: "Kept", Tags: tag}
	sync(t, s, map[Client][]SnapshotTorrent{a: {gone, kept}})
	// h1 leaves the client before it shows up in the bucket: alert.
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {kept}}); n != 1 {
		t.Fatalf("expected 1 alert, got %d", n)
	}

	b := &DiskS3{Endpoint: "https://s3.fr-par.scw.cloud", Region: "fr-par", Bucket: "archivio", Prefix: "torrent/", AccessKey: "AK", SecretKey: "SK"}
	disk, err := s.CreateDisk(ctx, Disk{Label: "Scaleway", Kind: "Cloud", S3: b})
	if err != nil {
		t.Fatal(err)
	}
	if disk.S3 == nil || !disk.S3.HasSecret || disk.S3.Bucket != "archivio" {
		t.Fatalf("bucket not saved: %+v", disk.S3)
	}

	scan := BucketScan{
		Matches:   []BucketMatch{{Hash: "h1", Path: "s3://archivio/torrent/Gone/"}, {Hash: "h2", Path: "s3://archivio/torrent/Kept/"}},
		Unmatched: []BucketEntry{{Path: "torrent/Altro/", Name: "Altro", Size: 5, Files: 1}},
		Objects:   3, Size: 30,
	}
	res, err := s.ApplyBucketScan(ctx, disk, scan, tag)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Removed != 0 {
		t.Fatalf("result %+v", res)
	}
	tr, _ := s.GetTorrent(ctx, "h1")
	if tr.Status != StatusArchived || len(tr.Archives) != 1 || tr.Archives[0].Source != ArchiveSourceS3 || tr.Archives[0].Path != "s3://archivio/torrent/Gone/" {
		t.Fatalf("h1 should be archived on the bucket: %+v", tr)
	}
	all, _ := s.ListAlerts(ctx, false, "h1")
	if len(all) != 1 || all[0].ResolvedAt == nil || all[0].Resolution != "Spostato su Scaleway · s3://archivio/torrent/Gone/" {
		t.Fatalf("alert should be resolved by the bucket: %+v", all)
	}
	// Already in the bucket when it leaves the client: no new alert.
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {}}); n != 0 {
		t.Fatalf("expected no alert, got %d", n)
	}
	disk, _ = s.GetDisk(ctx, disk.ID)
	if disk.ArchiveCount != 2 || disk.S3.Objects != 3 || disk.S3.Size != 30 || disk.S3.Unmatched != 1 || disk.S3.ScanAt == nil {
		t.Fatalf("disk after scan: %+v %+v", disk, disk.S3)
	}
	entries, _ := s.BucketEntries(ctx, disk.ID)
	if len(entries) != 1 || entries[0].Name != "Altro" {
		t.Fatalf("entries %+v", entries)
	}

	// Same scan again: nothing changes.
	if res, _ := s.ApplyBucketScan(ctx, disk, scan, tag); res != (BucketResult{}) {
		t.Fatalf("rescan %+v", res)
	}

	// h2 recorded by hand on the same disk replaces the automatic archive;
	// h1 is deleted from the bucket: it has nowhere to be, so an alert opens.
	if err := s.AddArchive(ctx, "h2", ArchiveInput{DiskID: &disk.ID, Path: "a mano"}); err != nil {
		t.Fatal(err)
	}
	res, err = s.ApplyBucketScan(ctx, disk, BucketScan{Matches: scan.Matches[1:]}, tag)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 || res.Alerts != 1 {
		t.Fatalf("result %+v", res)
	}
	tr, _ = s.GetTorrent(ctx, "h1")
	if tr.Status != StatusMissing || len(tr.Archives) != 0 || tr.OpenAlert.Message != "Rimosso da Scaleway" {
		t.Fatalf("h1 should be missing: %+v", tr)
	}
	tr, _ = s.GetTorrent(ctx, "h2")
	if len(tr.Archives) != 1 || tr.Archives[0].Source != "" {
		t.Fatalf("h2 should keep only the manual archive: %+v", tr.Archives)
	}

	// Back in the bucket: archived again and the alert closes.
	if _, err := s.ApplyBucketScan(ctx, disk, scan, tag); err != nil {
		t.Fatal(err)
	}
	if tr, _ = s.GetTorrent(ctx, "h1"); tr.Status != StatusArchived {
		t.Fatalf("h1 should be archived again: %+v", tr)
	}

	// A scan of the old bucket after the archive moved elsewhere is discarded.
	moved := disk
	moved.S3 = &DiskS3{Endpoint: b.Endpoint, Region: b.Region, Bucket: "altro", AccessKey: "AK", SecretKey: "SK"}
	if _, err := s.UpdateDisk(ctx, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyBucketScan(ctx, disk, scan, tag); err != ErrBucketChanged {
		t.Fatalf("expected ErrBucketChanged, got %v", err)
	}
	// Moving the archive forgets what was found in the old bucket, silently.
	tr, _ = s.GetTorrent(ctx, "h1")
	if len(tr.Archives) != 0 || tr.OpenAlert != nil {
		t.Fatalf("h1 after the bucket changed: %+v", tr)
	}
	disk, _ = s.GetDisk(ctx, disk.ID)
	if disk.S3.ScanAt != nil || disk.S3.Unmatched != 0 || disk.ArchiveCount != 1 {
		t.Fatalf("disk after the bucket changed: %+v %+v", disk, disk.S3)
	}

	// Deleting the disk drops the bucket archives and keeps the manual ones.
	if _, err := s.ApplyBucketScan(ctx, disk, scan, tag); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDisk(ctx, disk.ID); err != nil {
		t.Fatal(err)
	}
	tr, _ = s.GetTorrent(ctx, "h2")
	if len(tr.Archives) != 1 || tr.Archives[0].DiskID != nil || tr.Archives[0].Path != "a mano" {
		t.Fatalf("h2 after deleting the disk: %+v", tr.Archives)
	}
}

func TestBucketScanDropsDuplicateArchives(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	b := &DiskS3{Endpoint: "https://s3.fr-par.scw.cloud", Bucket: "archivio", AccessKey: "AK", SecretKey: "SK"}
	disk, _ := s.CreateDisk(ctx, Disk{Label: "Scaleway", Kind: "Cloud", S3: b})
	for _, h := range []string{"manual-1", "abc"} {
		if _, err := s.CreateManualTorrent(ctx, ManualInput{TorrentInput: TorrentInput{Hash: h, Name: "Nome"}}); err != nil {
			t.Fatal(err)
		}
	}
	scan := BucketScan{Matches: []BucketMatch{{Hash: "manual-1", Path: "s3://archivio/Nome/"}, {Hash: "abc", Path: "s3://archivio/Nome/"}}}
	if _, err := s.ApplyBucketScan(ctx, disk, scan, tag); err != nil {
		t.Fatal(err)
	}
	// The tracker tells that the placeholder is abc: both archives end up on abc.
	if _, err := s.ReplaceHash(ctx, "manual-1", TrackerTorrent{Hash: "abc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyBucketScan(ctx, disk, BucketScan{Matches: scan.Matches[1:]}, tag); err != nil {
		t.Fatal(err)
	}
	tr, _ := s.GetTorrent(ctx, "abc")
	if len(tr.Archives) != 1 {
		t.Fatalf("archives %+v", tr.Archives)
	}
}
