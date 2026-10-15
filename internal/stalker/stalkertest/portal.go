// Package stalkertest is a fake Stalker/Ministra portal: enough of the
// protocol, and of real portals' quirks, to test Dreaminator's client and to
// drive the virtual Dreambox without a provider account.
package stalkertest

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Channel struct {
	ID       string
	Name     string
	Genre    string
	Stream   string // real address handed out by create_link
	Static   bool   // cmd is the stream itself (use_http_tmp_link=0)
	Censored bool
	NoGuide  bool // get_short_epg answers an empty list
}

type Portal struct {
	// Path the API answers on, e.g. "/stalker_portal/server/load.php".
	Path     string
	Channels []Channel
	// Login/Password, when set, make get_profile answer status 2 until
	// do_auth succeeds.
	Login, Password string
	// TokenUses invalidates a token after that many authorised calls
	// (0 = never), to exercise re-authentication.
	TokenUses int
	// NoAllChannels makes get_all_channels return nothing, forcing the
	// paged get_ordered_list crawl.
	NoAllChannels bool
	PageSize      int
	// Catalog enables movie and three series-mode fixtures.
	Catalog bool
	// CatalogStream optionally serves every catalog selection from a sample URL.
	CatalogStream string
	// DemoMovies adds a second VOD category with that many films with posters
	// and full metadata.
	DemoMovies int
	// IgnoreSearch makes get_ordered_list ignore "search", like some portals.
	IgnoreSearch bool

	mu          sync.Mutex
	tokens      map[string]int // token -> calls used
	profiled    map[string]bool
	authed      bool
	Handshake   int
	Profiles    int
	Links       int
	Watchdogs   int
	ShortGuides int
	seq         int
}

func (p *Portal) init() {
	if p.tokens == nil {
		p.tokens = map[string]int{}
		p.profiled = map[string]bool{}
	}
	if p.PageSize == 0 {
		p.PageSize = 14
	}
}

func reply(w http.ResponseWriter, js any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"js": js})
}

func mac(r *http.Request) string {
	for _, part := range strings.Split(r.Header.Get("Cookie"), ";") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && k == "mac" {
			return v
		}
	}
	return ""
}

func (p *Portal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	if strings.Contains(r.URL.Path, "/misc/logos/") {
		serveLogo(w, strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".png"))
		return
	}
	if strings.Contains(r.URL.Path, "/misc/posters/") {
		servePoster(w, strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".jpg"))
		return
	}
	if r.URL.Path != p.Path {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	action := q.Get("type") + "." + q.Get("action")
	if r.Header.Get("User-Agent") == "" || !strings.Contains(r.Header.Get("User-Agent"), "MAG") {
		http.Error(w, "Access denied.", http.StatusOK)
		return
	}
	m := mac(r)
	if m == "" {
		fmt.Fprint(w, "Unauthorized request.")
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

	switch action {
	case "stb.handshake":
		sum := sha1.Sum([]byte(strings.ToUpper(m)))
		if q.Get("prehash") != strings.ToUpper(hex.EncodeToString(sum[:])) {
			reply(w, map[string]any{"error": "bad prehash"})
			return
		}
		p.Handshake++
		t := q.Get("token")
		if _, ok := p.tokens[t]; !ok || t == "" {
			p.seq++
			t = fmt.Sprintf("T%04d%s", p.seq, strings.ReplaceAll(m, ":", ""))
			p.tokens[t] = 0
		}
		reply(w, map[string]any{"token": t, "random": "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", "not_valid": 0})
		return
	case "stb.get_profile":
		if _, ok := p.tokens[token]; !ok {
			fmt.Fprint(w, "Authorization failed.")
			return
		}
		p.Profiles++
		if !strings.HasPrefix(m, "00:1A:79:") {
			reply(w, map[string]any{"status": 1, "block_msg": "MAC-адрес не зарегистрирован"})
			return
		}
		if p.Login != "" && !p.authed {
			if q.Get("auth_second_step") != "1" {
				reply(w, map[string]any{"status": 2})
				return
			}
		}
		p.profiled[token] = true
		reply(w, map[string]any{"status": "0", "timezone": "Europe/Moscow", "watchdog_timeout": "60",
			"account_info": map[string]any{"login": p.Login, "expire_date": time.Now().AddDate(0, 6, 0).Unix()}})
		return
	case "stb.do_auth":
		p.authed = q.Get("login") == p.Login && q.Get("password") == p.Password
		reply(w, p.authed)
		return
	}

	// Everything else needs an adopted, unexpired token.
	uses, ok := p.tokens[token]
	if !ok || !p.profiled[token] || (p.TokenUses > 0 && uses >= p.TokenUses) {
		delete(p.tokens, token)
		fmt.Fprint(w, "Authorization failed.")
		return
	}
	p.tokens[token] = uses + 1
	if p.Catalog && p.serveCatalog(w, r) {
		return
	}

	switch action {
	case "itv.get_genres":
		genres := []map[string]any{{"id": "*", "title": "Все"}}
		seen := map[string]bool{}
		for _, c := range p.Channels {
			if !seen[c.Genre] {
				seen[c.Genre] = true
				g := map[string]any{"id": genreID(c.Genre), "title": c.Genre, "censored": 0}
				if c.Censored {
					g["censored"] = "1"
				}
				genres = append(genres, g)
			}
		}
		reply(w, genres)
	case "itv.get_all_channels":
		var rows []map[string]any
		if !p.NoAllChannels {
			for i, c := range p.Channels {
				if !c.Censored {
					rows = append(rows, p.row(i, c))
				}
			}
		}
		reply(w, map[string]any{"total_items": len(rows), "max_page_items": len(rows), "data": rows})
	case "itv.get_ordered_list":
		var rows []map[string]any
		for i, c := range p.Channels {
			if g := q.Get("genre"); g == "*" && !c.Censored || g == genreID(c.Genre) {
				rows = append(rows, p.row(i, c))
			}
		}
		n, _ := strconv.Atoi(q.Get("p"))
		if n < 1 {
			n = 1
		}
		from, to := (n-1)*p.PageSize, n*p.PageSize
		var pageRows []map[string]any
		if from < len(rows) {
			pageRows = rows[from:min(to, len(rows))]
		}
		reply(w, map[string]any{"total_items": strconv.Itoa(len(rows)), "max_page_items": strconv.Itoa(p.PageSize), "data": pageRows})
	case "itv.create_link":
		p.Links++
		cmd := q.Get("cmd")
		for i, c := range p.Channels {
			if cmd == cmdFor(i, c) {
				sep := "?"
				if strings.Contains(c.Stream, "?") {
					sep = "&"
				}
				reply(w, map[string]any{"id": c.ID, "cmd": "ffmpeg " + c.Stream + sep + "play_token=tmp" + strconv.Itoa(p.Links)})
				return
			}
		}
		reply(w, map[string]any{"cmd": "", "error": "nothing_to_play"})
	case "itv.get_epg_info":
		hours, _ := strconv.Atoi(q.Get("period"))
		now := time.Now().Truncate(time.Hour)
		data := map[string][]map[string]any{}
		for _, c := range p.Channels {
			for h := -1; h < hours; h++ {
				start := now.Add(time.Duration(h) * time.Hour)
				data[c.ID] = append(data[c.ID], map[string]any{
					"id": fmt.Sprintf("%s-%d", c.ID, h), "ch_id": c.ID,
					"name":            fmt.Sprintf("%s: передача в %s", c.Name, start.Format("15:04")),
					"descr":           "Описание передачи",
					"start_timestamp": strconv.FormatInt(start.Unix(), 10),
					"stop_timestamp":  strconv.FormatInt(start.Add(time.Hour).Unix(), 10),
				})
			}
		}
		reply(w, map[string]any{"data": data})
	case "itv.get_short_epg":
		// What is on now and next, as a plain list (the portal's own times).
		p.ShortGuides++
		size, _ := strconv.Atoi(q.Get("size"))
		now := time.Now().Truncate(time.Hour)
		rows := []map[string]any{}
		for _, c := range p.Channels {
			if c.ID != q.Get("ch_id") || c.NoGuide {
				continue
			}
			for h := 0; h < size; h++ {
				start := now.Add(time.Duration(h) * time.Hour)
				rows = append(rows, map[string]any{
					"id": fmt.Sprintf("%s-%d", c.ID, h), "ch_id": c.ID,
					"name":            fmt.Sprintf("%s: передача в %s", c.Name, start.Format("15:04")),
					"descr":           fmt.Sprintf("Описание передачи в %s", start.Format("15:04")),
					"start_timestamp": strconv.FormatInt(start.Unix(), 10),
					"stop_timestamp":  strconv.FormatInt(start.Add(time.Hour).Unix(), 10),
				})
			}
		}
		reply(w, rows)
	case "account_info.get_main_info":
		reply(w, map[string]any{"mac": m, "phone": "", "fname": "Демо",
			"end_date": time.Now().AddDate(0, 2, 0).Format("2006-01-02 15:04:05"), "tariff_plan": "Базовый", "status": "1"})
	case "watchdog.get_events":
		p.Watchdogs++
		reply(w, map[string]any{"data": map[string]any{"msgs": 0}})
	default:
		reply(w, map[string]any{"error": "Unknown action"})
	}
}

// serveLogo draws a simple coloured badge per channel, so demo portals
// have logos to turn into picons.
func serveLogo(w http.ResponseWriter, id string) {
	sum := sha1.Sum([]byte(id))
	c := color.NRGBA{sum[0] | 0x40, sum[1] | 0x40, sum[2] | 0x40, 255}
	img := image.NewNRGBA(image.Rect(0, 0, 200, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 200; x++ {
			dx, dy := min(x, 199-x), min(y, 119-y)
			if dx < 20 && dy < 20 && (20-dx)*(20-dx)+(20-dy)*(20-dy) > 400 {
				continue // rounded corners
			}
			if x > 70 && x < 130 && y > 30 && y < 90 && (x-70)*2 > (y-30) && (x-70)*2 > (90-y) == false {
				img.Set(x, y, color.NRGBA{255, 255, 255, 255})
				continue
			}
			img.Set(x, y, c)
		}
	}
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, img)
}

func genreID(title string) string {
	sum := sha1.Sum([]byte(title))
	return strconv.Itoa(int(sum[0])<<8 | int(sum[1]))
}

func cmdFor(i int, c Channel) string {
	if c.Static {
		return "ffmpeg " + c.Stream
	}
	return fmt.Sprintf("ffrt3 http://localhost/ch/%d_", 1000+i)
}

func (p *Portal) row(i int, c Channel) map[string]any {
	tmp := 1
	if c.Static {
		tmp = 0
	}
	return map[string]any{
		"id": c.ID, "name": c.Name, "number": strconv.Itoa(i + 1), "cmd": cmdFor(i, c),
		"tv_genre_id": genreID(c.Genre), "logo": "/stalker_portal/misc/logos/" + c.ID + ".png",
		"use_http_tmp_link": strconv.Itoa(tmp), "use_load_balancing": 0,
	}
}

// Stats is a snapshot of the counters, for tests.
func (p *Portal) Stats() (handshakes, profiles, links, watchdogs int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Handshake, p.Profiles, p.Links, p.Watchdogs
}
