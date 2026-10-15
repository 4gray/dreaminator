package server

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"dreaminator/internal/app"
	"dreaminator/internal/stalker/stalkertest"
	"dreaminator/internal/tmdb/tmdbtest"
)

const tmdbKey = "SECRETkey0123456789SECRETkey0123"

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// tmdbHarness records every response body so a test can prove the key never
// left the receiver.
type tmdbHarness struct {
	t      *testing.T
	h      http.Handler
	app    *app.App
	cfg    app.Config
	logs   *lockedBuffer
	bodies []string
}

func newTMDBHarness(t *testing.T, fake *tmdbtest.Server) *tmdbHarness {
	t.Helper()
	dir := t.TempDir()
	cfg := app.Config{DataDir: filepath.Join(dir, "data"), E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg"),
		TMDBAPIBase: fake.APIBase, TMDBImageBase: fake.ImageBase}
	th := &tmdbHarness{t: t, cfg: cfg, logs: &lockedBuffer{}}
	th.start()
	return th
}

func (th *tmdbHarness) start() *app.App {
	defaults := app.DefaultSettings()
	defaults.WebifURL = ""
	a, err := app.New(th.cfg, defaults, log.New(th.logs, "", 0))
	if err != nil {
		th.t.Fatal(err)
	}
	th.h, th.app = New(a, fstest.MapFS{}), a
	return a
}

// settle waits for the refresh a new source starts in the background.
func (th *tmdbHarness) settle(id string) {
	th.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range th.app.View().Sources {
			if s.ID == id && !s.Refreshing && (s.UpdatedAt != nil || s.Error != "") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	th.t.Fatal("source refresh did not finish")
}

func (th *tmdbHarness) do(method, path string, body any) (int, map[string]any) {
	th.t.Helper()
	var rd *strings.Reader
	if s, ok := body.(string); ok {
		rd = strings.NewReader(s)
	} else if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	w := httptest.NewRecorder()
	th.h.ServeHTTP(w, httptest.NewRequest(method, path, rd))
	th.bodies = append(th.bodies, w.Body.String())
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (th *tmdbHarness) assertNoKey() {
	th.t.Helper()
	for i, b := range append(th.bodies, th.logs.String()) {
		if strings.Contains(b, tmdbKey) || strings.Contains(b, tmdbKey[:12]) || strings.Contains(b, "api_key") || strings.Contains(b, "apiKey\":\"S") {
			th.t.Fatalf("reply %d leaks the TMDB key: %s", i, b)
		}
	}
}

func TestTMDBSettingsRoundTrip(t *testing.T) {
	fake := tmdbtest.New(tmdbKey)
	defer fake.Close()
	th := newTMDBHarness(t, fake)
	defer th.assertNoKey()

	if code, st := th.do("GET", "/api/tmdb", nil); code != 200 || st["keySet"] != false || st["language"] != "ru-RU" {
		t.Fatalf("defaults: %d %v", code, st)
	}
	if code, _ := th.do("PUT", "/api/tmdb", map[string]any{"apiKey": "not a key"}); code != 400 {
		t.Fatalf("bad key: %d", code)
	}
	code, st := th.do("PUT", "/api/tmdb", map[string]any{"apiKey": tmdbKey, "language": "en-US"})
	if code != 200 || st["keySet"] != true || st["keyHint"] != "••••0123" || st["enabled"] != true || st["language"] != "en-US" {
		t.Fatalf("save key: %d %v", code, st)
	}

	// The old WebUI sends the settings it got from /api/state, wholesale.
	_, state := th.do("GET", "/api/state", nil)
	if _, ok := state["tmdb"]; ok {
		t.Fatal("state exposes the TMDB block")
	}
	if code, _ := th.do("PUT", "/api/settings", state["settings"]); code != 200 {
		t.Fatalf("settings: %d", code)
	}
	if code, _ := th.do("PUT", "/api/settings", map[string]any{"serviceType": 4097}); code != 200 {
		t.Fatalf("minimal settings: %d", code)
	}
	if _, st := th.do("GET", "/api/tmdb", nil); st["keySet"] != true || st["language"] != "en-US" {
		t.Fatalf("settings wiped TMDB: %v", st)
	}
	th.do("GET", "/api/summary", nil)

	// The key survives a restart, and only state.json holds it.
	th.start()
	if _, st := th.do("GET", "/api/tmdb", nil); st["keySet"] != true {
		t.Fatalf("after restart: %v", st)
	}
	saved, _ := os.ReadFile(filepath.Join(th.cfg.DataDir, "state.json"))
	if !strings.Contains(string(saved), tmdbKey) {
		t.Fatal("key not persisted")
	}

	if _, res := th.do("POST", "/api/tmdb/test", nil); res["ok"] != true {
		t.Fatalf("stored key test: %v", res)
	}
	_, res := th.do("POST", "/api/tmdb/test", map[string]any{"apiKey": "WRONGkey0123456789WRONGkey012345"})
	if res["ok"] != false || !strings.Contains(res["error"].(string), "Ключ не подошёл") {
		t.Fatalf("wrong key test: %v", res)
	}
	if _, st := th.do("PUT", "/api/tmdb", map[string]any{"clearKey": true}); st["keySet"] != false || st["keyHint"] != nil {
		t.Fatalf("clear: %v", st)
	}
	if code, _ := th.do("GET", "/api/tmdb/search?q=matrix&type=movie", nil); code != http.StatusConflict {
		t.Fatalf("search without a key: %d", code)
	}
}

func TestTMDBMatchRoutes(t *testing.T) {
	fake := tmdbtest.New(tmdbKey)
	defer fake.Close()
	fake.AddMovie(tmdbtest.Record{ID: 200, Title: "Демо фильм", Original: "Demo Film", Date: "2025-01-01", Poster: "/p200.jpg"})
	fake.AddMovie(tmdbtest.Record{ID: 201, Title: "Демо фильм 2", Original: "Demo Film 2", Date: "2026-01-01"})
	portal := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Catalog: true})
	defer portal.Close()
	th := newTMDBHarness(t, fake)
	defer th.assertNoKey()

	code, src := th.do("POST", "/api/sources", map[string]any{"type": "stalker", "name": "Портал", "server": portal.URL + "/c/", "mac": "00:1A:79:AA:BB:CC"})
	if code != 200 {
		t.Fatalf("source: %d %v", code, src)
	}
	id := src["id"].(string)
	th.settle(id)
	code, page := th.do("GET", "/api/catalog/"+id+"/items?type=vod&category=1&page=1", nil)
	if code != 200 {
		t.Fatalf("catalog: %d %v", code, page)
	}
	key := page["items"].([]any)[0].(map[string]any)["key"].(string)
	q := "source=" + url.QueryEscape(id) + "&key=" + url.QueryEscape(key)

	if _, v := th.do("GET", "/api/tmdb/match?"+q, nil); v["reason"] != "disabled" {
		t.Fatalf("disabled: %v", v)
	}
	th.do("PUT", "/api/tmdb", map[string]any{"apiKey": tmdbKey})

	code, v := th.do("GET", "/api/tmdb/match?"+q, nil)
	if code != 200 || v["reason"] != "matched" || v["current"].(map[string]any)["id"] != float64(200) {
		t.Fatalf("match view: %d %v", code, v)
	}
	item := v["item"].(map[string]any)
	cands := v["candidates"].([]any)
	if item["name"] != "Демо фильм" || len(cands) != 2 || cands[0].(map[string]any)["poster"] != fake.ImageBase+"/w185/p200.jpg" {
		t.Fatalf("view details: %v", v)
	}
	code, info := th.do("PUT", "/api/tmdb/match", map[string]any{"source": id, "key": key, "id": 201, "type": "movie"})
	if code != 200 || info["id"] != float64(201) || info["match"] != "manual" {
		t.Fatalf("set: %d %v", code, info)
	}
	if code, _ := th.do("PUT", "/api/tmdb/match", map[string]any{"source": id, "key": key, "id": 0, "type": "movie"}); code != 400 {
		t.Fatalf("bad choice: %d", code)
	}
	if _, v := th.do("GET", "/api/tmdb/match?"+q, nil); v["manual"] != true || v["current"].(map[string]any)["id"] != float64(201) {
		t.Fatalf("after set: %v", v)
	}
	if code, _ := th.do("DELETE", "/api/tmdb/match?"+q, nil); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if _, v := th.do("GET", "/api/tmdb/match?"+q, nil); v["manual"] != false || v["current"].(map[string]any)["id"] != float64(200) {
		t.Fatalf("after reset: %v", v)
	}
	if code, res := th.do("GET", "/api/tmdb/search?q=demo&type=movie&year=2025", nil); code != 200 || len(res["candidates"].([]any)) != 2 {
		t.Fatalf("search: %d %v", code, res)
	}

	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/tmdb/match?source=missing&key=x", 404},
		{"/api/tmdb/match?source=" + id + "&key=stale", 410},
		{"/api/tmdb/match?source=" + id, 400},
		{"/api/tmdb/search?q=x&type=anime", 400},
	} {
		if code, _ := th.do("GET", tc.path, nil); code != tc.status {
			t.Errorf("%s: %d, want %d", tc.path, code, tc.status)
		}
	}
	if code, _ := th.do("DELETE", "/api/tmdb/match?source="+id+"&key=stale", nil); code != 410 {
		t.Errorf("reset with a stale key: %d", code)
	}

	// TMDB down: the match screen says so, browsing keeps working.
	fake.Set(func(s *tmdbtest.Server) { s.Status = http.StatusServiceUnavailable })
	th.do("PUT", "/api/tmdb", map[string]any{"language": "de-DE"}) // drop the caches
	if code, v := th.do("GET", "/api/tmdb/match?"+q, nil); code != 200 || v["reason"] != "error" || v["error"] == "" {
		t.Fatalf("TMDB down: %d %v", code, v)
	}
	if code, _ := th.do("GET", "/api/tmdb/search?q=demo&type=movie", nil); code != http.StatusBadGateway {
		t.Fatalf("search while TMDB is down: %d", code)
	}
	if code, _ := th.do("GET", "/api/catalog/"+id+"/details?key="+url.QueryEscape(key), nil); code != 200 {
		t.Fatalf("catalog broken by TMDB: %d", code)
	}
}
