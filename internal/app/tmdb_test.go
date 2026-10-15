package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"dreaminator/internal/stalker"
	"dreaminator/internal/stalker/stalkertest"
	"dreaminator/internal/tmdb"
	"dreaminator/internal/tmdb/tmdbtest"
)

const testTMDBKey = "SECRETkey0123456789SECRETkey0123"

// syncBuffer collects log output written from lookup goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func picture(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 40, 60), color.Palette{color.Black, color.White})
	var b bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&b, img, nil)
	case "png":
		err = png.Encode(&b, img)
	case "gif":
		err = gif.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fakeTMDB knows a few films and shows, including the demo portal's catalog.
func fakeTMDB(t *testing.T) *tmdbtest.Server {
	t.Helper()
	s := tmdbtest.New(testTMDBKey)
	t.Cleanup(s.Close)
	s.AddMovie(tmdbtest.Record{ID: 603, Title: "Матрица", Original: "The Matrix", Date: "1999-03-30", Overview: "Хакер Нео узнаёт правду.", Runtime: 136, Genres: []string{"боевик", "фантастика"}, Vote: 8.24, Votes: 25000, Poster: "/p603.jpg", Backdrop: "/b603.png"})
	s.AddMovie(tmdbtest.Record{ID: 604, Title: "Матрица: Перезагрузка", Original: "The Matrix Reloaded", Date: "2003-05-15", Votes: 5, Vote: 7, Poster: "/huge.jpg"})
	s.AddMovie(tmdbtest.Record{ID: 275, Title: "Фарго", Original: "Fargo", Date: "1996-03-08", Poster: "/fake.jpg"})
	s.AddMovie(tmdbtest.Record{ID: 1001, Title: "Двойник", Original: "The Double", Date: "2013-09-06"})
	s.AddMovie(tmdbtest.Record{ID: 1002, Title: "Двойник", Original: "The Double", Date: "2013-11-01"})
	s.AddMovie(tmdbtest.Record{ID: 550, Title: "Бойцовский клуб", Original: "Fight Club", Date: "1999-10-15", Queries: []string{"Fight Club"}})
	s.AddMovie(tmdbtest.Record{ID: 200, Title: "Демо фильм", Original: "Demo Film", Date: "2025-01-01", Poster: "/p200.jpg"})
	s.AddShow(tmdbtest.Record{ID: 60622, Title: "Фарго", Original: "Fargo", Date: "2014-04-15", Runtime: 53})
	s.AddShow(tmdbtest.Record{ID: 9999, Title: "Американский заложник", Original: "American Hostage", Date: "2024-02-01"})
	s.AddShow(tmdbtest.Record{ID: 100, Title: "Обычный сериал", Original: "Regular Show", Date: "2020-01-01"})
	s.AddShow(tmdbtest.Record{ID: 101, Title: "Сериал с эпизодами", Original: "Embedded Show", Date: "2021-01-01"})
	s.AddShow(tmdbtest.Record{ID: 102, Title: "Сериал Ministra", Original: "Ministra Show", Date: "2022-01-01"})
	for _, id := range []int{9999, 100, 101, 102} {
		s.AddSeason(id, 1, tmdbtest.Episode{Number: 1, Name: "Пилот"}, tmdbtest.Episode{Number: 2, Name: "Вторая"}, tmdbtest.Episode{Number: 3, Name: "Третья"})
	}
	s.AddImage("/p603.jpg", picture(t, "jpeg"))
	s.AddImage("/b603.png", picture(t, "png"))
	s.AddImage("/fake.jpg", picture(t, "gif"))
	s.AddImage("/huge.jpg", append(picture(t, "jpeg"), make([]byte, tmdbImageMax)...))
	return s
}

func tmdbTestApp(t *testing.T, s *tmdbtest.Server, enable bool) (*App, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	a, err := New(Config{DataDir: t.TempDir(), TMDBAPIBase: s.APIBase, TMDBImageBase: s.ImageBase, Version: "test"}, DefaultSettings(), log.New(logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	a.st.Sources = []*Source{{ID: "portal", Name: "Portal", Type: "stalker", Server: "http://provider.example/c/", MAC: "00:1A:79:AA:BB:CC"}}
	if enable {
		key := testTMDBKey
		if _, err := a.UpdateTMDB(TMDBUpdate{APIKey: &key}); err != nil {
			t.Fatal(err)
		}
	}
	return a, logs
}

func vodMovie(id, name, original, year string) stalker.CatalogItem {
	return stalker.CatalogItem{Type: "vod", Kind: "movie", ID: id, Name: name, OriginalName: original, Year: year}
}

// browse puts entries into the portal's catalog cache, as viewing pages would.
func browse(t *testing.T, a *App, parent string, items ...stalker.CatalogItem) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	cache, _, err := a.catalogLocked("portal")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		cache.items[it.Key] = catalogRecord{item: it, parent: parent}
	}
}

func assertNoKey(t *testing.T, where, s string) {
	t.Helper()
	if strings.Contains(s, testTMDBKey) || strings.Contains(s, testTMDBKey[:12]) || strings.Contains(s, "api_key") {
		t.Fatalf("%s leaks the TMDB key: %s", where, s)
	}
}

func TestTMDBMatchesMovieByTitleAndYear(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	info, err := a.TMDBFor(ctx, "portal", vodMovie("m1", "The Matrix", "", "1999"))
	if err != nil || info == nil {
		t.Fatalf("no match: %v", err)
	}
	if info.ID != 603 || info.Type != "movie" || info.Match != "auto" || info.Title != "Матрица" || info.OriginalTitle != "The Matrix" ||
		info.Year != "1999" || info.Runtime != 136 || len(info.Genres) != 2 || info.Rating != "8.2" || info.Votes != 25000 || !info.HasPoster || !info.HasBackdrop {
		t.Fatalf("info %+v", info)
	}
	if s.Hits("search/movie") != 1 || s.Hits("movie") != 1 || s.Hits("search/tv") != 0 {
		t.Fatalf("requests: search %d details %d", s.Hits("search/movie"), s.Hits("movie"))
	}
	if langs := s.Languages(); langs[0] != "ru-RU" {
		t.Fatalf("default language: %v", langs)
	}
	again, _ := a.TMDBFor(ctx, "portal", vodMovie("m1", "The Matrix", "", "1999"))
	if again == nil || again.ID != 603 || s.Hits("") != 2 {
		t.Fatalf("a second look must come from the cache: %d requests", s.Hits(""))
	}
	again.Genres[0] = "changed"
	if third, _ := a.TMDBFor(ctx, "portal", vodMovie("m1", "The Matrix", "", "1999")); third.Genres[0] != "боевик" {
		t.Fatal("callers must not be able to change the cache")
	}

	if info, _ := a.TMDBFor(ctx, "portal", vodMovie("m2", "Матрица [HD]", "", "2000")); info == nil || info.ID != 603 {
		t.Fatalf("a year off by one: %+v", info)
	}
	if info, _ := a.TMDBFor(ctx, "portal", vodMovie("m3", "Reloaded", "", "2003")); info != nil {
		t.Fatalf("a partial title must not match: %+v", info)
	}
	// Votes below 10 give no rating.
	if info, _ := a.TMDBFor(ctx, "portal", vodMovie("m4", "Матрица: Перезагрузка", "", "2003")); info == nil || info.Rating != "" || info.Votes != 5 {
		t.Fatalf("few votes: %+v", info)
	}
}

func TestTMDBAmbiguousTitleStaysUndecorated(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	info, err := a.TMDBFor(context.Background(), "portal", vodMovie("d", "Двойник", "", "2013"))
	if info != nil || err != nil {
		t.Fatalf("two films with one title and year must stay unmatched: %+v %v", info, err)
	}
	n := s.Hits("")
	a.TMDBFor(context.Background(), "portal", vodMovie("d", "Двойник", "", "2013"))
	if s.Hits("") != n {
		t.Fatal("a negative answer must be cached")
	}
}

func TestTMDBOriginalTitleFallback(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	info, err := a.TMDBFor(context.Background(), "portal", vodMovie("fc", "Бойцовский клуб", "Fight Club", "1999"))
	if err != nil || info == nil || info.ID != 550 {
		t.Fatalf("original title: %+v %v", info, err)
	}
	if q := s.Queries(); len(q) != 2 || q[0] != "Бойцовский клуб" || q[1] != "Fight Club" {
		t.Fatalf("queries %q", q)
	}
}

func TestTMDBWorkTypeAndSeasonSuffix(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	film, _ := a.TMDBFor(ctx, "portal", vodMovie("fargo", "Fargo", "", "1996"))
	if film == nil || film.ID != 275 || film.Type != "movie" || s.Hits("search/tv") != 0 {
		t.Fatalf("film: %+v", film)
	}
	show, _ := a.TMDBFor(ctx, "portal", stalker.CatalogItem{Type: "series", Kind: "series", ID: "fargo", Name: "Фарго s01", Year: "2014"})
	if show == nil || show.ID != 60622 || show.Type != "tv" || show.Runtime != 53 {
		t.Fatalf("show: %+v", show)
	}
	// A series inside the VOD section is a show too.
	hostage, _ := a.TMDBFor(ctx, "portal", stalker.CatalogItem{Type: "vod", Kind: "series", ID: "ah", Name: "American Hostage s01", Year: "2024"})
	if hostage == nil || hostage.ID != 9999 {
		t.Fatalf("season suffix: %+v", hostage)
	}
	if q := s.Queries(); q[len(q)-1] != "American Hostage" {
		t.Fatalf("season suffix sent to TMDB: %q", q)
	}
	if info, _ := a.TMDBFor(ctx, "portal", stalker.CatalogItem{Type: "itv", Kind: "channel", ID: "1", Name: "Fargo"}); info != nil {
		t.Fatal("channels are never matched")
	}
}

func TestTMDBExplicitIDSkipsSearch(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	item := vodMovie("x", "Совсем другое название", "", "1980")
	item.TMDBID = "603"
	info, _ := a.TMDBFor(context.Background(), "portal", item)
	if info == nil || info.ID != 603 || info.Match != "explicit" || s.Hits("search/movie") != 0 || s.Hits("movie") != 1 {
		t.Fatalf("explicit id: %+v, %d searches", info, s.Hits("search/movie"))
	}
	stale := vodMovie("y", "The Matrix", "", "1999")
	stale.TMDBID = "777777"
	info, _ = a.TMDBFor(context.Background(), "portal", stale)
	if info == nil || info.ID != 603 || info.Match != "auto" {
		t.Fatalf("an unknown portal id falls back to search: %+v", info)
	}
}

func TestTMDBFailuresFallBackWithoutLeakingTheKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*tmdbtest.Server)
		want error
	}{
		{"401", func(s *tmdbtest.Server) { s.Key = "OTHERkey0123456789OTHERkey012345" }, tmdb.ErrUnauthorized},
		{"429", func(s *tmdbtest.Server) { s.Status, s.RetryAfter = http.StatusTooManyRequests, "60" }, tmdb.ErrRateLimited},
		{"500", func(s *tmdbtest.Server) { s.Status = http.StatusInternalServerError }, tmdb.ErrUnavailable},
		{"timeout", func(s *tmdbtest.Server) { s.Delay = time.Second }, tmdb.ErrTimeout},
		{"malformed", func(s *tmdbtest.Server) { s.Body = `{"results":[{"id":"x"` }, tmdb.ErrMalformed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fakeTMDB(t)
			s.Set(tc.set)
			a, logs := tmdbTestApp(t, s, true)
			a.tmdbSvc().timeout = 150 * time.Millisecond
			ctx := context.Background()
			item := vodMovie("m", "The Matrix", "", "1999")
			info, err := a.TMDBFor(ctx, "portal", item)
			if info != nil || !errors.Is(err, tc.want) {
				t.Fatalf("got %+v, %v; want nil and %v", info, err, tc.want)
			}
			assertNoKey(t, "error", err.Error())
			n := s.Hits("")
			if _, err := a.TMDBArtwork(ctx, "portal", item, "poster"); err == nil {
				t.Fatal("artwork without TMDB")
			}
			if eps, _ := a.TMDBEpisodes(ctx, "portal", stalker.CatalogItem{Type: "series", Kind: "series", ID: "s", Name: "Fargo"}, 1); len(eps) != 0 {
				t.Fatal("episodes without TMDB")
			}
			if tc.name != "malformed" && tc.name != "timeout" && s.Hits("") != n {
				t.Fatalf("%s must keep requests away for a while: %d more", tc.name, s.Hits("")-n)
			}
			res := a.TestTMDB(ctx, "")
			if res.OK || res.Error == "" {
				t.Fatalf("test result %+v", res)
			}
			view, _ := json.Marshal(a.View())
			summary, _ := json.Marshal(a.Summary())
			for where, text := range map[string]string{"log": logs.String(), "test": res.Error, "state": string(view), "summary": string(summary)} {
				assertNoKey(t, where, text)
			}
			if !strings.Contains(logs.String(), "TMDB") {
				t.Fatalf("failure not logged: %q", logs.String())
			}
		})
	}
}

func TestTMDBOffMakesNoRequests(t *testing.T) {
	s := fakeTMDB(t)
	for _, enable := range []bool{false, true} {
		a, _ := tmdbTestApp(t, s, enable)
		if enable {
			off := false
			a.UpdateTMDB(TMDBUpdate{Enabled: &off})
		}
		ctx := context.Background()
		item := vodMovie("m", "The Matrix", "", "1999")
		item.Key = "km"
		browse(t, a, "", item)
		if info, err := a.TMDBFor(ctx, "portal", item); info != nil || err != nil {
			t.Fatalf("off: %+v %v", info, err)
		}
		if _, err := a.TMDBArtwork(ctx, "portal", item, "poster"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("artwork: %v", err)
		}
		if eps, err := a.TMDBEpisodes(ctx, "portal", item, 1); len(eps) != 0 || err != nil {
			t.Fatal("episodes")
		}
		if v, err := a.TMDBMatch(ctx, "portal", "km"); err != nil || v.Reason != "disabled" {
			t.Fatalf("match view: %+v %v", v, err)
		}
		if _, err := a.TMDBSearch(ctx, "Matrix", "movie", ""); !errors.Is(err, ErrTMDBDisabled) {
			t.Fatalf("search: %v", err)
		}
	}
	if s.Hits("") != 0 {
		t.Fatalf("%d requests while TMDB is off", s.Hits(""))
	}
}

func TestTMDBConcurrentLookupsShareRequests(t *testing.T) {
	s := fakeTMDB(t)
	s.Set(func(s *tmdbtest.Server) { s.Delay = 50 * time.Millisecond })
	a, _ := tmdbTestApp(t, s, true)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := a.TMDBFor(context.Background(), "portal", vodMovie("m", "The Matrix", "", "1999"))
			if err != nil || info == nil || info.ID != 603 {
				t.Error(info, err)
			}
		}()
	}
	wg.Wait()
	if s.Hits("search/movie") != 1 || s.Hits("movie") != 1 {
		t.Fatalf("search %d, details %d", s.Hits("search/movie"), s.Hits("movie"))
	}
}

func TestTMDBManualMatchPersistsAndResets(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	item := vodMovie("m", "The Matrix", "", "1999")
	item.Key = "km"
	browse(t, a, "", item)
	if info, _ := a.TMDBFor(ctx, "portal", item); info == nil || info.Match != "auto" {
		t.Fatal("auto match first")
	}
	if _, err := a.SetTMDBMatch(ctx, "portal", "km", 777777, "movie"); err == nil {
		t.Fatal("a record TMDB does not have was accepted")
	}
	info, err := a.SetTMDBMatch(ctx, "portal", "km", 604, "movie")
	if err != nil || info.ID != 604 || info.Match != "manual" {
		t.Fatalf("set: %+v %v", info, err)
	}
	if info, _ := a.TMDBFor(ctx, "portal", item); info == nil || info.ID != 604 || info.Match != "manual" {
		t.Fatalf("manual match not used: %+v", info)
	}
	view, err := a.TMDBMatch(ctx, "portal", "km")
	if err != nil || view.Reason != "matched" || !view.Manual || view.Current.ID != 604 || len(view.Candidates) == 0 || view.Candidates[0].ID != 603 {
		t.Fatalf("view %+v %v", view, err)
	}
	if !strings.HasPrefix(view.Candidates[0].Poster, s.ImageBase+"/w185/") {
		t.Fatalf("thumbnail %q", view.Candidates[0].Poster)
	}

	// Survives a restart.
	b, err := New(a.cfg, DefaultSettings(), a.log)
	if err != nil {
		t.Fatal(err)
	}
	if ov := b.st.TMDB.Overrides; len(ov) != 1 || ov[0] != (TMDBOverride{Source: "portal", Type: "vod", Root: "m", ID: 604, Kind: "movie"}) {
		t.Fatalf("overrides after restart: %+v", ov)
	}

	if err := a.ResetTMDBMatch(ctx, "portal", "km"); err != nil {
		t.Fatal(err)
	}
	if info, _ := a.TMDBFor(ctx, "portal", item); info == nil || info.ID != 603 || info.Match != "auto" {
		t.Fatalf("reset must bring the automatic match back, not the cached manual one: %+v", info)
	}
	if len(a.st.TMDB.Overrides) != 0 {
		t.Fatal("override kept")
	}

	// A manual choice that vanished from TMDB shows provider data until reset.
	a.mu.Lock()
	a.st.TMDB.Overrides = []TMDBOverride{{Source: "portal", Type: "vod", Root: "m", ID: 31337, Kind: "movie"}}
	a.tmdbSvc().forget(tmdbRootKey{source: "portal", typ: "vod", root: "m"})
	a.mu.Unlock()
	if info, err := a.TMDBFor(ctx, "portal", item); info != nil || err != nil {
		t.Fatalf("stale manual match: %+v %v", info, err)
	}
	if view, _ := a.TMDBMatch(ctx, "portal", "km"); view.Current != nil || !view.Manual || view.Error == "" {
		t.Fatalf("stale view %+v", view)
	}
	a.ResetTMDBMatch(ctx, "portal", "km")
	if info, _ := a.TMDBFor(ctx, "portal", item); info == nil || info.ID != 603 {
		t.Fatalf("after resetting a stale match: %+v", info)
	}

	if _, err := a.TMDBMatch(ctx, "portal", "gone"); !errors.Is(err, ErrCatalogExpired) {
		t.Fatalf("stale navigation key: %v", err)
	}
}

func TestTMDBSearchByTextAndLink(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	c, err := a.TMDBSearch(ctx, "matrix", "movie", "1999")
	if err != nil || len(c) != 2 || c[0].ID != 603 || c[0].Year != "1999" || c[0].Poster == "" {
		t.Fatalf("search %+v %v", c, err)
	}
	c, err = a.TMDBSearch(ctx, "https://www.themoviedb.org/tv/60622-fargo?language=ru", "movie", "")
	if err != nil || len(c) != 1 || c[0].ID != 60622 || c[0].Type != "tv" {
		t.Fatalf("link %+v %v", c, err)
	}
	if _, err := a.TMDBSearch(ctx, "x", "anime", ""); err == nil {
		t.Fatal("bad type accepted")
	}
}

// The demo portal's three series shapes: seasons and episodes resolve to their
// series even when TMDBFor sees them first.
func TestTMDBSeasonsAndEpisodesResolveToSeries(t *testing.T) {
	s := fakeTMDB(t)
	portal := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Catalog: true})
	defer portal.Close()
	a, _ := tmdbTestApp(t, s, true)
	a.st.Sources[0].Server = portal.URL + "/c/"
	ctx := context.Background()
	for _, tc := range []struct {
		section, name string
		show          int
	}{{"series", "Обычный сериал", 100}, {"vod", "Сериал с эпизодами", 101}, {"vod", "Сериал Ministra", 102}} {
		roots, err := a.CatalogItems(ctx, "portal", tc.section, "1", "", 1, false)
		if err != nil {
			t.Fatal(err)
		}
		var root stalker.CatalogItem
		for _, it := range roots.Items {
			if it.Name == tc.name {
				root = it
			}
		}
		seasons, err := a.CatalogItems(ctx, "portal", tc.section, "1", root.Key, 1, false)
		if err != nil || len(seasons.Items) == 0 {
			t.Fatalf("%s seasons: %v", tc.name, err)
		}
		episodes, err := a.CatalogItems(ctx, "portal", tc.section, "1", seasons.Items[0].Key, 1, false)
		if err != nil || len(episodes.Items) < 2 {
			t.Fatalf("%s episodes: %v", tc.name, err)
		}
		episode := episodes.Items[1]
		info, err := a.TMDBFor(ctx, "portal", episode)
		if err != nil || info == nil || info.ID != tc.show || info.Type != "tv" {
			t.Fatalf("%s episode → %+v %v", tc.name, info, err)
		}
		if info, _ := a.TMDBFor(ctx, "portal", seasons.Items[0]); info == nil || info.ID != tc.show {
			t.Fatalf("%s season → %+v", tc.name, info)
		}
		all, err := a.TMDBEpisodes(ctx, "portal", seasons.Items[0], 1)
		if err != nil || len(all) != 3 || all[1].Name != "Пилот" {
			t.Fatalf("%s season episodes %+v %v", tc.name, all, err)
		}
		one, _ := a.TMDBEpisodes(ctx, "portal", episode, 1)
		if len(one) != 1 || one[2].Name != "Вторая" {
			t.Fatalf("%s: an episode must get only itself: %+v", tc.name, one)
		}
		if none, err := a.TMDBEpisodes(ctx, "portal", seasons.Items[0], 7); len(none) != 0 || err != nil {
			t.Fatalf("%s: a season TMDB lacks: %+v %v", tc.name, none, err)
		}
	}
	// A season whose series was never browsed has nothing to search with.
	orphan := stalker.CatalogItem{Type: "series", Kind: "season", ID: "zz:1", MovieID: "zz", Name: "Сезон 1"}
	if info, err := a.TMDBFor(ctx, "portal", orphan); info != nil || err != nil {
		t.Fatalf("orphan season: %+v %v", info, err)
	}
	// The demo film, through its real navigation key.
	films, _ := a.CatalogItems(ctx, "portal", "vod", "1", "", 1, false)
	view, err := a.TMDBMatch(ctx, "portal", films.Items[0].Key)
	if err != nil || view.Reason != "matched" || view.Current.ID != 200 || view.Item.Name != "Демо фильм" || view.Item.Year != "2025" {
		t.Fatalf("demo film view %+v %v", view, err)
	}
}

func TestTMDBArtworkPassesThroughWithinLimits(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	matrix := vodMovie("m", "The Matrix", "", "1999")
	art, err := a.TMDBArtwork(ctx, "portal", matrix, "poster")
	if err != nil || art.ContentType != "image/jpeg" || !bytes.Equal(art.Data, picture(t, "jpeg")) {
		t.Fatalf("poster: %q %v", art.ContentType, err)
	}
	if art, err := a.TMDBArtwork(ctx, "portal", matrix, "backdrop"); err != nil || art.ContentType != "image/png" {
		t.Fatalf("backdrop: %v", err)
	}
	n := s.Hits("image")
	a.TMDBArtwork(ctx, "portal", matrix, "poster")
	if s.Hits("image") != n {
		t.Fatal("pictures must be cached")
	}
	if _, err := a.TMDBArtwork(ctx, "portal", matrix, "logo"); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown kind")
	}
	huge := vodMovie("h", "Матрица: Перезагрузка", "", "2003")
	if _, err := a.TMDBArtwork(ctx, "portal", huge, "poster"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("a picture over the size cap: %v", err)
	}
	if _, err := a.TMDBArtwork(ctx, "portal", huge, "backdrop"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no backdrop: %v", err)
	}
	fargo := vodMovie("f", "Fargo", "", "1996")
	if _, err := a.TMDBArtwork(ctx, "portal", fargo, "poster"); err == nil {
		t.Fatal("a GIF must not pass through to the receiver")
	}
	if _, err := a.TMDBArtwork(ctx, "portal", vodMovie("u", "Нет такого", "", ""), "poster"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unmatched: %v", err)
	}
	if b := a.tmdbSvc().pictures.Bytes(); b > tmdbImageBytes {
		t.Fatalf("picture cache %d bytes", b)
	}
}

func TestTMDBSettings(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, false)
	if st := a.TMDBStatus(); st.Enabled || st.KeySet || st.Language != "ru-RU" {
		t.Fatalf("defaults %+v", st)
	}
	bad := "not a key!"
	if _, err := a.UpdateTMDB(TMDBUpdate{APIKey: &bad}); err == nil {
		t.Fatal("malformed key accepted")
	}
	lang := "russian"
	if _, err := a.UpdateTMDB(TMDBUpdate{Language: &lang}); err == nil {
		t.Fatal("malformed language accepted")
	}
	key := testTMDBKey
	st, err := a.UpdateTMDB(TMDBUpdate{APIKey: &key})
	if err != nil || !st.Enabled || !st.KeySet || st.KeyHint != "••••0123" {
		t.Fatalf("first key: %+v %v", st, err)
	}
	b, _ := json.Marshal(st)
	assertNoKey(t, "status", string(b))

	// An empty key keeps the stored one.
	empty := ""
	if st, _ := a.UpdateTMDB(TMDBUpdate{APIKey: &empty}); !st.KeySet {
		t.Fatal("empty key wiped the stored one")
	}
	ctx := context.Background()
	a.TMDBFor(ctx, "portal", vodMovie("m", "The Matrix", "", "1999"))
	n := s.Hits("")
	lang = "en-US"
	if st, _ := a.UpdateTMDB(TMDBUpdate{Language: &lang}); st.Language != "en-US" {
		t.Fatal("language")
	}
	a.TMDBFor(ctx, "portal", vodMovie("m", "The Matrix", "", "1999"))
	if s.Hits("") == n || s.Languages()[len(s.Languages())-1] != "en-US" {
		t.Fatal("a new language must drop the cached answers")
	}

	// Unchanged settings do not touch the flash.
	os.Remove(a.statePath())
	if _, err := a.UpdateTMDB(TMDBUpdate{Language: &lang}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.statePath()); !os.IsNotExist(err) {
		t.Fatal("state written without a change")
	}

	if res := a.TestTMDB(ctx, ""); !res.OK {
		t.Fatalf("stored key test: %+v", res)
	}
	if res := a.TestTMDB(ctx, "OTHERkey0123456789OTHERkey012345"); res.OK || !strings.Contains(res.Error, "Ключ не подошёл") {
		t.Fatalf("wrong key test: %+v", res)
	}
	// Testing a wrong key must not block the saved one.
	if info, _ := a.TMDBFor(ctx, "portal", vodMovie("m9", "Fargo", "", "1996")); info == nil {
		t.Fatal("stored key blocked by a test")
	}
	if st, _ := a.UpdateTMDB(TMDBUpdate{ClearKey: true}); st.KeySet || st.KeyHint != "" {
		t.Fatal("clear key")
	}
}

func TestTMDBVODSeriesFallsBackToFilm(t *testing.T) {
	s := fakeTMDB(t)
	s.AddMovie(tmdbtest.Record{ID: 8100, Title: "...И ещё одна ночь Шахерезады", Original: "...I eshchyo odna noch Shakherezady", Date: "1984-01-01"})
	s.AddMovie(tmdbtest.Record{ID: 8200, Title: "Роботы", Original: "Robots", Date: "2023-05-19"})
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	two := stalker.CatalogItem{Type: "vod", Kind: "series", ID: "sh", Name: "... и ещё одна ночь Шахерезады", Year: "1984"}
	info, err := a.TMDBFor(ctx, "portal", two)
	if err != nil || info == nil || info.ID != 8100 || info.Type != "movie" {
		t.Fatalf("two-part film filed as a series: %+v %v", info, err)
	}
	robots, err := a.TMDBFor(ctx, "portal", vodMovie("r", "Robots 2023", "", "2023"))
	if err != nil || robots == nil || robots.ID != 8200 {
		t.Fatalf("year glued to the title: %+v %v", robots, err)
	}
	show := stalker.CatalogItem{Type: "series", Kind: "series", ID: "x", Name: "Нет такого шоу", Year: "2001"}
	if info, _ := a.TMDBFor(ctx, "portal", show); info != nil {
		t.Fatalf("the series section never falls back to films: %+v", info)
	}
}
