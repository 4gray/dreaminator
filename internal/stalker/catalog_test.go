package stalker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func catalogClient(t *testing.T, handle func(http.ResponseWriter, *http.Request)) *Client {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(handle))
	t.Cleanup(s.Close)
	return New(s.URL, Identity{MAC: "00:1A:79:12:34:56"}, Session{Endpoint: s.URL + "/portal.php", Simple: true}, s.Client())
}
func catalogReply(w http.ResponseWriter, data any) {
	json.NewEncoder(w).Encode(map[string]any{"js": data})
}

func TestCatalogCategoriesAndPage(t *testing.T) {
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("type") != "vod" {
			t.Errorf("wrong type %v", q)
		}
		if q.Get("action") == "get_categories" {
			catalogReply(w, map[string]any{"data": []any{map[string]any{"id": 7, "title": "Films"}}})
			return
		}
		if q.Get("category") != "7" || q.Get("p") != "2" {
			t.Errorf("wrong query %v", q)
		}
		catalogReply(w, map[string]any{"total_items": "5", "max_page_items": 2, "data": []any{map[string]any{"id": 3, "name": "Film", "description": "Plot", "screenshot_uri": "/poster.jpg", "year": 2025, "time": "91", "cmd": "/media/film.mpg", "use_http_tmp_link": "1"}}})
	})
	cats, err := c.CatalogCategories(context.Background(), "vod")
	if err != nil || len(cats) != 1 || cats[0].ID != "7" {
		t.Fatalf("categories %v %v", cats, err)
	}
	pg, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod", Category: "7", Page: 2})
	if err != nil || len(pg.Items) != 1 || !pg.HasMore || pg.Page != 2 {
		t.Fatalf("page %+v %v", pg, err)
	}
	i := pg.Items[0]
	if i.Kind != "movie" || i.Year != "2025" || i.Duration != "91" || !strings.HasSuffix(i.Poster, "/poster.jpg") {
		t.Fatalf("item %+v", i)
	}
}

func TestCatalogSeriesModesAndFreshLinks(t *testing.T) {
	for _, section := range []string{"series", "vod"} {
		t.Run(section, func(t *testing.T) {
			var calls []url.Values
			c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				calls = append(calls, q)
				switch q.Get("action") {
				case "create_link":
					if q.Get("type") != "vod" || q.Get("series") != "2" || q.Get("cmd") != "ffrt4://season/2" {
						t.Errorf("lost selection: %v", q)
					}
					catalogReply(w, map[string]any{"cmd": fmt.Sprintf("ffmpeg https://cdn.example/play?token=%d", len(calls))})
				case "get_ordered_list":
					if q.Get("movie_id") != "42" {
						t.Errorf("movie query %v", q)
					}
					catalogReply(w, []any{map[string]any{"id": "42:2", "name": "Season 2", "cmd": "ffrt4://season/2", "series": []any{1, "2"}}})
				}
			})
			parent := CatalogItem{ID: "42", Name: "Show", Type: section, Kind: "series", Cmd: "ffrt4://season/2"}
			if section == "vod" {
				parent.embedded = []string{"1", "2"}
			}
			pg, err := c.CatalogPage(context.Background(), CatalogQuery{Type: section, Parent: &parent})
			if err != nil {
				t.Fatal(err)
			}
			if section == "series" || section == "vod" {
				if len(pg.Items) != 1 || pg.Items[0].Kind != "season" {
					t.Fatalf("seasons %+v", pg)
				}
				season := pg.Items[0]
				pg, err = c.CatalogPage(context.Background(), CatalogQuery{Type: section, Parent: &season})
			}
			if err != nil || len(pg.Items) != 2 || pg.Items[1].Kind != "episode" {
				t.Fatalf("episodes %+v %v", pg, err)
			}
			a, err := c.ResolveCatalog(context.Background(), pg.Items[1])
			if err != nil {
				t.Fatal(err)
			}
			b, err := c.ResolveCatalog(context.Background(), pg.Items[1])
			if err != nil || a == b {
				t.Fatalf("links %q %q %v", a, b, err)
			}
		})
	}
}

func TestCatalogLazyMinistraAndRenewal(t *testing.T) {
	portal := newPortal()
	portal.Catalog = true
	portal.TokenUses = 2
	portal.PageSize = 2
	c, _ := client(t, portal, "00:1A:79:12:34:56", Identity{})
	ctx := context.Background()
	pg, err := c.CatalogPage(ctx, CatalogQuery{Type: "vod", Page: 2})
	if err != nil || len(pg.Items) != 1 || pg.Items[0].Kind != "series" {
		t.Fatalf("shows %+v %v", pg, err)
	}
	parent := pg.Items[0]
	seasons, err := c.CatalogPage(ctx, CatalogQuery{Type: "vod", Parent: &parent})
	if err != nil || len(seasons.Items) != 2 {
		t.Fatalf("seasons %+v %v", seasons, err)
	}
	season := seasons.Items[1]
	episodes, err := c.CatalogPage(ctx, CatalogQuery{Type: "vod", Parent: &season})
	if err != nil || len(episodes.Items) != 2 {
		t.Fatalf("episodes %+v %v", episodes, err)
	}
	ep := episodes.Items[1]
	if ep.SeasonID != "lazy:2" || ep.MovieID != "lazy" || ep.Episode != "2" {
		t.Fatalf("episode %+v", ep)
	}
	stream, err := c.ResolveCatalog(ctx, ep)
	if err != nil || !strings.Contains(stream, "lazy-2-2") {
		t.Fatalf("selected link %q %v", stream, err)
	}
	hs, _, _, _ := portal.Stats()
	if hs < 2 {
		t.Fatalf("did not renew: %d", hs)
	}
}

func TestCatalogRejectsMalformedAndSanitizesErrors(t *testing.T) {
	for _, body := range []string{`{"js":{"error":"secret MAC TOKEN"}}`, `secret MAC TOKEN`, `{"js":{"data":"wrong"}}`} {
		c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		_, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("error %v", err)
		}
	}
	for _, cmd := range []string{"javascript:alert(1)", "file:///etc/passwd", "http://", "https://user:pass@host/path"} {
		c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { catalogReply(w, map[string]any{"cmd": cmd}) })
		if _, err := c.ResolveCatalog(context.Background(), CatalogItem{Kind: "movie", Type: "vod", Cmd: "secret", NeedsLink: true}); err == nil {
			t.Errorf("accepted %q", cmd)
		}
	}
}

func TestCatalogPlaybackHeaders(t *testing.T) {
	c := New("https://portal.example/c/", Identity{MAC: "00:1A:79:12:34:56"}, Session{Endpoint: "https://portal.example/portal.php", Token: "secret"}, nil)
	for _, target := range []string{"https://portal.example:8443/play", "https://cdn.example/play", "http://portal.example/play", "file:///secret"} {
		h := c.PlaybackHeaders(target)
		if h["User-Agent"] != UserAgent {
			t.Fatal("missing MAG identity")
		}
		same := strings.HasPrefix(target, "https://portal.example:")
		if (h["Authorization"] != "") != same || (h["Cookie"] != "") != same {
			t.Fatalf("credential scope for %s: %v", target, h)
		}
	}
}

func TestCatalogMinistraFiltersFlagsAndKeepsEpisodeCommand(t *testing.T) {
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "create_link" {
			if r.URL.Query().Get("cmd") != "ffmpeg /media/specific.mpg" || r.URL.Query().Get("series") != "9" {
				t.Errorf("selection lost %v", r.URL.Query())
			}
			catalogReply(w, map[string]any{"cmd": "//cdn.example/selected.mp4"})
			return
		}
		catalogReply(w, map[string]any{"data": []any{map[string]any{"id": 101, "is_episode": "0", "name": "Not an episode"}, map[string]any{"id": 102, "is_episode": "1", "name": "Selected episode", "series_number": "9", "cmd": "ffmpeg /media/specific.mpg"}}})
	})
	season := CatalogItem{ID: "s2", MovieID: "show", Kind: "season", Type: "vod", lazy: true}
	pg, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod", Parent: &season})
	if err != nil || len(pg.Items) != 1 || pg.Items[0].ID != "102" {
		t.Fatalf("episode flags %+v %v", pg, err)
	}
	stream, err := c.ResolveCatalog(context.Background(), pg.Items[0])
	if err != nil || stream != "http://cdn.example/selected.mp4" {
		t.Fatalf("stream %s %v", stream, err)
	}
}

func TestCatalogFixtureRegularAndEmbedded(t *testing.T) {
	portal := newPortal()
	portal.Catalog = true
	c, _ := client(t, portal, "00:1A:79:12:34:56", Identity{})
	ctx := context.Background()
	for _, section := range []string{"series", "vod"} {
		pg, err := c.CatalogPage(ctx, CatalogQuery{Type: section})
		if err != nil {
			t.Fatal(err)
		}
		parent := pg.Items[0]
		expected := "regular-2-2"
		if section == "vod" {
			parent = pg.Items[1]
			expected = "embedded-2"
		}
		children, err := c.CatalogPage(ctx, CatalogQuery{Type: section, Parent: &parent})
		if err != nil {
			t.Fatal(err)
		}
		if section == "series" || section == "vod" {
			season := children.Items[0]
			if section == "series" {
				season = children.Items[1]
			}
			children, err = c.CatalogPage(ctx, CatalogQuery{Type: section, Parent: &season})
			if err != nil {
				t.Fatal(err)
			}
		}
		stream, err := c.ResolveCatalog(ctx, children.Items[1])
		if err != nil || !strings.Contains(stream, expected) {
			t.Fatalf("%s selected %s %v", section, stream, err)
		}
	}
}

func TestCatalogRegularColonIDAndAllCategory(t *testing.T) {
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "get_categories" {
			catalogReply(w, []any{map[string]any{"id": "*", "title": "Все"}})
			return
		}
		if r.URL.Query().Get("movie_id") != "42" {
			t.Errorf("movie id %v", r.URL.Query())
		}
		catalogReply(w, []any{})
	})
	cats, err := c.CatalogCategories(context.Background(), "series")
	if err != nil || len(cats) != 1 || cats[0].ID != "*" {
		t.Fatalf("all category %v %v", cats, err)
	}
	parent := CatalogItem{Type: "series", Kind: "series", ID: "42:7"}
	_, err = c.CatalogPage(context.Background(), CatalogQuery{Type: "series", Parent: &parent})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCatalogBoundsRowsAndEmbeddedEpisodes(t *testing.T) {
	rows := make([]map[string]any, 5001)
	for i := range rows {
		rows[i] = map[string]any{"id": i, "name": "row"}
	}
	for _, data := range []any{rows, []any{map[string]any{"id": 1, "name": "show", "series": make([]int, 5001)}}} {
		c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { catalogReply(w, data) })
		if _, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"}); err == nil {
			t.Fatal("unbounded response accepted")
		}
	}
}

func TestCatalogUnsupportedAndEmpty(t *testing.T) {
	for _, body := range []string{`{"error":"Unknown action"}`, `{"error":"Not supported secret token"}`} {
		c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"js":%s}`, body) })
		_, err := c.CatalogCategories(context.Background(), "vod")
		if err != ErrCatalogUnsupported {
			t.Fatalf("unsupported error %v", err)
		}
	}
	for _, body := range []string{`[]`, `{"data":[]}`, `{"data":null}`} {
		c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"js":%s}`, body) })
		pg, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
		if err != nil || pg.Items == nil || len(pg.Items) != 0 || pg.HasMore {
			t.Fatalf("empty %+v %v", pg, err)
		}
	}
}

func TestCatalogRelativeLinkAndPrivateJSON(t *testing.T) {
	for cmd, want := range map[string]string{"ffmpeg /media/a.mpg": "/media/a.mpg", "?token=fresh": "/media/original.mpg?token=fresh"} {
		c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { catalogReply(w, map[string]any{"cmd": cmd}) })
		item := CatalogItem{Key: "opaque", Name: "Movie", Kind: "movie", Type: "vod", Cmd: "/media/original.mpg", ID: "secret-provider-id", Poster: "secret-poster", MovieID: "secret-movie", SeasonID: "secret-season", Episode: "secret-episode"}
		stream, err := c.ResolveCatalog(context.Background(), item)
		if err != nil || stream != c.Portal+want {
			t.Fatalf("relative link %q %v", stream, err)
		}
		public, _ := json.Marshal(item)
		if strings.Contains(string(public), "secret") || strings.Contains(string(public), "original.mpg") {
			t.Fatalf("private data serialized: %s", public)
		}
	}
}

func TestCatalogPreservesCommandQueryEscapes(t *testing.T) {
	original := "ffmpeg https://cdn.example/video?signature=a+b%2Fc&other=one two"
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("cmd"); got != original {
			t.Errorf("command changed: got %q want %q", got, original)
		}
		catalogReply(w, map[string]any{"cmd": "https://cdn.example/resolved.mp4"})
	})
	if _, err := c.ResolveCatalog(context.Background(), CatalogItem{Kind: "movie", Type: "vod", Cmd: original}); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogMinistraUsesSeasonVideoID(t *testing.T) {
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("season_id") == "" {
			if q.Get("movie_id") != "root" {
				t.Errorf("root query %v", q)
			}
			catalogReply(w, []any{map[string]any{"id": "season-4", "video_id": 987, "name": "Season 4", "is_season": "1"}})
			return
		}
		if q.Get("movie_id") != "987" || q.Get("season_id") != "season-4" {
			t.Errorf("season query %v", q)
		}
		catalogReply(w, []any{map[string]any{"id": "episode-2", "name": "Episode 2", "is_episode": true, "series_number": 2}})
	})
	root := CatalogItem{ID: "root", Kind: "series", Type: "vod", lazy: true}
	seasons, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod", Parent: &root})
	if err != nil || len(seasons.Items) != 1 {
		t.Fatalf("seasons %+v %v", seasons, err)
	}
	season := seasons.Items[0]
	episodes, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod", Parent: &season})
	if err != nil || len(episodes.Items) != 1 || episodes.Items[0].MovieID != "987" {
		t.Fatalf("episodes %+v %v", episodes, err)
	}
}

func TestCatalogNetworkErrorIsDistinctAndSanitized(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.Close()
	endpoint := s.URL + "/secret-token.php"
	c := New(endpoint, Identity{}, Session{Endpoint: endpoint, Simple: true}, s.Client())
	_, networkErr := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
	malformed := catalogClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "invalid secret-token") })
	_, malformedErr := malformed.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
	if networkErr == nil || malformedErr == nil || networkErr.Error() == malformedErr.Error() {
		t.Fatalf("network %v malformed %v", networkErr, malformedErr)
	}
	for _, err := range []error{networkErr, malformedErr} {
		if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), s.URL) {
			t.Fatalf("sensitive error: %v", err)
		}
	}
}

func TestCatalogMockUsesConfiguredSampleStream(t *testing.T) {
	portal := newPortal()
	portal.Catalog = true
	portal.CatalogStream = "https://media.example/sample.mp4?existing=keep#part"
	c, _ := client(t, portal, "00:1A:79:12:34:56", Identity{})
	pg, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.ResolveCatalog(context.Background(), pg.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.ResolveCatalog(context.Background(), pg.Items[0])
	if err != nil || first == second {
		t.Fatalf("renewal %q %q %v", first, second, err)
	}
	u, err := url.Parse(first)
	if err != nil || u.Host != "media.example" || u.Path != "/sample.mp4" || u.Query().Get("existing") != "keep" || u.Query().Get("play_token") == "" || u.Fragment != "part" {
		t.Fatalf("configured stream %q %v", first, err)
	}
}

func TestCatalogGeneratedEpisodesArePaged(t *testing.T) {
	c := New("http://unused.example", Identity{}, Session{}, nil)
	parent := CatalogItem{Type: "series", Kind: "season", ID: "show:season", MovieID: "show", Cmd: "season-command"}
	for n := 1; n <= 123; n++ {
		parent.embedded = append(parent.embedded, fmt.Sprint(n))
	}
	for page, want := range map[int]int{1: 50, 2: 50, 3: 23, 4: 0} {
		pg, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "series", Parent: &parent, Page: page})
		if err != nil || len(pg.Items) != want || pg.HasMore != (page < 3) {
			t.Fatalf("page %d: %+v %v", page, pg, err)
		}
		if want > 0 && (pg.Items[0].Episode != fmt.Sprint((page-1)*50+1) || pg.Items[0].Cmd != parent.Cmd) {
			t.Fatalf("wrong page selection %+v", pg.Items[0])
		}
	}
}

func TestCatalogCacheSizeIncludesPrivateMetadata(t *testing.T) {
	empty := CatalogItem{}
	item := CatalogItem{ID: "provider-id", Cmd: "secret-command", embedded: []string{strings.Repeat("a", 100), strings.Repeat("b", 200)}}
	if empty.CacheSize() <= 0 || item.CacheSize()-empty.CacheSize() < len(item.ID)+len(item.Cmd)+300 {
		t.Fatalf("private metadata undercounted: %d -> %d", empty.CacheSize(), item.CacheSize())
	}
	item.Key = strings.Repeat("k", 1000)
	if item.CacheSize() < 1300 {
		t.Fatalf("public metadata undercounted: %d", item.CacheSize())
	}
}

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type catalogCountReader struct {
	io.Reader
	read int
}

func (r *catalogCountReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}
func (r *catalogCountReader) Close() error { return nil }

func TestCatalogWireResponseBound(t *testing.T) {
	const limit = 8 << 20
	for _, declared := range []int64{limit + 1, -1} {
		body := &catalogCountReader{Reader: strings.NewReader(strings.Repeat("x", limit+1024))}
		c := New("http://portal.example", Identity{}, Session{}, &http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: declared, Body: body}, nil
		})})
		_, _, err := c.raw(context.Background(), "http://portal.example/portal.php", []param{p("type", "vod"), p("action", "get_ordered_list")}, "")
		if err == nil || body.read > limit+1 || declared > limit && body.read != 0 {
			t.Fatalf("declared %d read %d error %v", declared, body.read, err)
		}
	}
	body := &catalogCountReader{Reader: strings.NewReader(strings.Repeat("x", limit+1024))}
	c := New("http://portal.example", Identity{}, Session{}, &http.Client{Transport: catalogTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: limit + 1024, Body: body}, nil
	})})
	_, data, err := c.raw(context.Background(), "http://portal.example/portal.php", []param{p("type", "itv"), p("action", "get_all_channels")}, "")
	if err != nil || len(data) != limit+1024 {
		t.Fatalf("live bulk changed: %d %v", len(data), err)
	}
}

func TestCatalogInterruptedResponseIsNetworkError(t *testing.T) {
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		fmt.Fprint(w, `{"js":"secret partial`)
	})
	_, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
	if err != ErrCatalogNetwork {
		t.Fatalf("interrupted body classified as %v", err)
	}
}

func TestCatalogInterruptedColdAuthenticationIsNetworkError(t *testing.T) {
	for _, action := range []string{"handshake", "get_profile"} {
		t.Run(action, func(t *testing.T) {
			interrupted := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("action") == action {
					interrupted = true
					w.Header().Set("Content-Length", "1024")
					fmt.Fprint(w, `{"js":"secret partial auth`)
					return
				}
				if r.URL.Query().Get("action") == "handshake" {
					catalogReply(w, map[string]any{"token": "secret-token"})
					return
				}
				fmt.Fprint(w, "Unauthorized request.")
			}))
			defer server.Close()
			c := New(server.URL, Identity{MAC: "00:1A:79:12:34:56"}, Session{}, server.Client())
			_, err := c.CatalogPage(context.Background(), CatalogQuery{Type: "vod"})
			if !interrupted || err != ErrCatalogNetwork {
				t.Fatalf("cold %s interruption: %v (reached=%v)", action, err, interrupted)
			}
		})
	}
}

func TestCatalogSortAppliesToListsOnly(t *testing.T) {
	var seen []string
	c := catalogClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("sortby"))
		catalogReply(w, map[string]any{"total_items": "1", "max_page_items": 14, "data": []any{map[string]any{"id": "s", "name": "Show", "series": []any{1, 2}}}})
	})
	ctx := context.Background()
	for _, sort := range []string{"", "top", "bogus"} {
		if _, err := c.CatalogPage(ctx, CatalogQuery{Type: "vod", Category: "1", Page: 1, Sort: sort}); err != nil {
			t.Fatal(err)
		}
	}
	parent := CatalogItem{Type: "vod", Kind: "series", ID: "s", lazy: true}
	if _, err := c.CatalogPage(ctx, CatalogQuery{Type: "vod", Page: 1, Parent: &parent, Sort: "top"}); err != nil {
		t.Fatal(err)
	}
	// New arrivals first by default (omitting sortby means "by name" on some
	// portals); unknown orders fall back; seasons keep their natural order.
	want := []string{"added", "top", "added", "name"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("sortby %v, want %v", seen, want)
	}
}
