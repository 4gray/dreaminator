package tmdb_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dreaminator/internal/tmdb"
	"dreaminator/internal/tmdb/tmdbtest"
)

const key = "SECRETkey0123456789SECRETkey0123"

func client(s *tmdbtest.Server) *tmdb.Client {
	return &tmdb.Client{HTTP: &http.Client{Timeout: 2 * time.Second}, APIBase: s.APIBase, Key: key, Language: "ru-RU", Limiter: tmdb.NewLimiter(2)}
}

func noKey(t *testing.T, err error) {
	t.Helper()
	if err != nil && (strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "api_key")) {
		t.Fatalf("error leaks the request: %v", err)
	}
}

func TestClientSearchAndDetails(t *testing.T) {
	s := tmdbtest.New(key)
	defer s.Close()
	s.AddMovie(tmdbtest.Record{ID: 603, Title: "Матрица", Original: "The Matrix", Date: "1999-03-30", Runtime: 136, Genres: []string{"боевик"}, Vote: 8.2, Votes: 25000, Poster: "/p.jpg", Backdrop: "../../etc/passwd"})
	c := client(s)
	res, err := c.Search(context.Background(), "movie", "The Matrix")
	if err != nil || len(res) != 1 || res[0].ID != 603 || res[0].Year() != 1999 || res[0].PosterPath != "/p.jpg" {
		t.Fatalf("search: %+v %v", res, err)
	}
	if res[0].BackdropPath != "" {
		t.Fatal("a picture path that is not a plain TMDB file name must be dropped")
	}
	d, err := c.Details(context.Background(), "movie", 603)
	if err != nil || d.Runtime != 136 || d.Genres[0] != "боевик" || d.OriginalTitle != "The Matrix" {
		t.Fatalf("details: %+v %v", d, err)
	}
	if _, err := c.Details(context.Background(), "movie", 1); !errors.Is(err, tmdb.ErrNotFound) {
		t.Fatalf("missing record: %v", err)
	}
	if langs := s.Languages(); langs[0] != "ru-RU" {
		t.Fatalf("language not sent: %v", langs)
	}
}

func TestClientFailuresAreSentinelsWithoutTheKey(t *testing.T) {
	s := tmdbtest.New(key)
	defer s.Close()
	ctx := context.Background()

	bad := client(s)
	bad.Key = "WRONGkey0123456789WRONGkey012345"
	_, err := bad.Search(ctx, "movie", "x")
	if !errors.Is(err, tmdb.ErrUnauthorized) {
		t.Fatalf("401: %v", err)
	}
	noKey(t, err)
	before := s.Hits("")
	if _, err := bad.Search(ctx, "movie", "y"); !errors.Is(err, tmdb.ErrUnauthorized) || s.Hits("") != before {
		t.Fatalf("a rejected key must not be retried at once: %v, %d requests", err, s.Hits("")-before)
	}
	bad.Limiter.Reset()

	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusInternalServerError, "", tmdb.ErrUnavailable},
		{http.StatusBadGateway, "", tmdb.ErrUnavailable},
		{0, "<html>maintenance</html>", tmdb.ErrMalformed},
		{0, `{"results": "nope"}`, tmdb.ErrMalformed},
	} {
		s.Set(func(s *tmdbtest.Server) { s.Status, s.Body = tc.status, tc.body })
		c := client(s)
		_, err := c.Search(ctx, "movie", "x")
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d body %q: %v", tc.status, tc.body, err)
		}
		noKey(t, err)
	}

	s.Set(func(s *tmdbtest.Server) { s.Status, s.Body, s.RetryAfter = http.StatusTooManyRequests, "", "30" })
	c := client(s)
	if _, err := c.Search(ctx, "movie", "x"); !errors.Is(err, tmdb.ErrRateLimited) {
		t.Fatalf("429: %v", err)
	}
	before = s.Hits("")
	if _, err := c.Search(ctx, "movie", "x"); !errors.Is(err, tmdb.ErrRateLimited) || s.Hits("") != before {
		t.Fatal("Retry-After must keep requests away")
	}
	probe := *c
	probe.Probe = true
	s.Set(func(s *tmdbtest.Server) { s.Status, s.RetryAfter = 0, "" })
	if err := probe.Check(ctx); err != nil {
		t.Fatalf("a connection test ignores the backoff: %v", err)
	}

	s.Set(func(s *tmdbtest.Server) { s.Delay = 500 * time.Millisecond })
	slow := client(s)
	slow.HTTP = &http.Client{Timeout: 100 * time.Millisecond}
	_, err = slow.Search(ctx, "movie", "x")
	if !errors.Is(err, tmdb.ErrTimeout) {
		t.Fatalf("timeout: %v", err)
	}
	noKey(t, err)

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	down := client(s)
	down.APIBase = "http://" + addr + "/3"
	_, err = down.Search(ctx, "movie", "x")
	if !errors.Is(err, tmdb.ErrNetwork) {
		t.Fatalf("refused: %v", err)
	}
	noKey(t, err)
	if !tmdb.IsServiceError(err) || tmdb.IsServiceError(tmdb.ErrNotFound) {
		t.Fatal("IsServiceError")
	}
}

func TestLimiterCapsConcurrency(t *testing.T) {
	var inFlight, peak atomic.Int32
	s := tmdbtest.New(key)
	defer s.Close()
	s.Set(func(s *tmdbtest.Server) { s.Delay = 30 * time.Millisecond })
	c := client(s)
	hc := c.HTTP
	c.HTTP = &http.Client{Timeout: hc.Timeout, Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Search(context.Background(), "movie", fmt.Sprint("q", i))
		}(i)
	}
	wg.Wait()
	if peak.Load() > 2 {
		t.Fatalf("%d requests at once", peak.Load())
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImageURLAndDownload(t *testing.T) {
	if u := tmdb.ImageURL("", "w342", "/abc.jpg"); u != "https://image.tmdb.org/t/p/w342/abc.jpg" {
		t.Fatal(u)
	}
	for _, p := range []string{"", "abc.jpg", "/../x.jpg", "//evil.example/x.jpg", "/x.svg", "/x.jpg?a=b"} {
		if tmdb.ImageURL("", "w342", p) != "" {
			t.Errorf("accepted path %q", p)
		}
	}
	s := tmdbtest.New(key)
	defer s.Close()
	s.AddImage("/big.jpg", make([]byte, 2000))
	if _, err := tmdb.Download(context.Background(), http.DefaultClient, s.ImageBase+"/w342/big.jpg", "", 1000); !errors.Is(err, tmdb.ErrTooLarge) {
		t.Fatalf("size cap: %v", err)
	}
	if _, err := tmdb.Download(context.Background(), http.DefaultClient, s.ImageBase+"/w342/none.jpg", "", 1000); !errors.Is(err, tmdb.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestCacheIsBounded(t *testing.T) {
	c := tmdb.NewCache[int](1024, 0, nil)
	for i := 0; i < 5000; i++ {
		c.Put(fmt.Sprint(i), i, time.Hour)
	}
	if c.Len() != 1024 {
		t.Fatalf("%d entries", c.Len())
	}
	if _, ok := c.Get("0"); ok {
		t.Fatal("oldest entry must be evicted")
	}
	if v, ok := c.Get("4999"); !ok || v != 4999 {
		t.Fatal("newest entry lost")
	}

	b := tmdb.NewCache(1000, 4<<20, func(p []byte) int { return len(p) })
	for i := 0; i < 100; i++ {
		b.Put(fmt.Sprint(i), make([]byte, 100<<10), time.Hour)
	}
	if b.Bytes() > 4<<20 {
		t.Fatalf("%d bytes kept", b.Bytes())
	}
	b.Put("huge", make([]byte, 5<<20), time.Hour)
	if _, ok := b.Get("huge"); ok {
		t.Fatal("a value over the budget must not be kept")
	}

	e := tmdb.NewCache[int](10, 0, nil)
	e.Put("short", 1, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, ok := e.Get("short"); ok || e.Len() != 0 {
		t.Fatal("expired entries must go")
	}
}

func TestGroupDeduplicates(t *testing.T) {
	var g tmdb.Group[int]
	var calls atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := g.Do(context.Background(), "k", time.Second, func(context.Context) (int, error) {
				calls.Add(1)
				<-release
				return 7, nil
			})
			if v != 7 || err != nil {
				t.Error(v, err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("%d calls", calls.Load())
	}
	// A panic becomes an error, not a crash.
	if _, err := g.Do(context.Background(), "p", time.Second, func(context.Context) (int, error) { panic("boom") }); err == nil {
		t.Fatal("panic swallowed silently")
	}
	// A caller that gives up returns at once; the work goes on for others.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Do(ctx, "c", time.Second, func(context.Context) (int, error) { time.Sleep(10 * time.Millisecond); return 1, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller: %v", err)
	}
}
