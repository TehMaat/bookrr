package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiskBucketSecretIsWriteOnly(t *testing.T) {
	ctx := context.Background()
	st, h := newServer(t)

	var created map[string]any
	in := map[string]any{"label": "Scaleway", "s3": map[string]any{
		"endpoint": "s3.fr-par.scw.cloud", "region": "fr-par", "bucket": "archivio", "prefix": "/torrent", "accessKey": "AK", "secretKey": "SK",
	}}
	if code := call(t, h, "POST", "/api/disks", in, &created); code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, created)
	}
	b := created["s3"].(map[string]any)
	if created["kind"] != "Cloud" || b["endpoint"] != "https://s3.fr-par.scw.cloud" || b["prefix"] != "torrent/" || b["hasSecret"] != true {
		t.Fatalf("created %v", created)
	}
	if _, ok := b["secretKey"]; ok {
		t.Fatal("the secret key must never be sent back")
	}

	// Editing without retyping the secret keeps it.
	id := int64(created["id"].(float64))
	delete(in["s3"].(map[string]any), "secretKey")
	in["notes"] = "bucket in sola lettura"
	if code := call(t, h, "PUT", fmt.Sprintf("/api/disks/%d", id), in, nil); code != 200 {
		t.Fatalf("update: %d", code)
	}
	d, err := st.GetDisk(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.S3 == nil || d.S3.SecretKey != "SK" || d.Notes != "bucket in sola lettura" {
		t.Fatalf("after update: %+v %+v", d, d.S3)
	}

	// No bucket: the archive is a plain disk again.
	in["s3"] = nil
	call(t, h, "PUT", fmt.Sprintf("/api/disks/%d", id), in, nil)
	if d, _ = st.GetDisk(ctx, id); d.S3 != nil {
		t.Fatalf("bucket should be removed: %+v", d.S3)
	}

	var bad map[string]string
	in["s3"] = map[string]any{"endpoint": "https://s3.fr-par.scw.cloud", "bucket": "archivio/torrent", "accessKey": "AK", "secretKey": "SK"}
	if code := call(t, h, "POST", "/api/disks", in, &bad); code != http.StatusBadRequest || !strings.Contains(bad["error"], "prefisso") {
		t.Fatalf("bucket with a folder: %d %v", code, bad)
	}
}

func TestTestS3UsesStoredSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/archivio" || r.URL.Query().Get("prefix") != "torrent/" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>torrent/a</Key><Size>1</Size></Contents></ListBucketResult>`)
	}))
	defer srv.Close()

	_, h := newServer(t)
	var created struct {
		ID int64 `json:"id"`
	}
	call(t, h, "POST", "/api/disks", map[string]any{"label": "Bucket", "s3": map[string]any{
		"endpoint": srv.URL, "bucket": "archivio", "prefix": "torrent", "accessKey": "AK", "secretKey": "SK",
	}}, &created)

	var res map[string]any
	call(t, h, "POST", "/api/disks/test-s3", map[string]any{"id": created.ID, "endpoint": srv.URL, "bucket": "archivio", "prefix": "torrent", "accessKey": "AK"}, &res)
	if res["ok"] != true || res["objects"] != float64(1) {
		t.Fatalf("test: %v", res)
	}
	call(t, h, "POST", "/api/disks/test-s3", map[string]any{"endpoint": srv.URL, "bucket": "archivio", "accessKey": "AK"}, &res)
	if res["ok"] != false {
		t.Fatalf("a new archive needs the secret key: %v", res)
	}
}
