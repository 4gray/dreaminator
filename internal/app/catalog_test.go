package app

import (
	"context"
	"dreaminator/internal/stalker/stalkertest"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
)

func catalogTestApp(t *testing.T) *App {
	t.Helper()
	a, err := New(Config{DataDir: t.TempDir()}, DefaultSettings(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	a.st.Sources = []*Source{{ID: "portal", Name: "Portal", Type: "stalker", Server: "http://provider.example", MAC: "00:1A:79:AA:BB:CC", Password: "secret"}, {ID: "xtream", Type: "xtream"}}
	return a
}

func TestCatalogSourcesIndependentOfBouquets(t *testing.T) {
	a := catalogTestApp(t)
	b, _ := json.Marshal(a.CatalogSources())
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "MAC") || strings.Contains(string(b), "provider.example") {
		t.Fatalf("unsafe sources: %s", b)
	}
	want := `[{"id":"portal","name":"Portal","type":"stalker","sections":["itv","vod","series"]},{"id":"xtream","name":"","type":"xtream","sections":["itv"]}]`
	if string(b) != want {
		t.Fatalf("sources: %s", b)
	}
	if len(a.st.Bouquets[0].Channels) != 0 {
		t.Fatal("catalog must not select bouquet channels")
	}
}
func TestCatalogLivePaginationAndIsolation(t *testing.T) {
	a := catalogTestApp(t)
	for n := 0; n < 620; n++ {
		a.channels["portal"] = append(a.channels["portal"], playlist.Channel{ID: fmt.Sprint(n), Name: fmt.Sprintf("Channel %d", n), Group: "News", Cmd: "ffmpeg http://example.com/live.ts"})
	}
	seen := map[string]bool{}
	for n := 1; ; n++ {
		p, err := a.CatalogItems(context.Background(), "portal", "itv", "News", "", n, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range p.Items {
			if seen[item.Key] {
				t.Fatal("duplicate identity")
			}
			seen[item.Key] = true
		}
		if !p.HasMore {
			break
		}
		if n > 100 {
			t.Fatal("endless pages")
		}
	}
	if len(seen) != 620 {
		t.Fatalf("got %d channels", len(seen))
	}
	if _, err := a.CatalogItems(context.Background(), "xtream", "vod", "", "", 1, false); err != stalker.ErrCatalogUnsupported {
		t.Fatalf("playlist films: %v", err)
	}
	if _, err := a.CatalogItems(context.Background(), "gone", "itv", "", "", 1, false); err != ErrNotFound {
		t.Fatalf("unknown source: %v", err)
	}
	if _, err := a.CatalogItems(context.Background(), "portal", "vod", "", "foreign-key", 1, false); err == nil {
		t.Fatal("accepted foreign parent")
	}
}
func TestCatalogSourceChangeInvalidatesItems(t *testing.T) {
	a := catalogTestApp(t)
	a.channels["portal"] = []playlist.Channel{{ID: "1", Name: "One", Group: "News"}}
	p, err := a.CatalogItems(context.Background(), "portal", "itv", "", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.st.Sources[0].MAC = "00:1A:79:DD:EE:FF"
	a.mu.Unlock()
	if _, err := a.CatalogPlay(context.Background(), "portal", p.Items[0].Key); err == nil {
		t.Fatal("stale item allowed after credential change")
	}
}

func TestCatalogVODOnlySourceAndThreeSeriesModes(t *testing.T) {
	portal := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Catalog: true, PageSize: 2, TokenUses: 3})
	defer portal.Close()
	a := catalogTestApp(t)
	a.st.Sources[0].Server = portal.URL + "/c/"
	ctx := context.Background()
	if err := a.RefreshSource(ctx, "portal"); err != nil {
		t.Fatalf("VOD-only refresh: %v", err)
	}
	films, err := a.CatalogItems(ctx, "portal", "vod", "1", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(films.Items) != 2 || !films.HasMore {
		t.Fatalf("first page %+v", films)
	}
	serialized, _ := json.Marshal(films)
	if strings.Contains(string(serialized), "file_film") || strings.Contains(string(serialized), "cmd") {
		t.Fatal("provider commands exposed")
	}
	film := films.Items[0]
	play, err := a.CatalogPlay(ctx, "portal", film.Key)
	if err != nil || !strings.Contains(play.URL, "film.mp4") {
		t.Fatalf("film %+v %v", play, err)
	}
	again, err := a.CatalogPlay(ctx, "portal", film.Key)
	if err != nil || again.URL == play.URL {
		t.Fatal("temporary URL reused")
	}
	poster, err := a.CatalogPoster(ctx, "portal", film.Key)
	if err != nil || len(poster.Data) == 0 || poster.ContentType != "image/png" {
		t.Fatalf("poster %v", err)
	}
	for _, tc := range []struct {
		section     string
		page, index int
		expected    string
	}{{"vod", 1, 1, "embedded-2"}, {"vod", 2, 0, "lazy-2-2"}, {"series", 1, 0, "regular-2-2"}} {
		roots, err := a.CatalogItems(ctx, "portal", tc.section, "1", "", tc.page, false)
		if err != nil {
			t.Fatal(err)
		}
		root := roots.Items[tc.index]
		seasons, err := a.CatalogItems(ctx, "portal", tc.section, "1", root.Key, 1, false)
		if err != nil || len(seasons.Items) == 0 {
			t.Fatalf("seasons %v", err)
		}
		season := seasons.Items[len(seasons.Items)-1]
		episodes, err := a.CatalogItems(ctx, "portal", tc.section, "1", season.Key, 1, false)
		if err != nil || len(episodes.Items) < 2 {
			t.Fatalf("episodes %+v %v", episodes, err)
		}
		played, err := a.CatalogPlay(ctx, "portal", episodes.Items[1].Key)
		if err != nil || !strings.Contains(played.URL, tc.expected) {
			t.Fatalf("episode %+v %v expected %s", played, err, tc.expected)
		}
	}
	if len(a.st.Bouquets[0].Channels) != 0 {
		t.Fatal("browsing selected bouquet channels")
	}
}
func TestCatalogDetectsRepeatedProviderPage(t *testing.T) {
	fixture := &stalkertest.Portal{Path: "/server/load.php", Catalog: true, PageSize: 2}
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("action") == "get_ordered_list" {
			q.Set("p", "1")
			r.URL.RawQuery = q.Encode()
		}
		fixture.ServeHTTP(w, r)
	}))
	defer portal.Close()
	a := catalogTestApp(t)
	a.st.Sources[0].Server = portal.URL + "/c/"
	ctx := context.Background()
	if _, err := a.CatalogItems(ctx, "portal", "vod", "1", "", 1, false); err != nil {
		t.Fatal(err)
	}
	repeated, err := a.CatalogItems(ctx, "portal", "vod", "1", "", 2, false)
	if err != nil || repeated.HasMore || len(repeated.Items) != 0 {
		t.Fatalf("repeated page %+v %v", repeated, err)
	}
}

func TestCatalogRefreshReplacesLivePagesAndGroups(t *testing.T) {
	portal := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Channels: []stalkertest.Channel{{ID: "new", Name: "New", Genre: "New group", Stream: "http://example.test/new.ts"}}})
	defer portal.Close()
	a := catalogTestApp(t)
	a.st.Sources[0].Server = portal.URL + "/c/"
	a.setChannels("portal", []playlist.Channel{{ID: "old", Name: "Old", Group: "Old group"}})
	old, err := a.CatalogItems(context.Background(), "portal", "itv", "", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := a.CatalogItems(context.Background(), "portal", "itv", "", "", 1, true)
	if err != nil || len(fresh.Items) != 1 || fresh.Items[0].Name != "New" {
		t.Fatalf("yellow did not fetch live channels: %+v %v", fresh, err)
	}
	if _, err := a.CatalogPlay(context.Background(), "portal", old.Items[0].Key); err == nil {
		t.Fatal("old channel still cached")
	}
	// A list rebuilt from the previous lineup must be invalidated at publication.
	a.setChannels("portal", []playlist.Channel{{ID: "newer", Name: "Newer", Group: "Latest group"}})
	newest, err := a.CatalogItems(context.Background(), "portal", "itv", "", "", 1, false)
	if err != nil || newest.Items[0].Name != "Newer" {
		t.Fatalf("published lineup did not invalidate: %+v %v", newest, err)
	}
}

func TestCatalogMetadataBudget(t *testing.T) {
	a := catalogTestApp(t)
	for n := 0; n < 4000; n++ {
		a.channels["portal"] = append(a.channels["portal"], playlist.Channel{ID: fmt.Sprint(n), Name: strings.Repeat("n", 2000), Group: "G"})
	}
	for page := 1; page <= 80; page++ {
		if _, err := a.CatalogItems(context.Background(), "portal", "itv", "", "", page, false); err != nil {
			t.Fatal(err)
		}
		c := a.catalogs["portal"]
		if c.itemBytes+c.pageBytes > 3500000 {
			for _, section := range []string{"vod", "series"} {
				if err := cacheCategories(c, section, []stalker.CatalogCategory{{ID: "1", Name: strings.Repeat("c", 500000)}}); err != nil {
					t.Fatal(err)
				}
			}
		}
		if c.itemBytes+c.pageBytes+c.categoryBytes > catalogMaxBytes || len(c.items) > catalogMaxItems {
			t.Fatal("unbounded catalog")
		}
	}
	c := a.catalogs["portal"]
	cats := []stalker.CatalogCategory{{ID: "1", Name: strings.Repeat("c", 500000)}}
	for _, section := range []string{"vod", "series"} {
		if err := cacheCategories(c, section, cats); err != nil {
			t.Fatal(err)
		}
	}
	if c.itemBytes+c.pageBytes+c.categoryBytes > catalogMaxBytes {
		t.Fatal("categories bypass metadata bound")
	}
}

func TestCatalogNavigationSurvivesLongPlayback(t *testing.T) {
	portal := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Catalog: true})
	defer portal.Close()
	a := catalogTestApp(t)
	a.st.Sources[0].Server = portal.URL + "/c/"
	ctx := context.Background()
	roots, err := a.CatalogItems(ctx, "portal", "vod", "1", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	parent := roots.Items[1].Key
	seasons, err := a.CatalogItems(ctx, "portal", "vod", "1", parent, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	season := seasons.Items[0].Key
	episodes, err := a.CatalogItems(ctx, "portal", "vod", "1", season, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	a.catalogs["portal"].created = time.Now().Add(-time.Hour)
	if _, err := a.CatalogPlay(ctx, "portal", episodes.Items[0].Key); err != nil {
		t.Fatalf("play after long viewing: %v", err)
	}
	if _, err := a.CatalogItems(ctx, "portal", "vod", "1", season, 1, false); err != nil {
		t.Fatalf("return to series after long viewing: %v", err)
	}
}

// Playlists browse as Live TV only: groups become categories, channels play
// their own address with the service type the bouquets use, logos are
// fetched without any portal credentials.
func TestCatalogPlaylistLiveOnly(t *testing.T) {
	var logoHeaders http.Header
	logos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logoHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "image/png")
		w.Write(encoded(t, 220, 132, false))
	}))
	defer logos.Close()
	a := catalogTestApp(t)
	a.st.Settings.ServiceType = 5002
	a.st.Sources = append(a.st.Sources, &Source{ID: "m3u", Name: "Cyrius", Type: "m3u", URL: "http://lists.example/tv.m3u"})
	chans := []playlist.Channel{
		{ID: "a", Name: "Первый", Group: "Эфир", Logo: logos.URL + "/1.png", URL: "http://stream.example/1.ts"},
		{ID: "b", Name: "Детский", Group: "Детские", URL: "http://stream.example/2.ts"},
		{ID: "c", Name: "Первый HD", Group: "Эфир", URL: "http://stream.example/3.ts"},
	}
	a.setChannels("m3u", chans)
	ctx := context.Background()
	cats, err := a.CatalogCategories(ctx, "m3u", "itv", false)
	if err != nil || len(cats) != 3 || cats[0].Name != "Все каналы" || cats[1].Name != "Детские" || cats[2].Name != "Эфир" {
		t.Fatalf("categories %+v %v", cats, err)
	}
	for _, section := range []string{"vod", "series"} {
		if _, err := a.CatalogCategories(ctx, "m3u", section, false); err != stalker.ErrCatalogUnsupported {
			t.Fatalf("%s categories: %v", section, err)
		}
	}
	p, err := a.CatalogItems(ctx, "m3u", "itv", "Эфир", "", 1, false)
	if err != nil || len(p.Items) != 2 || p.Items[0].Number != "1" || !p.Items[0].HasPoster || p.Items[1].HasPoster {
		t.Fatalf("items %+v %v", p, err)
	}
	play, err := a.CatalogPlay(ctx, "m3u", p.Items[0].Key)
	if err != nil || play.URL != "http://stream.example/1.ts" || play.ServiceType != 5002 || len(play.Headers) != 0 || play.Name != "Первый" {
		t.Fatalf("play %+v %v", play, err)
	}
	art, err := a.CatalogPoster(ctx, "m3u", p.Items[0].Key)
	if err != nil || art.ContentType != "image/png" {
		t.Fatalf("logo %v %v", art.ContentType, err)
	}
	if logoHeaders.Get("Authorization") != "" || logoHeaders.Get("Cookie") != "" || strings.Contains(logoHeaders.Get("User-Agent"), "MAG") {
		t.Fatalf("portal headers on a playlist logo: %v", logoHeaders)
	}
	res, err := a.CatalogSearch(ctx, "m3u", "itv", "перв", 1)
	if err != nil || res.Scope != "channels" || len(res.Items) != 2 {
		t.Fatalf("search %+v %v", res, err)
	}
	// A refreshed playlist replaces the catalog: old keys expire, the new
	// listing plays the new address. The channel just played is a recent:
	// library rows keep their keys, and it plays the new address too.
	moved := append([]playlist.Channel(nil), chans...)
	moved[0].URL = "http://stream.example/1-new.ts"
	a.setChannels("m3u", moved)
	if _, err := a.CatalogPlay(ctx, "m3u", p.Items[1].Key); err != ErrCatalogExpired {
		t.Fatalf("key after refresh: %v", err)
	}
	if play, err := a.CatalogPlay(ctx, "m3u", p.Items[0].Key); err != nil || play.URL != "http://stream.example/1-new.ts" {
		t.Fatalf("recent channel after refresh: %+v %v", play, err)
	}
	p, err = a.CatalogItems(ctx, "m3u", "itv", "Эфир", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if play, err := a.CatalogPlay(ctx, "m3u", p.Items[0].Key); err != nil || play.URL != "http://stream.example/1-new.ts" {
		t.Fatalf("moved stream %+v %v", play, err)
	}
	// Another playlist address is another catalog.
	key := p.Items[1].Key
	a.st.Sources[len(a.st.Sources)-1].URL = "http://other.example/tv.m3u"
	if _, err := a.CatalogPlay(ctx, "m3u", key); err != ErrCatalogExpired {
		t.Fatalf("old key after the playlist moved: %v", err)
	}
}
