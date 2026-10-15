package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"dreaminator/internal/app"
)

func TestLibraryRoutes(t *testing.T) {
	dir := t.TempDir()
	defaults := app.DefaultSettings()
	defaults.WebifURL = ""
	a, err := app.New(app.Config{DataDir: filepath.Join(dir, "data"), E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg")},
		defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "e2"), 0o755)
	m3u := "#EXTM3U\n#EXTINF:-1 group-title=\"Новости\",Мир 24\nhttp://stream.example/mir.ts\n#EXTINF:-1,Спорт\nhttp://stream.example/sport.ts\n"
	src, err := a.AddUploadedSource("Плейлист", "", "tv.m3u", strings.NewReader(m3u))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Wait() // the guide and picons that /api/apply starts write into dir
	h := New(a, fstest.MapFS{})
	call := func(method, path string, body any, out any) int {
		t.Helper()
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(data)))
		if out != nil {
			if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
				t.Fatalf("%s %s: %v %s", method, path, err, w.Body)
			}
		}
		return w.Code
	}
	var page app.CatalogCards
	call("GET", "/api/catalog/"+src.ID+"/items?type=itv&page=1", nil, &page)
	if len(page.Items) != 2 || !strings.HasPrefix(page.Items[0].ID, src.ID+"/itv/") || page.Items[0].Fav {
		t.Fatalf("rows %+v", page.Items)
	}
	mir := page.Items[0]
	var fav app.FavoriteResult
	if code := call("POST", "/api/catalog/favorites", map[string]string{"id": mir.ID}, &fav); code != 200 || !fav.Fav || !fav.Channel || fav.List != "Избранное" {
		t.Fatalf("add %d %+v", code, fav)
	}
	call("GET", "/api/catalog/"+src.ID+"/items?type=itv&page=1", nil, &page)
	if !page.Items[0].Fav || page.Items[1].Fav {
		t.Fatalf("star after adding %+v", page.Items)
	}
	var found app.CatalogSearchResult
	call("GET", "/api/catalog/"+src.ID+"/search?type=itv&q=мир", nil, &found)
	if len(found.Items) != 1 || !found.Items[0].Fav {
		t.Fatalf("search rows %+v", found.Items)
	}
	var home app.CatalogHome
	call("GET", "/api/catalog/home", nil, &home)
	if home.Favorites[0].Count != 1 || home.Favorites[1].Count != 0 || home.Recent[0].Count != 0 {
		t.Fatalf("home %+v", home)
	}
	var rows app.CatalogCards
	call("GET", "/api/catalog/home/items?list=favorites&category=itv&page=1", nil, &rows)
	if len(rows.Items) != 1 || rows.Items[0].Source != src.ID || rows.Items[0].Key != mir.Key || rows.Items[0].Name != "Мир 24" {
		t.Fatalf("favorite rows %+v", rows.Items)
	}
	// Playback by id; the started channel becomes a recent.
	var play app.CatalogPlayback
	if code := call("POST", "/api/catalog/"+src.ID+"/play", map[string]string{"id": mir.ID}, &play); code != 200 || play.URL != "http://stream.example/mir.ts" {
		t.Fatalf("play by id %d %+v", code, play)
	}
	call("GET", "/api/catalog/home/items?list=recent&category=itv", nil, &rows)
	if len(rows.Items) != 1 || !rows.Items[0].Fav {
		t.Fatalf("recent rows %+v", rows.Items)
	}
	// What the TV sends on close: the channel list "Избранное" reaches Enigma2.
	if code := call("POST", "/api/apply", nil, nil); code != 200 {
		t.Fatalf("apply %d", code)
	}
	bouquets, _ := filepath.Glob(filepath.Join(dir, "e2", "userbouquet.*"))
	sawFavorites := false
	for _, b := range bouquets {
		data, _ := os.ReadFile(b)
		if strings.Contains(string(data), "#NAME Избранное") && strings.Contains(string(data), "stream.example/mir.ts") {
			sawFavorites = true
		}
	}
	if !sawFavorites {
		t.Fatalf("favorites not in the receiver's bouquets: %v", bouquets)
	}
	if code := call("DELETE", "/api/catalog/favorites?id="+mir.ID, nil, &fav); code != 200 || fav.Fav {
		t.Fatalf("remove %d %+v", code, fav)
	}
	if code := call("POST", "/api/catalog/close", nil, nil); code != 200 {
		t.Fatalf("close %d", code)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "data", "library.json")); err != nil || !strings.Contains(string(data), mir.ID) {
		t.Fatalf("library on close: %s %v", data, err)
	}
	for _, tc := range []struct {
		method, path string
		body         any
		status       int
	}{
		{"GET", "/api/catalog/home/items?list=everything&category=itv", nil, 400},
		{"GET", "/api/catalog/home/items?list=recent&category=itv&page=x", nil, 400},
		{"POST", "/api/catalog/favorites", map[string]string{}, 400},
		{"POST", "/api/catalog/favorites", map[string]string{"id": "gone/itv/1"}, 404},
		{"POST", "/api/catalog/" + src.ID + "/play", map[string]string{}, 400},
		{"POST", "/api/catalog/" + src.ID + "/play", map[string]string{"id": src.ID + "/itv/nope"}, 410},
	} {
		if code := call(tc.method, tc.path, tc.body, nil); code != tc.status {
			t.Errorf("%s %s: %d", tc.method, tc.path, code)
		}
	}
}
