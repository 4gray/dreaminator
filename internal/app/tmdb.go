package app

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"image"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"dreaminator/internal/fetch"
	"dreaminator/internal/stalker"
	"dreaminator/internal/tmdb"
)

// TMDB decorates Stalker catalog entries with descriptions, posters and
// ratings from The Movie Database. It is strictly optional: without a key,
// without a confident match, or when TMDB is unreachable, everything here
// returns nothing and the catalog shows the provider's own data. Nothing here
// changes playback commands or ids.
//
// The API key lives in State.TMDB, which no API response serialises; the WebUI
// and the plugin see TMDBStatus with a masked hint only. PUT /api/settings
// replaces Settings wholesale and therefore cannot touch it.

const (
	tmdbDefaultLanguage = "ru-RU"
	tmdbRequestTimeout  = 8 * time.Second
	tmdbLookupTimeout   = 25 * time.Second // a whole lookup: search, maybe a second search, details
	tmdbConcurrent      = 2
	tmdbPositiveTTL     = 24 * time.Hour
	tmdbNegativeTTL     = time.Hour
	tmdbCacheEntries    = 1024
	tmdbTitleBytes      = 2 << 20
	tmdbSeasonEntries   = 128
	tmdbSeasonBytes     = 1 << 20
	tmdbImageBytes      = 4 << 20
	tmdbImageMax        = 1536 << 10
	tmdbMaxOverrides    = 1000
	tmdbPosterSize      = "w342"
	tmdbBackdropSize    = "w780"
	tmdbProfileSize     = "w185" // actor portraits: 185x278, shown at 120x180
	tmdbStillSize       = "w300" // episode stills: 300x169
	tmdbThumbSize       = "w185" // candidate list in the tablet browser
	tmdbMaxCandidates   = 12
)

var (
	ErrTMDBDisabled     = errors.New("TMDB выключен или не задан ключ API — включите его в настройках")
	errTMDBArtwork      = errors.New("картинка TMDB не подходит для ресивера")
	errTMDBNotMatchable = errors.New("для этой записи TMDB не используется")
	errTMDBBadChoice    = errors.New("неверная запись TMDB")
	errTMDBKeyFormat    = errors.New("это не похоже на ключ API TMDB: нужен «Ключ API» (v3) — 32 латинские буквы и цифры из настроек аккаунта themoviedb.org")
	errTMDBLanguage     = errors.New("неверный язык: нужен код вида ru-RU")
)

// TMDBInfo is what TMDB adds to a catalog card.
type TMDBInfo struct {
	ID            int          `json:"id"`
	Type          string       `json:"type"` // movie | tv
	Title         string       `json:"title"`
	OriginalTitle string       `json:"originalTitle,omitempty"`
	Overview      string       `json:"overview,omitempty"`
	Year          string       `json:"year,omitempty"`
	Runtime       int          `json:"runtime,omitempty"` // minutes (tv: typical episode runtime)
	Genres        []string     `json:"genres,omitempty"`
	Rating        string       `json:"rating,omitempty"` // vote_average, one decimal; "" when votes are too few (<10)
	Votes         int          `json:"votes,omitempty"`
	HasPoster     bool         `json:"hasPoster"`
	HasBackdrop   bool         `json:"hasBackdrop"`
	Match         string       `json:"match"`               // explicit | auto | manual
	Cast          []TMDBPerson `json:"cast,omitempty"`      // billing order; photos by index via TMDBCastPhoto
	Directors     []string     `json:"directors,omitempty"` // films: directors; series: creators
}

type TMDBPerson struct {
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	HasPhoto  bool   `json:"hasPhoto,omitempty"`
}

type TMDBEpisode struct {
	Number   int    `json:"number"`
	Name     string `json:"name,omitempty"`
	Overview string `json:"overview,omitempty"`
	Runtime  int    `json:"runtime,omitempty"`
	Still    string `json:"-"` // TMDB picture path; fetched through TMDBStill
}

// TMDBStatus is the masked view for the WebUI and the plugin's About screen.
type TMDBStatus struct {
	Enabled  bool   `json:"enabled"`
	Language string `json:"language"`
	KeySet   bool   `json:"keySet"`
	KeyHint  string `json:"keyHint,omitempty"`
}

// TMDBConfig is persisted in State.TMDB. It holds the key: never serve it.
type TMDBConfig struct {
	APIKey    string         `json:"apiKey,omitempty"`
	Language  string         `json:"language,omitempty"`
	Enabled   bool           `json:"enabled"`
	Overrides []TMDBOverride `json:"overrides,omitempty"`
}

// TMDBOverride is a match chosen by hand for one provider movie or series.
type TMDBOverride struct {
	Source string `json:"source"`
	Type   string `json:"type"` // provider section: vod | series
	Root   string `json:"root"` // provider id of the movie or series
	ID     int    `json:"id"`
	Kind   string `json:"kind"` // movie | tv
}

// tmdbService holds what TMDB lookups share: caches, the request limiter and
// in-flight lookups. Lock order: a.mu, then t.mu; never the other way round.
type tmdbService struct {
	once      sync.Once
	mu        sync.Mutex
	gen       uint64 // bumped when the key, language or a manual match changes
	apiBase   string
	imageBase string
	timeout   time.Duration
	hc        *http.Client
	hcTLS     bool
	limiter   *tmdb.Limiter
	images    chan struct{} // concurrent picture downloads

	matches   *tmdb.Cache[tmdbMatch]
	titles    *tmdb.Cache[tmdb.Title]
	seasons   *tmdb.Cache[map[int]TMDBEpisode]
	pictures  *tmdb.Cache[Artwork]
	lookups   tmdb.Group[*tmdbResolved]
	details   tmdb.Group[tmdb.Title]
	episodes  tmdb.Group[map[int]TMDBEpisode]
	downloads tmdb.Group[Artwork]

	logMu   sync.Mutex
	lastErr error
	lastLog time.Time
}

// tmdbMatch is a remembered decision for one provider entry; id 0 means none.
type tmdbMatch struct {
	kind string
	id   int
	how  string // explicit | auto | manual
}

type tmdbResolved struct {
	title tmdb.Title
	how   string
	env   tmdbEnv
}

// tmdbEnv is a snapshot of the configuration for one lookup.
type tmdbEnv struct {
	client    *tmdb.Client
	hc        *http.Client
	gen       uint64
	imageBase string
	userAgent string
}

// tmdbRootKey names the provider movie or series an item belongs to.
type tmdbRootKey struct{ source, typ, root, kind string }

func (k tmdbRootKey) String() string { return k.source + "\x00" + k.typ + "\x00" + k.root }

func (a *App) tmdbSvc() *tmdbService {
	t := &a.tmdbs
	t.once.Do(func() {
		t.apiBase = cmp.Or(a.cfg.TMDBAPIBase, tmdb.DefaultAPIBase)
		t.imageBase = cmp.Or(a.cfg.TMDBImageBase, tmdb.DefaultImageBase)
		t.timeout = tmdbRequestTimeout
		t.limiter = tmdb.NewLimiter(tmdbConcurrent)
		t.images = make(chan struct{}, tmdbConcurrent)
		t.matches = tmdb.NewCache[tmdbMatch](tmdbCacheEntries, 0, nil)
		t.titles = tmdb.NewCache(tmdbCacheEntries, tmdbTitleBytes, tmdb.Title.Size)
		t.seasons = tmdb.NewCache(tmdbSeasonEntries, tmdbSeasonBytes, seasonSize)
		t.pictures = tmdb.NewCache(tmdbCacheEntries, tmdbImageBytes, func(art Artwork) int { return len(art.Data) + 64 })
	})
	return t
}

func seasonSize(m map[int]TMDBEpisode) int {
	n := 64
	for _, e := range m {
		n += 48 + len(e.Name) + len(e.Overview)
	}
	return n
}

// keep stores into a cache unless the configuration changed meanwhile.
func (t *tmdbService) keep(gen uint64, put func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.gen == gen {
		put()
	}
}

// reset forgets everything learned with the previous key or language.
func (t *tmdbService) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gen++
	t.matches.Clear()
	t.titles.Clear()
	t.seasons.Clear()
	t.pictures.Clear()
	t.limiter.Reset()
}

// forget drops the decision for one entry after its manual match changed.
func (t *tmdbService) forget(rk tmdbRootKey) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gen++
	t.matches.Delete(rk.String())
}

func genKey(gen uint64, key string) string { return strconv.FormatUint(gen, 10) + "\x00" + key }

func tmdbLanguage(c TMDBConfig) string { return cmp.Or(c.Language, tmdbDefaultLanguage) }

// tmdbEnv snapshots what a lookup needs, so no network I/O happens under a.mu.
// ok is false when TMDB is off or has no key.
func (a *App) tmdbEnv(rk *tmdbRootKey) (env tmdbEnv, ov *TMDBOverride, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.st.TMDB
	if !c.Enabled || c.APIKey == "" {
		return tmdbEnv{}, nil, false
	}
	if rk != nil {
		for _, o := range c.Overrides {
			if o.Source == rk.source && o.Type == rk.typ && o.Root == rk.root {
				ov = &o
				break
			}
		}
	}
	return a.tmdbEnvLocked(), ov, true
}

// tmdbEnvLocked builds the snapshot; callers hold a.mu.
func (a *App) tmdbEnvLocked() tmdbEnv {
	t := a.tmdbSvc()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hc == nil || t.hcTLS != a.st.Settings.InsecureTLS {
		// The app's client (embedded CA roots, the user's TLS choice), built
		// once so keep-alive spares the receiver repeated TLS handshakes.
		t.hc = fetch.Client(a.fetchOptions(a.st.Settings, t.timeout))
		t.hcTLS = a.st.Settings.InsecureTLS
	}
	ua := "Dreaminator/" + cmp.Or(a.cfg.Version, "dev")
	return tmdbEnv{
		client:    &tmdb.Client{HTTP: t.hc, APIBase: t.apiBase, Key: a.st.TMDB.APIKey, Language: tmdbLanguage(a.st.TMDB), UserAgent: ua, Limiter: t.limiter},
		hc:        t.hc,
		gen:       t.gen,
		imageBase: t.imageBase,
		userAgent: ua,
	}
}

// tmdbRoot maps a catalog item to its provider movie or series: seasons and
// episodes belong to their series (MovieID; series-section ids carry a
// ":season" suffix the portal itself strips).
func tmdbRoot(sourceID string, item stalker.CatalogItem) (tmdbRootKey, bool) {
	var kind string
	switch item.Kind {
	case "movie":
		kind = "movie"
	case "series", "season", "episode":
		kind = "tv"
	default:
		return tmdbRootKey{}, false
	}
	if item.Type != "vod" && item.Type != "series" {
		return tmdbRootKey{}, false
	}
	id := item.ID
	if item.Kind == "season" || item.Kind == "episode" {
		id = item.MovieID
	}
	if item.Type == "series" {
		id, _, _ = strings.Cut(id, ":")
	}
	if id == "" || sourceID == "" {
		return tmdbRootKey{}, false
	}
	return tmdbRootKey{source: sourceID, typ: item.Type, root: id, kind: kind}, true
}

// tmdbRootItem finds the series a season or episode belongs to among the
// catalog entries the viewer has browsed: its name is what we search for.
func (a *App) tmdbRootItem(rk tmdbRootKey, item stalker.CatalogItem) (stalker.CatalogItem, bool) {
	if item.Kind == "movie" || item.Kind == "series" {
		return item, true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cache := a.catalogs[rk.source]
	if cache == nil {
		return stalker.CatalogItem{}, false
	}
	isRoot := func(it stalker.CatalogItem) bool {
		k, ok := tmdbRoot(rk.source, it)
		return ok && it.Kind == "series" && k == rk
	}
	for key, n := item.Key, 0; key != "" && n < 8; n++ {
		rec, ok := a.catalogRecordLocked(rk.source, cache, key)
		if !ok {
			break
		}
		if isRoot(rec.item) {
			return rec.item, true
		}
		key = rec.parent
	}
	for _, rec := range cache.items {
		if isRoot(rec.item) {
			return rec.item, true
		}
	}
	return stalker.CatalogItem{}, false
}

// ---- the API the catalog uses ----

// TMDBFor returns TMDB's view of a catalog movie, series, season or episode
// (seasons and episodes resolve to their series). It returns (nil, nil) when
// TMDB is off, has no key, or no record matches confidently; errors only say
// why TMDB could not be asked. Either way the caller shows provider data.
func (a *App) TMDBFor(ctx context.Context, sourceID string, item stalker.CatalogItem) (*TMDBInfo, error) {
	r, err := a.tmdbResolve(ctx, sourceID, item)
	if err != nil || r == nil {
		return nil, err
	}
	info := tmdbInfo(r.title, r.how)
	return &info, nil
}

// TMDBArtwork returns the matched record's poster or backdrop, as TMDB's own
// JPEG or PNG (never decoded here). ErrNotFound when there is no match or no
// such picture.
func (a *App) TMDBArtwork(ctx context.Context, sourceID string, item stalker.CatalogItem, kind string) (Artwork, error) {
	var size string
	switch kind {
	case "poster":
		size = tmdbPosterSize
	case "backdrop":
		size = tmdbBackdropSize
	default:
		return Artwork{}, ErrNotFound
	}
	r, err := a.tmdbResolve(ctx, sourceID, item)
	if err != nil {
		return Artwork{}, err
	}
	if r == nil {
		return Artwork{}, ErrNotFound
	}
	path := r.title.PosterPath
	if kind == "backdrop" {
		path = r.title.BackdropPath
	}
	u := tmdb.ImageURL(r.env.imageBase, size, path)
	if u == "" {
		return Artwork{}, ErrNotFound
	}
	return a.tmdbPicture(ctx, r.env, u)
}

// TMDBCastPhoto returns the portrait of the n-th actor of a matched title.
func (a *App) TMDBCastPhoto(ctx context.Context, sourceID string, item stalker.CatalogItem, n int) (Artwork, error) {
	r, err := a.tmdbResolve(ctx, sourceID, item)
	if err != nil {
		return Artwork{}, err
	}
	if r == nil || n < 0 || n >= len(r.title.Cast) {
		return Artwork{}, ErrNotFound
	}
	u := tmdb.ImageURL(r.env.imageBase, tmdbProfileSize, r.title.Cast[n].ProfilePath)
	if u == "" {
		return Artwork{}, ErrNotFound
	}
	return a.tmdbPicture(ctx, r.env, u)
}

// TMDBStill returns the still of one episode the provider lists; season is
// the provider's season number mapped to TMDB's (see seasonNumber).
func (a *App) TMDBStill(ctx context.Context, sourceID string, episode stalker.CatalogItem, season int) (Artwork, error) {
	eps, err := a.TMDBEpisodes(ctx, sourceID, episode, season)
	if err != nil {
		return Artwork{}, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(cmp.Or(episode.Episode, episode.Number)))
	e, ok := eps[n]
	if !ok || e.Still == "" {
		return Artwork{}, ErrNotFound
	}
	r, err := a.tmdbResolve(ctx, sourceID, episode)
	if err != nil || r == nil {
		return Artwork{}, ErrNotFound
	}
	u := tmdb.ImageURL(r.env.imageBase, tmdbStillSize, e.Still)
	if u == "" {
		return Artwork{}, ErrNotFound
	}
	return a.tmdbPicture(ctx, r.env, u)
}

// TMDBEpisodes returns TMDB's episode details for one season of the matched
// series, keyed by episode number. Season numbers are the provider's. The map
// is a lookup table: callers decorate the episodes they already list and never
// add entries from it. For an episode item only that episode is returned.
// The map is empty on any problem.
func (a *App) TMDBEpisodes(ctx context.Context, sourceID string, item stalker.CatalogItem, season int) (map[int]TMDBEpisode, error) {
	out := map[int]TMDBEpisode{}
	if season <= 0 {
		return out, nil
	}
	want := 0
	if item.Kind == "episode" {
		n, err := strconv.Atoi(strings.TrimSpace(cmp.Or(item.Episode, item.Number)))
		if err != nil || n <= 0 {
			return out, nil
		}
		want = n
	}
	r, err := a.tmdbResolve(ctx, sourceID, item)
	if err != nil || r == nil || r.title.Type != "tv" {
		return out, err
	}
	t := a.tmdbSvc()
	key := strconv.Itoa(r.title.ID) + "/" + strconv.Itoa(season)
	eps, ok := t.seasons.Get(key)
	if !ok {
		eps, err = t.episodes.Do(ctx, genKey(r.env.gen, key), tmdbLookupTimeout, func(ctx context.Context) (map[int]TMDBEpisode, error) {
			list, err := r.env.client.Season(ctx, r.title.ID, season)
			if errors.Is(err, tmdb.ErrNotFound) {
				list, err = nil, nil // the provider has a season TMDB does not know
			}
			if err != nil {
				return nil, err
			}
			m := make(map[int]TMDBEpisode, len(list))
			for _, e := range list {
				m[e.Number] = TMDBEpisode{Number: e.Number, Name: e.Name, Overview: e.Overview, Runtime: e.Runtime, Still: e.Still}
			}
			ttl := tmdbPositiveTTL
			if len(m) == 0 {
				ttl = tmdbNegativeTTL
			}
			t.keep(r.env.gen, func() { t.seasons.Put(key, m, ttl) })
			return m, nil
		})
		if err != nil {
			return out, a.tmdbNote(err)
		}
	}
	for n, e := range eps {
		if want == 0 || n == want {
			out[n] = e
		}
	}
	return out, nil
}

// TMDBStatus is the masked configuration: never the key itself.
func (a *App) TMDBStatus() TMDBStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return tmdbStatus(a.st.TMDB)
}

func tmdbStatus(c TMDBConfig) TMDBStatus {
	st := TMDBStatus{Enabled: c.Enabled, Language: tmdbLanguage(c), KeySet: c.APIKey != ""}
	if st.KeySet {
		st.KeyHint = maskTMDBKey(c.APIKey)
	}
	return st
}

func maskTMDBKey(k string) string {
	if len(k) < 12 {
		return "••••"
	}
	return "••••" + k[len(k)-4:]
}

// ---- resolving ----

func (a *App) tmdbResolve(ctx context.Context, sourceID string, item stalker.CatalogItem) (*tmdbResolved, error) {
	rk, ok := tmdbRoot(sourceID, item)
	if !ok {
		return nil, nil
	}
	env, ov, ok := a.tmdbEnv(&rk)
	if !ok {
		return nil, nil
	}
	t := a.tmdbSvc()
	if m, ok := t.matches.Get(rk.String()); ok {
		if m.id == 0 {
			return nil, nil
		}
		title, err := a.tmdbTitle(ctx, env, m.kind, m.id)
		if err != nil {
			return nil, a.tmdbNote(err)
		}
		return &tmdbResolved{title: title, how: m.how, env: env}, nil
	}
	// Grid pages ask for the same entry from several places at once.
	r, err := t.lookups.Do(ctx, genKey(env.gen, rk.String()), tmdbLookupTimeout, func(ctx context.Context) (*tmdbResolved, error) {
		return a.tmdbLookup(ctx, env, rk, ov, item)
	})
	return r, a.tmdbNote(err)
}

func (a *App) tmdbLookup(ctx context.Context, env tmdbEnv, rk tmdbRootKey, ov *TMDBOverride, item stalker.CatalogItem) (*tmdbResolved, error) {
	t := a.tmdbSvc()
	remember := func(m tmdbMatch) {
		ttl := tmdbPositiveTTL
		if m.id == 0 {
			ttl = tmdbNegativeTTL
		}
		t.keep(env.gen, func() { t.matches.Put(rk.String(), m, ttl) })
	}
	if ov != nil {
		title, err := a.tmdbTitle(ctx, env, ov.Kind, ov.ID)
		if errors.Is(err, tmdb.ErrNotFound) {
			// The record chosen by hand is gone from TMDB. Show the provider's
			// data rather than second-guess the viewer with an automatic match.
			remember(tmdbMatch{})
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		remember(tmdbMatch{kind: ov.Kind, id: ov.ID, how: "manual"})
		return &tmdbResolved{title: title, how: "manual", env: env}, nil
	}
	root, ok := a.tmdbRootItem(rk, item)
	if !ok {
		return nil, nil // a season of a series never browsed: nothing to search with
	}
	if id := explicitTMDBID(root.TMDBID); id > 0 {
		title, err := a.tmdbTitle(ctx, env, rk.kind, id)
		if err == nil {
			remember(tmdbMatch{kind: rk.kind, id: id, how: "explicit"})
			return &tmdbResolved{title: title, how: "explicit", env: env}, nil
		}
		if !errors.Is(err, tmdb.ErrNotFound) {
			return nil, err
		}
		// A stale or mistyped id on the portal: search instead.
	}
	kind := rk.kind
	ranked, err := a.tmdbSearch(ctx, env, tmdb.NewQuery(kind, root.Name, root.OriginalName, root.Year))
	if err != nil {
		return nil, err
	}
	best, _ := tmdb.Pick(ranked)
	if best == nil && kind == "tv" && rk.typ == "vod" {
		// Portals file two-part films and miniseries as VOD "series": when no
		// show fits, a film may.
		kind = "movie"
		if ranked, err = a.tmdbSearch(ctx, env, tmdb.NewQuery(kind, root.Name, root.OriginalName, root.Year)); err != nil {
			return nil, err
		}
		best, _ = tmdb.Pick(ranked)
	}
	if best == nil {
		remember(tmdbMatch{})
		return nil, nil
	}
	title, err := a.tmdbTitle(ctx, env, kind, best.ID)
	switch {
	case errors.Is(err, tmdb.ErrNotFound):
		remember(tmdbMatch{})
		return nil, nil
	case err != nil:
		title = best.Title // details are a bonus: the search row has title, overview and pictures
	}
	remember(tmdbMatch{kind: kind, id: best.ID, how: "auto"})
	return &tmdbResolved{title: title, how: "auto", env: env}, nil
}

func explicitTMDBID(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 || n > 1e9 {
		return 0
	}
	return n
}

// tmdbSearch asks TMDB with the provider title, then with the original title
// when the first answer is not conclusive.
func (a *App) tmdbSearch(ctx context.Context, env tmdbEnv, q tmdb.Query) ([]tmdb.Candidate, error) {
	var titles []tmdb.Title
	var ranked []tmdb.Candidate
	seen := map[int]bool{}
	for _, text := range q.Search {
		res, err := env.client.Search(ctx, q.Type, text)
		if err != nil {
			return nil, err // a partial answer must not become a cached "no match"
		}
		for _, r := range res {
			if !seen[r.ID] {
				seen[r.ID] = true
				titles = append(titles, r)
			}
		}
		ranked = tmdb.Rank(q, titles)
		if best, _ := tmdb.Pick(ranked); best != nil {
			break
		}
	}
	return ranked, nil
}

func (a *App) tmdbTitle(ctx context.Context, env tmdbEnv, kind string, id int) (tmdb.Title, error) {
	t := a.tmdbSvc()
	key := kind + "/" + strconv.Itoa(id)
	if v, ok := t.titles.Get(key); ok {
		return v, nil
	}
	return t.details.Do(ctx, genKey(env.gen, key), tmdbLookupTimeout, func(ctx context.Context) (tmdb.Title, error) {
		v, err := env.client.Details(ctx, kind, id)
		if err == nil {
			t.keep(env.gen, func() { t.titles.Put(key, v, tmdbPositiveTTL) })
		}
		return v, err
	})
}

func (a *App) tmdbPicture(ctx context.Context, env tmdbEnv, u string) (Artwork, error) {
	t := a.tmdbSvc()
	if art, ok := t.pictures.Get(u); ok {
		return art, nil
	}
	art, err := t.downloads.Do(ctx, u, 2*tmdbRequestTimeout, func(ctx context.Context) (Artwork, error) {
		select {
		case t.images <- struct{}{}:
			defer func() { <-t.images }()
		case <-ctx.Done():
			return Artwork{}, tmdb.ErrTimeout
		}
		data, err := tmdb.Download(ctx, env.hc, u, env.userAgent, tmdbImageMax)
		if err != nil {
			return Artwork{}, err
		}
		art, err := passArtwork(data)
		if err != nil {
			return Artwork{}, err
		}
		t.keep(env.gen, func() { t.pictures.Put(u, art, tmdbPositiveTTL) })
		return art, nil
	})
	if errors.Is(err, tmdb.ErrNotFound) {
		return Artwork{}, ErrNotFound
	}
	if err != nil {
		return Artwork{}, a.tmdbNote(err)
	}
	return art, nil
}

// passArtwork checks a TMDB picture by its header only. Decoding or scaling in
// Go takes a soft-float MIPS tens of seconds; Enigma2's ePicLoad scales JPEG
// and PNG itself, so they pass through untouched.
func passArtwork(data []byte) (Artwork, error) {
	if len(data) > tmdbImageMax {
		return Artwork{}, errTMDBArtwork
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width <= 0 || cfg.Height <= 0 ||
		cfg.Width > artworkMaxPassSide || cfg.Height > artworkMaxPassSide {
		return Artwork{}, errTMDBArtwork
	}
	return Artwork{Data: data, ContentType: "image/" + format}, nil
}

func tmdbInfo(t tmdb.Title, how string) TMDBInfo {
	info := TMDBInfo{ID: t.ID, Type: t.Type, Title: t.Title, Overview: t.Overview, Runtime: t.Runtime,
		Votes: t.VoteCount, HasPoster: t.PosterPath != "", HasBackdrop: t.BackdropPath != "", Match: how}
	if t.OriginalTitle != t.Title {
		info.OriginalTitle = t.OriginalTitle
	}
	if y := t.Year(); y > 0 {
		info.Year = strconv.Itoa(y)
	}
	if len(t.Genres) > 0 {
		info.Genres = slices.Clone(t.Genres)
	}
	if t.VoteCount >= 10 && t.VoteAverage > 0 {
		info.Rating = strconv.FormatFloat(math.Round(t.VoteAverage*10)/10, 'f', 1, 64)
	}
	for _, p := range t.Cast {
		info.Cast = append(info.Cast, TMDBPerson{Name: p.Name, Character: p.Character, HasPhoto: p.ProfilePath != ""})
	}
	info.Directors = slices.Clone(t.Directors)
	return info
}

// tmdbNote logs TMDB trouble without flooding the receiver's log, and makes
// sure only known, key-free errors travel further.
func (a *App) tmdbNote(err error) error {
	if errors.Is(err, errTMDBArtwork) {
		return errTMDBArtwork
	}
	if err = tmdb.Sanitize(err); err == nil || err == context.Canceled {
		return err
	}
	t := a.tmdbSvc()
	t.logMu.Lock()
	quiet := err == t.lastErr && time.Since(t.lastLog) < 10*time.Minute
	if !quiet {
		t.lastErr, t.lastLog = err, time.Now()
	}
	t.logMu.Unlock()
	if !quiet {
		a.log.Printf("TMDB: %v", err)
	}
	return err
}

// ---- settings ----

// TMDBUpdate is PUT /api/tmdb. Absent fields keep their value; an empty key
// keeps the stored one (the WebUI never receives it to send back).
type TMDBUpdate struct {
	APIKey   *string `json:"apiKey"`
	Language *string `json:"language"`
	Enabled  *bool   `json:"enabled"`
	ClearKey bool    `json:"clearKey"`
}

var tmdbLanguageRe = regexp.MustCompile(`^[a-z]{2}-[A-Z]{2}$`)

func (a *App) UpdateTMDB(in TMDBUpdate) (TMDBStatus, error) {
	key := ""
	if in.APIKey != nil {
		key = strings.TrimSpace(*in.APIKey)
		if key != "" && !tmdb.ValidKey(key) {
			return a.TMDBStatus(), errTMDBKeyFormat
		}
	}
	if in.Language != nil && !tmdbLanguageRe.MatchString(strings.TrimSpace(*in.Language)) {
		return a.TMDBStatus(), errTMDBLanguage
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.st.TMDB
	next := prev
	if in.ClearKey {
		next.APIKey = ""
	}
	if key != "" {
		if next.APIKey == "" && in.Enabled == nil {
			next.Enabled = true // a first key means "use it"
		}
		next.APIKey = key
	}
	if in.Language != nil {
		next.Language = strings.TrimSpace(*in.Language)
	}
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	if next.APIKey == prev.APIKey && tmdbLanguage(next) == tmdbLanguage(prev) && next.Enabled == prev.Enabled {
		return tmdbStatus(prev), nil // nothing to write to flash
	}
	a.st.TMDB = next
	// A new key or language invalidates everything learned; switching off frees the memory.
	if next.APIKey != prev.APIKey || tmdbLanguage(next) != tmdbLanguage(prev) || !next.Enabled {
		a.tmdbSvc().reset()
	}
	st := tmdbStatus(next)
	if err := a.save(); err != nil {
		return st, errors.New("не удалось сохранить настройки TMDB")
	}
	a.log.Printf("TMDB: включён=%v, язык %s, ключ задан=%v", st.Enabled, st.Language, st.KeySet)
	return st, nil
}

// TMDBTestResult is POST /api/tmdb/test.
type TMDBTestResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// TestTMDB checks a key (the typed one, or the stored one when empty) with
// one authenticated request. It ignores the backoff and never trips it.
func (a *App) TestTMDB(ctx context.Context, apiKey string) TMDBTestResult {
	key := strings.TrimSpace(apiKey)
	a.mu.Lock()
	stored := a.st.TMDB.APIKey
	env := a.tmdbEnvLocked()
	a.mu.Unlock()
	if key == "" {
		key = stored
	}
	if key == "" {
		return TMDBTestResult{Error: "Сначала вставьте ключ API"}
	}
	if !tmdb.ValidKey(key) {
		return TMDBTestResult{Error: errTMDBKeyFormat.Error()}
	}
	c := *env.client
	c.Key, c.Probe = key, true
	err := c.Check(ctx)
	if err != nil {
		return TMDBTestResult{Error: tmdbTestMessage(err)}
	}
	if key == stored {
		a.tmdbSvc().limiter.Reset() // the saved key works: stop backing off
	}
	return TMDBTestResult{OK: true}
}

func tmdbTestMessage(err error) string {
	switch {
	case errors.Is(err, tmdb.ErrUnauthorized):
		return "Ключ не подошёл: TMDB его не принял. Скопируйте «Ключ API» (v3) из настроек аккаунта themoviedb.org → API."
	case errors.Is(err, tmdb.ErrRateLimited):
		return "TMDB просит подождать: слишком много запросов. Попробуйте через минуту."
	case errors.Is(err, tmdb.ErrNetwork), errors.Is(err, tmdb.ErrTimeout), errors.Is(err, context.Canceled):
		return "Нет связи с TMDB — проверьте, что у ресивера есть интернет."
	case errors.Is(err, tmdb.ErrUnavailable):
		return "TMDB временно недоступен — попробуйте позже."
	}
	return "TMDB прислал непонятный ответ — попробуйте позже."
}

// ---- manual matching (WebUI) ----

type TMDBMatchItem struct {
	Name         string `json:"name"`
	OriginalName string `json:"originalName,omitempty"`
	Year         string `json:"year,omitempty"`
	Kind         string `json:"kind"`
}

type TMDBCandidate struct {
	ID            int    `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	Year          string `json:"year,omitempty"`
	Score         int    `json:"score"`
	Poster        string `json:"poster"` // full w185 URL for the tablet browser, may be ""
	Overview      string `json:"overview,omitempty"`
}

type TMDBMatchView struct {
	Item       TMDBMatchItem   `json:"item"`
	Current    *TMDBInfo       `json:"current"`
	Candidates []TMDBCandidate `json:"candidates"`
	Reason     string          `json:"reason"`          // matched | ambiguous | none | disabled | error
	Manual     bool            `json:"manual"`          // a match chosen by hand is stored
	Error      string          `json:"error,omitempty"` // why, for "error" and a vanished manual match
}

// tmdbCatalogEntry resolves a catalog navigation key to its item and root.
func (a *App) tmdbCatalogEntry(ctx context.Context, sourceID, key string) (stalker.CatalogItem, tmdbRootKey, error) {
	if sourceID == "" || key == "" {
		return stalker.CatalogItem{}, tmdbRootKey{}, ErrCatalogInput
	}
	d, err := a.CatalogDetails(ctx, sourceID, key)
	if err != nil {
		return stalker.CatalogItem{}, tmdbRootKey{}, err
	}
	rk, ok := tmdbRoot(sourceID, d.CatalogItem)
	if !ok {
		return stalker.CatalogItem{}, tmdbRootKey{}, errTMDBNotMatchable
	}
	return d.CatalogItem, rk, nil
}

// TMDBMatch shows the current match of a catalog entry and the candidates
// TMDB offers for it.
func (a *App) TMDBMatch(ctx context.Context, sourceID, key string) (TMDBMatchView, error) {
	item, rk, err := a.tmdbCatalogEntry(ctx, sourceID, key)
	if err != nil {
		return TMDBMatchView{}, err
	}
	root, ok := a.tmdbRootItem(rk, item)
	if !ok {
		root = item
	}
	view := TMDBMatchView{Item: TMDBMatchItem{Name: root.Name, OriginalName: root.OriginalName, Year: root.Year, Kind: root.Kind}, Candidates: []TMDBCandidate{}}
	env, ov, enabled := a.tmdbEnv(&rk)
	if !enabled {
		view.Reason = "disabled"
		return view, nil
	}
	view.Manual = ov != nil
	cur, curErr := a.TMDBFor(ctx, sourceID, item)
	view.Current = cur
	ranked, searchErr := a.tmdbSearch(ctx, env, tmdb.NewQuery(rk.kind, root.Name, root.OriginalName, root.Year))
	view.Candidates = tmdbCandidates(env.imageBase, ranked)
	switch {
	case cur != nil:
		view.Reason = "matched"
	case curErr != nil || searchErr != nil:
		view.Reason = "error"
		view.Error = a.tmdbNote(cmp.Or(curErr, searchErr)).Error()
	default:
		_, view.Reason = tmdb.Pick(ranked)
		if ov != nil {
			view.Error = "Выбранная вручную запись больше не существует в TMDB — выберите другую или сбросьте выбор."
		}
	}
	return view, nil
}

func tmdbCandidates(imageBase string, ranked []tmdb.Candidate) []TMDBCandidate {
	out := make([]TMDBCandidate, 0, min(len(ranked), tmdbMaxCandidates))
	for _, c := range ranked {
		if len(out) == tmdbMaxCandidates {
			break
		}
		tc := TMDBCandidate{ID: c.ID, Type: c.Type, Title: c.Title.Title, Score: c.Score, Poster: tmdb.ImageURL(imageBase, tmdbThumbSize, c.PosterPath)}
		if c.OriginalTitle != c.Title.Title {
			tc.OriginalTitle = c.OriginalTitle
		}
		if y := c.Year(); y > 0 {
			tc.Year = strconv.Itoa(y)
		}
		if ov := []rune(c.Overview); len(ov) > 220 {
			tc.Overview = strings.TrimSpace(string(ov[:220])) + "…"
		} else {
			tc.Overview = c.Overview
		}
		out = append(out, tc)
	}
	return out
}

var tmdbLinkRe = regexp.MustCompile(`^https?://(?:www\.)?themoviedb\.org/(movie|tv)/(\d{1,9})(?:[-/?#].*)?$`)

// TMDBSearch is the free-text search of the manual matching screen. A link to
// a themoviedb.org page finds exactly that record.
func (a *App) TMDBSearch(ctx context.Context, query, kind, year string) ([]TMDBCandidate, error) {
	query = strings.TrimSpace(query)
	if kind != "movie" && kind != "tv" {
		return nil, errors.New("тип должен быть movie или tv")
	}
	if query == "" {
		return []TMDBCandidate{}, nil
	}
	env, _, ok := a.tmdbEnv(nil)
	if !ok {
		return nil, ErrTMDBDisabled
	}
	if m := tmdbLinkRe.FindStringSubmatch(query); m != nil {
		id, _ := strconv.Atoi(m[2])
		t, err := a.tmdbTitle(ctx, env, m[1], id)
		if errors.Is(err, tmdb.ErrNotFound) {
			return []TMDBCandidate{}, nil
		}
		if err != nil {
			return nil, a.tmdbNote(err)
		}
		return tmdbCandidates(env.imageBase, []tmdb.Candidate{{Title: t, Score: 100}}), nil
	}
	res, err := env.client.Search(ctx, kind, query)
	if err != nil {
		return nil, a.tmdbNote(err)
	}
	return tmdbCandidates(env.imageBase, tmdb.Rank(tmdb.NewQuery(kind, query, "", year), res)), nil
}

// SetTMDBMatch stores a match chosen by hand, after checking that TMDB has
// the record. It survives restarts and catalog reloads.
func (a *App) SetTMDBMatch(ctx context.Context, sourceID, key string, id int, kind string) (*TMDBInfo, error) {
	if (kind != "movie" && kind != "tv") || id <= 0 || id > 1e9 {
		return nil, errTMDBBadChoice
	}
	_, rk, err := a.tmdbCatalogEntry(ctx, sourceID, key)
	if err != nil {
		return nil, err
	}
	env, _, ok := a.tmdbEnv(&rk)
	if !ok {
		return nil, ErrTMDBDisabled
	}
	title, err := a.tmdbTitle(ctx, env, kind, id)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, errors.New("в TMDB нет такой записи")
	}
	if err != nil {
		return nil, a.tmdbNote(err)
	}
	if err := a.tmdbSetOverride(rk, &TMDBOverride{Source: rk.source, Type: rk.typ, Root: rk.root, ID: id, Kind: kind}); err != nil {
		return nil, err
	}
	info := tmdbInfo(title, "manual")
	return &info, nil
}

// ResetTMDBMatch returns an entry to automatic matching.
func (a *App) ResetTMDBMatch(ctx context.Context, sourceID, key string) error {
	_, rk, err := a.tmdbCatalogEntry(ctx, sourceID, key)
	if err != nil {
		return err
	}
	return a.tmdbSetOverride(rk, nil)
}

// tmdbSetOverride stores (ov != nil) or removes the manual match of one entry
// and forgets the cached decision. state.json is written only on a change.
func (a *App) tmdbSetOverride(rk tmdbRootKey, ov *TMDBOverride) error {
	t := a.tmdbSvc()
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.st.TMDB.Overrides
	list := make([]TMDBOverride, 0, len(old)+1)
	for _, o := range old {
		if a.source(o.Source) == nil || (o.Source == rk.source && o.Type == rk.typ && o.Root == rk.root) {
			continue // portal deleted, or the entry being changed
		}
		list = append(list, o)
	}
	if ov != nil {
		list = append(list, *ov)
	}
	if len(list) > tmdbMaxOverrides {
		list = list[len(list)-tmdbMaxOverrides:]
	}
	t.forget(rk)
	if slices.Equal(list, old) {
		return nil
	}
	if len(list) == 0 {
		list = nil
	}
	a.st.TMDB.Overrides = list
	if err := a.save(); err != nil {
		return errors.New("не удалось сохранить выбор")
	}
	return nil
}
