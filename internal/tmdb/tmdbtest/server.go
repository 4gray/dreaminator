// Package tmdbtest is a fake TMDB API and image CDN for tests: nothing in the
// test suite reaches the real network.
package tmdbtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"dreaminator/internal/tmdb"
)

type Record struct {
	ID       int
	Title    string // localized title or name
	Original string
	Date     string
	Overview string
	Runtime  int
	Genres   []string
	Vote     float64
	Votes    int
	Poster   string // "/abc.jpg"
	Backdrop string
	// Queries, when set, are the only (normalized) searches that find this
	// record; otherwise a search finds it when its query is part of a title.
	Queries []string
	Cast    []Person // sent as credits when the request appends them
	Crew    []string // directors (films) or creators (series)
}

type Person struct {
	Name, Character, Photo string
}

type Episode struct {
	Number   int
	Name     string
	Overview string
	Still    string
}

// Server answers /3/... like the TMDB API and /t/p/... like its image CDN.
// APIBase and ImageBase are what the app's config needs.
type Server struct {
	*httptest.Server
	APIBase, ImageBase string
	Key                string

	mu      sync.Mutex
	movies  []Record
	shows   []Record
	seasons map[string][]Episode
	images  map[string][]byte

	// Failure modes for API calls (not pictures).
	Status     int    // reply with this status instead
	RetryAfter string // with Status 429
	Body       string // reply 200 with this raw body
	Delay      time.Duration

	hits    map[string]int
	queries []string
	langs   []string
}

func New(key string) *Server {
	s := &Server{Key: key, seasons: map[string][]Episode{}, images: map[string][]byte{}, hits: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	s.APIBase, s.ImageBase = s.URL+"/3", s.URL+"/t/p"
	return s
}

func (s *Server) AddMovie(r Record) { s.mu.Lock(); s.movies = append(s.movies, r); s.mu.Unlock() }
func (s *Server) AddShow(r Record)  { s.mu.Lock(); s.shows = append(s.shows, r); s.mu.Unlock() }
func (s *Server) AddSeason(tvID, season int, eps ...Episode) {
	s.mu.Lock()
	s.seasons[fmt.Sprintf("%d/%d", tvID, season)] = eps
	s.mu.Unlock()
}
func (s *Server) AddImage(path string, data []byte) {
	s.mu.Lock()
	s.images[path] = data
	s.mu.Unlock()
}

// Set changes a failure mode safely while requests run.
func (s *Server) Set(f func(s *Server)) { s.mu.Lock(); f(s); s.mu.Unlock() }

// Hits counts requests by kind: search/movie, search/tv, movie, tv, season,
// configuration, image; "" sums them all.
func (s *Server) Hits(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if kind == "" {
		n := 0
		for _, v := range s.hits {
			n += v
		}
		return n
	}
	return s.hits[kind]
}

func (s *Server) Queries() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.queries...)
}

func (s *Server) Languages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.langs...)
}

func (s *Server) ResetHits() {
	s.mu.Lock()
	s.hits, s.queries, s.langs = map[string]int{}, nil, nil
	s.mu.Unlock()
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if rest, ok := strings.CutPrefix(r.URL.Path, "/t/p/"); ok {
		s.serveImage(w, rest)
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/3/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	kind := parts[0]
	switch {
	case kind == "search" && len(parts) == 2:
		kind = "search/" + parts[1]
	case kind == "tv" && len(parts) == 4 && parts[2] == "season":
		kind = "season"
	}
	q := r.URL.Query()
	s.mu.Lock()
	s.hits[kind]++
	s.langs = append(s.langs, q.Get("language"))
	if strings.HasPrefix(kind, "search/") {
		s.queries = append(s.queries, q.Get("query"))
	}
	status, retry, body, delay := s.Status, s.RetryAfter, s.Body, s.Delay
	s.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	key := q.Get("api_key")
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		key = bearer
	}
	if key != s.Key {
		// Echo the request like a careless proxy would: the client must not.
		reply(w, http.StatusUnauthorized, map[string]any{"status_code": 7, "status_message": "Invalid API key: " + r.URL.String()})
		return
	}
	if status != 0 {
		if retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		reply(w, status, map[string]any{"status_message": "failure for " + r.URL.String()})
		return
	}
	if body != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
		return
	}
	switch kind {
	case "configuration":
		reply(w, 200, map[string]any{"images": map[string]any{"secure_base_url": "https://image.tmdb.org/t/p/"}})
	case "search/movie":
		reply(w, 200, map[string]any{"page": 1, "results": s.search(s.movies, q.Get("query"), "movie")})
	case "search/tv":
		reply(w, 200, map[string]any{"page": 1, "results": s.search(s.shows, q.Get("query"), "tv")})
	case "movie", "tv":
		list := s.movies
		if kind == "tv" {
			list = s.shows
		}
		id, _ := strconv.Atoi(parts[1])
		for _, rec := range s.snapshot(list) {
			if rec.ID == id {
				reply(w, 200, details(rec, kind))
				return
			}
		}
		reply(w, 404, map[string]any{"status_code": 34})
	case "season":
		s.mu.Lock()
		eps, ok := s.seasons[parts[1]+"/"+parts[3]]
		s.mu.Unlock()
		if !ok {
			reply(w, 404, map[string]any{"status_code": 34})
			return
		}
		rows := []map[string]any{}
		for _, e := range eps {
			rows = append(rows, map[string]any{"episode_number": e.Number, "name": e.Name, "overview": e.Overview, "runtime": 42, "still_path": nullable(e.Still)})
		}
		reply(w, 200, map[string]any{"episodes": rows})
	default:
		reply(w, 404, map[string]any{"status_code": 34})
	}
}

func (s *Server) snapshot(list []Record) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Record(nil), list...)
}

func (s *Server) search(list []Record, query, kind string) []map[string]any {
	nq := tmdb.Normalize(query)
	out := []map[string]any{}
	for _, rec := range s.snapshot(list) {
		found := false
		if len(rec.Queries) > 0 {
			for _, q := range rec.Queries {
				found = found || tmdb.Normalize(q) == nq
			}
		} else {
			found = nq != "" && (strings.Contains(tmdb.Normalize(rec.Title), nq) || strings.Contains(tmdb.Normalize(rec.Original), nq))
		}
		if found {
			row := details(rec, kind)
			delete(row, "genres")
			delete(row, "runtime")
			delete(row, "episode_run_time")
			out = append(out, row)
		}
	}
	return out
}

func details(rec Record, kind string) map[string]any {
	genres := []map[string]any{}
	for i, g := range rec.Genres {
		genres = append(genres, map[string]any{"id": i + 1, "name": g})
	}
	row := map[string]any{"id": rec.ID, "overview": rec.Overview, "vote_average": rec.Vote, "vote_count": rec.Votes,
		"poster_path": nullable(rec.Poster), "backdrop_path": nullable(rec.Backdrop), "genres": genres}
	if kind == "tv" {
		row["name"], row["original_name"], row["first_air_date"] = rec.Title, rec.Original, rec.Date
		row["episode_run_time"] = []int{rec.Runtime}
	} else {
		row["title"], row["original_title"], row["release_date"] = rec.Title, rec.Original, rec.Date
		row["runtime"] = rec.Runtime
	}
	cast, crew, creators := []map[string]any{}, []map[string]any{}, []map[string]any{}
	for _, p := range rec.Cast {
		cast = append(cast, map[string]any{"name": p.Name, "character": p.Character, "profile_path": nullable(p.Photo)})
	}
	for _, name := range rec.Crew {
		if kind == "tv" {
			creators = append(creators, map[string]any{"name": name})
		} else {
			crew = append(crew, map[string]any{"name": name, "job": "Director"})
		}
	}
	crew = append(crew, map[string]any{"name": "Someone Else", "job": "Producer"})
	row["credits"] = map[string]any{"cast": cast, "crew": crew}
	if kind == "tv" {
		row["created_by"] = creators
	}
	return row
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Server) serveImage(w http.ResponseWriter, rest string) {
	_, file, ok := strings.Cut(rest, "/")
	s.mu.Lock()
	s.hits["image"]++
	data, found := s.images["/"+file]
	s.mu.Unlock()
	if !ok || !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Write(data)
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
