// Package tmdb talks to The Movie Database API v3 (search, details, seasons,
// pictures) and decides whether a provider catalog entry and a TMDB record
// are the same work.
//
// The API key travels in the query string, and *url.Error and friends print
// the whole request URL. So nothing in this package wraps a transport error:
// every failure becomes one of the sentinel errors below, which are safe to
// log, to show and to put into JSON.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	DefaultAPIBase   = "https://api.themoviedb.org/3"
	DefaultImageBase = "https://image.tmdb.org/t/p"

	maxJSON       = 2 << 20 // a season with long overviews is ~100 KB
	maxResults    = 20
	maxOverview   = 2000
	maxGenres     = 6
	maxEpisodes   = 500
	maxBackoff    = 5 * time.Minute
	rateBackoff   = 10 * time.Second // 429 without a usable Retry-After
	serverBackoff = 15 * time.Second
	netBackoff    = 30 * time.Second
	// A rejected key stays rejected until it changes or passes a test; retry
	// now and then in case TMDB itself had a bad moment.
	authBackoff = 30 * time.Minute
)

var (
	ErrNoKey        = errors.New("не задан ключ API TMDB")
	ErrUnauthorized = errors.New("TMDB не принял ключ API — проверьте его в настройках")
	ErrNotFound     = errors.New("в TMDB такой записи нет")
	ErrRateLimited  = errors.New("TMDB просит подождать: слишком много запросов")
	ErrUnavailable  = errors.New("TMDB временно недоступен")
	ErrNetwork      = errors.New("нет связи с TMDB — проверьте интернет на ресивере")
	ErrTimeout      = errors.New("TMDB не ответил вовремя")
	ErrMalformed    = errors.New("TMDB прислал непонятный ответ")
	ErrTooLarge     = errors.New("ответ TMDB слишком большой")
)

var serviceErrors = []error{ErrNoKey, ErrUnauthorized, ErrRateLimited, ErrUnavailable, ErrNetwork, ErrTimeout, ErrMalformed, ErrTooLarge}

// IsServiceError reports whether err came from talking to TMDB (as opposed to
// a bad request of ours).
func IsServiceError(err error) bool {
	for _, e := range serviceErrors {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// ErrFailed stands in for any error Sanitize does not know.
var ErrFailed = errors.New("TMDB: запрос не удался")

// Sanitize reduces err to the bare sentinel it matches, dropping anything a
// wrapper may have added (such as a request URL with the key). nil stays nil.
func Sanitize(err error) error {
	if err == nil {
		return nil
	}
	for _, e := range append([]error{ErrNotFound, context.Canceled}, serviceErrors...) {
		if errors.Is(err, e) {
			return e
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ErrFailed
}

// Limiter caps concurrent API requests and keeps everybody away from TMDB for
// a while after it said "slow down", failed, or rejected the key.
type Limiter struct {
	slots  chan struct{}
	mu     sync.Mutex
	until  time.Time
	reason error
	now    func() time.Time
}

func NewLimiter(concurrent int) *Limiter {
	return &Limiter{slots: make(chan struct{}, max(1, concurrent)), now: time.Now}
}

// Reset forgets backoff and a rejected key (new key, successful test).
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.until, l.reason = time.Time{}, nil
}

func (l *Limiter) blocked() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reason != nil && l.now().Before(l.until) {
		return l.reason
	}
	return nil
}

func (l *Limiter) trip(err error, d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err != ErrUnauthorized {
		d = min(d, maxBackoff)
	}
	if until := l.now().Add(d); until.After(l.until) || err == ErrUnauthorized {
		l.until = until
	}
	l.reason = err
}

func (l *Limiter) acquire(ctx context.Context, probe bool) (func(), error) {
	if !probe {
		if err := l.blocked(); err != nil {
			return nil, err
		}
	}
	select {
	case l.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctxError(ctx)
	}
	release := func() { <-l.slots }
	// Somebody may have tripped the backoff while we waited for a slot.
	if !probe {
		if err := l.blocked(); err != nil {
			release()
			return nil, err
		}
	}
	return release, nil
}

// Client is one configured view of the API. It is cheap: build one per lookup
// from the current settings and share the HTTP client and Limiter.
type Client struct {
	HTTP      *http.Client
	APIBase   string
	Key       string
	Language  string
	UserAgent string
	Limiter   *Limiter
	// Probe marks a connection test: it ignores the backoff and never trips
	// it, so testing a mistyped key does not block the saved one.
	Probe bool
}

// v4 "API Read Access Tokens" are JWTs and go into the Authorization header.
func isBearer(key string) bool { return strings.HasPrefix(key, "eyJ") && strings.Count(key, ".") == 2 }

var (
	v3KeyRe = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)
	v4KeyRe = regexp.MustCompile(`^eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+$`)
)

// ValidKey accepts what TMDB hands out: a v3 API key (32 hex characters) or a
// v4 read access token. Anything else would only produce a confusing 401.
func ValidKey(key string) bool {
	return v3KeyRe.MatchString(key) || (len(key) <= 1024 && v4KeyRe.MatchString(key))
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if c.Key == "" {
		return ErrNoKey
	}
	if c.Limiter != nil {
		release, err := c.Limiter.acquire(ctx, c.Probe)
		if err != nil {
			return err
		}
		defer release()
	}
	if q == nil {
		q = url.Values{}
	}
	bearer := isBearer(c.Key)
	if !bearer {
		q.Set("api_key", c.Key)
	}
	if c.Language != "" {
		q.Set("language", c.Language)
	}
	base := c.APIBase
	if base == "" {
		base = DefaultAPIBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path+"?"+q.Encode(), nil)
	if err != nil {
		return ErrMalformed // only a broken base URL gets here; never echo it
	}
	req.Header.Set("Accept", "application/json")
	if bearer {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return c.fail(ctx, transportError(ctx, err), netBackoff)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusUnauthorized:
		return c.fail(ctx, ErrUnauthorized, authBackoff)
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return c.fail(ctx, ErrRateLimited, retryAfter(resp.Header.Get("Retry-After"), time.Now()))
	case resp.StatusCode >= 500:
		return c.fail(ctx, ErrUnavailable, serverBackoff)
	default:
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSON+1))
	if err != nil {
		return c.fail(ctx, transportError(ctx, err), netBackoff)
	}
	if len(body) > maxJSON {
		return ErrTooLarge
	}
	if err := json.Unmarshal(body, out); err != nil {
		return ErrMalformed
	}
	return nil
}

// fail trips the shared backoff for real TMDB trouble, but not for our own
// callers giving up.
func (c *Client) fail(ctx context.Context, err error, d time.Duration) error {
	if c.Limiter != nil && !c.Probe && ctx.Err() == nil && err != context.Canceled {
		c.Limiter.trip(err, d)
	}
	return err
}

func ctxError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	return context.Canceled
}

// transportError reduces a transport failure to a sentinel; the original
// error contains the request URL and therefore the key.
func transportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctxError(ctx)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ErrTimeout
	}
	return ErrNetwork
}

func retryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	d := rateBackoff
	if n, err := strconv.Atoi(v); err == nil {
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = t.Sub(now)
	}
	return max(time.Second, min(d, maxBackoff))
}

// ---- records ----

// Title is a movie or a TV show, from a search row or from its details.
type Title struct {
	ID            int
	Type          string // movie | tv
	Title         string
	OriginalTitle string
	Overview      string
	Date          string // first release or first air date, YYYY-MM-DD
	Runtime       int    // minutes; tv: a typical episode
	Genres        []string
	VoteAverage   float64
	VoteCount     int
	PosterPath    string
	BackdropPath  string
	Cast          []Person // billing order, at most maxCast
	Directors     []string // films: directors; series: creators
}

// Person is one actor of a title's cast.
type Person struct {
	Name, Character, ProfilePath string
}

func (t Title) Year() int {
	if len(t.Date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(t.Date[:4])
	if err != nil || y < 1870 || y > 2200 {
		return 0
	}
	return y
}

// Size estimates retained bytes, for cache budgets.
func (t Title) Size() int {
	n := 160 + len(t.Title) + len(t.OriginalTitle) + len(t.Overview) + len(t.Date) + len(t.PosterPath) + len(t.BackdropPath)
	for _, p := range t.Cast {
		n += 48 + len(p.Name) + len(p.Character) + len(p.ProfilePath)
	}
	for _, d := range t.Directors {
		n += 16 + len(d)
	}
	for _, g := range t.Genres {
		n += 16 + len(g)
	}
	return n
}

type rawTitle struct {
	ID             int     `json:"id"`
	Title          string  `json:"title"`
	Name           string  `json:"name"`
	OriginalTitle  string  `json:"original_title"`
	OriginalName   string  `json:"original_name"`
	Overview       string  `json:"overview"`
	ReleaseDate    string  `json:"release_date"`
	FirstAirDate   string  `json:"first_air_date"`
	Runtime        int     `json:"runtime"`
	EpisodeRunTime []int   `json:"episode_run_time"`
	VoteAverage    float64 `json:"vote_average"`
	VoteCount      int     `json:"vote_count"`
	PosterPath     string  `json:"poster_path"`
	BackdropPath   string  `json:"backdrop_path"`
	Genres         []struct {
		Name string `json:"name"`
	} `json:"genres"`
	LastEpisode *struct {
		Runtime int `json:"runtime"`
	} `json:"last_episode_to_air"`
	CreatedBy []struct {
		Name string `json:"name"`
	} `json:"created_by"`
	Credits *struct {
		Cast []struct {
			Name        string `json:"name"`
			Character   string `json:"character"`
			ProfilePath string `json:"profile_path"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
}

func (r rawTitle) title(typ string) Title {
	t := Title{ID: r.ID, Type: typ, Title: r.Title, OriginalTitle: r.OriginalTitle, Date: r.ReleaseDate, Runtime: r.Runtime,
		VoteAverage: r.VoteAverage, VoteCount: r.VoteCount, Overview: clip(strings.TrimSpace(r.Overview), maxOverview)}
	if typ == "tv" {
		t.Title, t.OriginalTitle, t.Date = r.Name, r.OriginalName, r.FirstAirDate
		for _, n := range r.EpisodeRunTime {
			if n > 0 {
				t.Runtime = n
				break
			}
		}
		if t.Runtime == 0 && r.LastEpisode != nil {
			t.Runtime = r.LastEpisode.Runtime
		}
	}
	t.Title, t.OriginalTitle = clip(t.Title, 300), clip(t.OriginalTitle, 300)
	if t.Title == "" {
		t.Title = t.OriginalTitle
	}
	if t.Runtime < 0 || t.Runtime > 1000 {
		t.Runtime = 0
	}
	if t.VoteAverage < 0 || t.VoteAverage > 10 || t.VoteCount < 0 {
		t.VoteAverage, t.VoteCount = 0, 0
	}
	if len(t.Date) > 10 {
		t.Date = t.Date[:10]
	}
	for _, g := range r.Genres {
		if g.Name != "" && len(t.Genres) < maxGenres {
			t.Genres = append(t.Genres, clip(g.Name, 60))
		}
	}
	if ValidPath(r.PosterPath) {
		t.PosterPath = r.PosterPath
	}
	if ValidPath(r.BackdropPath) {
		t.BackdropPath = r.BackdropPath
	}
	if r.Credits != nil {
		for _, c := range r.Credits.Cast {
			if c.Name == "" || len(t.Cast) >= maxCast {
				continue
			}
			p := Person{Name: clip(strings.TrimSpace(c.Name), 80), Character: clip(strings.TrimSpace(c.Character), 80)}
			if ValidPath(c.ProfilePath) {
				p.ProfilePath = c.ProfilePath
			}
			t.Cast = append(t.Cast, p)
		}
	}
	// Series: creators first; then directors (films). One name once: TMDB
	// often lists a showrunner both ways.
	add := func(name string) {
		name = clip(strings.TrimSpace(name), 80)
		if name != "" && len(t.Directors) < maxDirectors && !slices.Contains(t.Directors, name) {
			t.Directors = append(t.Directors, name)
		}
	}
	if typ == "tv" {
		for _, c := range r.CreatedBy {
			add(c.Name)
		}
	}
	if r.Credits != nil && (typ != "tv" || len(t.Directors) == 0) {
		for _, c := range r.Credits.Crew {
			if c.Job == "Director" {
				add(c.Name)
			}
		}
	}
	return t
}

const (
	maxCast      = 10
	maxDirectors = 3
)

var pathRe = regexp.MustCompile(`^/[A-Za-z0-9_\-]{1,100}\.(?:jpg|jpeg|png)$`)

// ValidPath accepts TMDB picture paths ("/abc.jpg") and nothing that could
// steer an image URL elsewhere.
func ValidPath(p string) bool { return pathRe.MatchString(p) }

// ImageURL builds a CDN address, or "" for an unusable path.
func ImageURL(base, size, path string) string {
	if !ValidPath(path) {
		return ""
	}
	if base == "" {
		base = DefaultImageBase
	}
	return strings.TrimRight(base, "/") + "/" + size + path
}

func kindPath(typ string) (string, bool) {
	switch typ {
	case "movie":
		return "movie", true
	case "tv":
		return "tv", true
	}
	return "", false
}

// Search returns the first page of movies or shows for a title.
func (c *Client) Search(ctx context.Context, typ, query string) ([]Title, error) {
	kind, ok := kindPath(typ)
	query = strings.TrimSpace(query)
	if !ok || query == "" {
		return nil, nil
	}
	var out struct {
		Results []rawTitle `json:"results"`
	}
	if err := c.get(ctx, "/search/"+kind, url.Values{"query": {clip(query, 200)}, "include_adult": {"false"}, "page": {"1"}}, &out); err != nil {
		return nil, err
	}
	res := make([]Title, 0, min(len(out.Results), maxResults))
	for _, r := range out.Results {
		if r.ID > 0 && len(res) < maxResults {
			res = append(res, r.title(typ))
		}
	}
	return res, nil
}

// Details fetches one movie or show, with runtime and genre names.
func (c *Client) Details(ctx context.Context, typ string, id int) (Title, error) {
	kind, ok := kindPath(typ)
	if !ok || id <= 0 {
		return Title{}, ErrNotFound
	}
	var out rawTitle
	// One request: cast and directors ride along with the details.
	if err := c.get(ctx, "/"+kind+"/"+strconv.Itoa(id), url.Values{"append_to_response": {"credits"}}, &out); err != nil {
		return Title{}, err
	}
	if out.ID != id {
		return Title{}, ErrMalformed
	}
	return out.title(typ), nil
}

type Episode struct {
	Number   int
	Name     string
	Overview string
	Runtime  int
	Still    string // "/abc.jpg", validated
}

// Season lists the episodes TMDB knows for one season of a show.
func (c *Client) Season(ctx context.Context, tvID, season int) ([]Episode, error) {
	if tvID <= 0 || season < 0 {
		return nil, ErrNotFound
	}
	var out struct {
		Episodes []struct {
			Number   int    `json:"episode_number"`
			Name     string `json:"name"`
			Overview string `json:"overview"`
			Runtime  int    `json:"runtime"`
			Still    string `json:"still_path"`
		} `json:"episodes"`
	}
	if err := c.get(ctx, "/tv/"+strconv.Itoa(tvID)+"/season/"+strconv.Itoa(season), nil, &out); err != nil {
		return nil, err
	}
	eps := make([]Episode, 0, min(len(out.Episodes), maxEpisodes))
	for _, e := range out.Episodes {
		if e.Number <= 0 || len(eps) >= maxEpisodes {
			continue
		}
		rt := e.Runtime
		if rt < 0 || rt > 1000 {
			rt = 0
		}
		ep := Episode{Number: e.Number, Name: clip(strings.TrimSpace(e.Name), 200), Overview: clip(strings.TrimSpace(e.Overview), 1000), Runtime: rt}
		if ValidPath(e.Still) {
			ep.Still = e.Still
		}
		eps = append(eps, ep)
	}
	return eps, nil
}

// Check verifies the key with the cheapest authenticated call.
func (c *Client) Check(ctx context.Context) error {
	var out struct {
		Images *struct{} `json:"images"`
	}
	if err := c.get(ctx, "/configuration", nil, &out); err != nil {
		return err
	}
	if out.Images == nil {
		return ErrMalformed
	}
	return nil
}

// Download fetches a picture from the image CDN, refusing anything larger than
// limit. Like API calls, failures come back as sentinels only.
func Download(ctx context.Context, hc *http.Client, rawURL, userAgent string, limit int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ErrNotFound
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, ErrUnavailable
	}
	if resp.ContentLength > int64(limit) {
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return nil, transportError(ctx, err)
	}
	if len(data) > limit {
		return nil, ErrTooLarge
	}
	return data, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
