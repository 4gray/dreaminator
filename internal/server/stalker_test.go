package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"dreaminator/internal/app"
	"dreaminator/internal/stalker/stalkertest"
)

func TestStalkerResolverRedirects(t *testing.T) {
	portal := httptest.NewServer(&stalkertest.Portal{
		Path:     "/stalker_portal/server/load.php",
		Channels: []stalkertest.Channel{{ID: "7", Name: "Мир 24", Genre: "Новости", Stream: "http://cdn.example/mir24.ts"}},
	})
	defer portal.Close()

	dir := t.TempDir()
	defaults := app.DefaultSettings()
	defaults.WebifURL = ""
	a, err := app.New(app.Config{DataDir: filepath.Join(dir, "d"), E2Dir: filepath.Join(dir, "e2"),
		EPGImportDir: filepath.Join(dir, "epg"), Port: "8088"}, defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	src, err := a.AddSource(app.SourceInput{Type: "stalker", Name: "Портал", Server: portal.URL + "/stalker_portal/c/", MAC: "00:1A:79:AA:BB:CC"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RefreshSource(context.Background(), src.ID); err != nil {
		t.Fatal(err)
	}

	h := New(a, fstest.MapFS{})
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := noFollow.Get(srv.URL + "/stalker/" + src.ID + "/7")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "http://cdn.example/mir24.ts?play_token=") {
		t.Fatalf("got %d → %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	resp, _ = noFollow.Get(srv.URL + "/stalker/" + src.ID + "/nope")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown channel: %d", resp.StatusCode)
	}
}
