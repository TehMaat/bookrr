package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The "GET Bucket (List Objects)" example of the AWS Signature Version 4 documentation.
func TestSignMatchesAWSExample(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/?prefix=J&max-keys=2", nil)
	sign(req, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1",
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization\n got %s\nwant %s", got, want)
	}
	if req.URL.RawQuery != "max-keys=2&prefix=J" {
		t.Fatalf("query %q", req.URL.RawQuery)
	}
}

func TestCanonicalQueryEncoding(t *testing.T) {
	q := map[string][]string{"prefix": {"Film 2024/à+b"}, "list-type": {"2"}}
	if got, want := canonicalQuery(q), "list-type=2&prefix=Film%202024%2F%C3%A0%2Bb"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestListFollowsPages(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s: the client must only read", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AK/") {
			t.Errorf("unsigned request: %q", r.Header.Get("Authorization"))
		}
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Query().Get("continuation-token") == "" {
			fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>tok/1</NextContinuationToken>
<Contents><Key>torrent/A/a.mkv</Key><Size>10</Size><LastModified>2024-01-02T03:04:05.000Z</LastModified></Contents></ListBucketResult>`)
			return
		}
		fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated>
<Contents><Key>torrent/B.mkv</Key><Size>20</Size></Contents></ListBucketResult>`)
	}))
	defer srv.Close()

	c, err := New(Config{Endpoint: srv.URL + "/", Region: "fr-par", Bucket: "archivio", AccessKey: "AK", SecretKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	objs, err := c.List(context.Background(), "torrent/")
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 || objs[0].Key != "torrent/A/a.mkv" || objs[1].Size != 20 || objs[0].LastModified.Year() != 2024 {
		t.Fatalf("objects %+v", objs)
	}
	want := []string{
		"/archivio?list-type=2&max-keys=1000&prefix=torrent%2F",
		"/archivio?continuation-token=tok%2F1&list-type=2&max-keys=1000&prefix=torrent%2F",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests\n%s", strings.Join(paths, "\n"))
	}
}

func TestListErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)
	}))
	defer srv.Close()
	c, _ := New(Config{Endpoint: srv.URL, Bucket: "b"})
	if _, err := c.List(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "accesso negato") {
		t.Fatalf("err %v", err)
	}
}
