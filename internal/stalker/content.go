package stalker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the receiver may lack zoneinfo for the portal's timezone
)

type Genre struct {
	ID       string
	Title    string
	Censored bool
}

type Channel struct {
	ID        string
	Name      string
	Number    int
	Cmd       string
	Logo      string
	GenreID   string
	XMLTVID   string
	NeedsLink bool // the cmd must go through create_link before playback
}

func (c *Client) Genres(ctx context.Context) ([]Genre, error) {
	payload, err := c.Call(ctx, p("type", "itv"), p("action", "get_genres"))
	if err != nil {
		return nil, err
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(payload, &rows) != nil {
		var wrapped struct {
			Data []map[string]json.RawMessage `json:"data"`
		}
		json.Unmarshal(payload, &wrapped)
		rows = wrapped.Data
	}
	var out []Genre
	for _, r := range rows {
		id := field(r, "id", "category_id")
		if id == "" || id == "*" {
			continue
		}
		out = append(out, Genre{ID: id, Title: field(r, "title", "name", "category_name"), Censored: flex(field(r, "censored")).truthy()})
	}
	return out, nil
}

func field(r map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if raw, ok := r[k]; ok {
			var f flex
			if json.Unmarshal(raw, &f) == nil && f != "" {
				return strings.TrimSpace(string(f))
			}
		}
	}
	return ""
}

type page struct {
	Rows       []map[string]json.RawMessage
	Total      int
	MaxPerPage int
}

func decodePage(payload json.RawMessage) page {
	var pg struct {
		Data       []map[string]json.RawMessage `json:"data"`
		Total      flex                         `json:"total_items"`
		MaxPerPage flex                         `json:"max_page_items"`
	}
	if json.Unmarshal(payload, &pg) != nil {
		var arr []map[string]json.RawMessage
		json.Unmarshal(payload, &arr)
		return page{Rows: arr}
	}
	return page{Rows: pg.Data, Total: pg.Total.int(), MaxPerPage: pg.MaxPerPage.int()}
}

// Channels lists every live channel: get_all_channels first, a page crawl of
// get_ordered_list when the portal does not support it, and a per-genre pass
// for censored genres that get_all_channels usually leaves out.
func (c *Client) Channels(ctx context.Context, genres []Genre) ([]Channel, error) {
	seen := map[string]bool{}
	var rows []map[string]json.RawMessage
	keep := func(list []map[string]json.RawMessage) int {
		added := 0
		for _, r := range list {
			id := field(r, "id", "stream_id")
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			rows = append(rows, r)
			added++
		}
		return added
	}

	payload, err := c.Call(ctx, p("type", "itv"), p("action", "get_all_channels"))
	if err == nil {
		keep(decodePage(payload).Rows)
	}
	if len(rows) == 0 {
		if err := c.crawl(ctx, "*", keep); err != nil {
			return nil, err
		}
	}
	present := map[string]bool{}
	for _, r := range rows {
		present[field(r, "tv_genre_id")] = true
	}
	for _, g := range genres {
		if g.Censored && !present[g.ID] {
			c.crawl(ctx, g.ID, keep)
		}
	}

	out := make([]Channel, 0, len(rows))
	for _, r := range rows {
		ch := Channel{
			ID:      field(r, "id", "stream_id"),
			Name:    field(r, "name"),
			Number:  flex(field(r, "number")).int(),
			Cmd:     field(r, "cmd"),
			Logo:    c.absolute(field(r, "logo")),
			GenreID: field(r, "tv_genre_id"),
			XMLTVID: field(r, "xmltv_id"),
		}
		_, hasTmp := r["use_http_tmp_link"]
		_, hasLB := r["use_load_balancing"]
		ch.NeedsLink = NeedsLink(ch.Cmd, flex(field(r, "use_http_tmp_link")).truthy(), flex(field(r, "use_load_balancing")).truthy(), hasTmp || hasLB)
		if ch.Name == "" || ch.Cmd == "" {
			continue
		}
		out = append(out, ch)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Number != out[j].Number && out[i].Number > 0 && out[j].Number > 0 {
			return out[i].Number < out[j].Number
		}
		return false
	})
	return out, nil
}

const maxChannels = 30000

func (c *Client) crawl(ctx context.Context, genre string, keep func([]map[string]json.RawMessage) int) error {
	for n := 1; n <= 3000; n++ {
		payload, err := c.Call(ctx, p("type", "itv"), p("action", "get_ordered_list"),
			p("genre", genre), p("category", genre), p("sortby", "number"), p("p", strconv.Itoa(n)))
		if err != nil {
			if n == 1 {
				return err
			}
			// one retry per page, then stop with what we have
			if payload, err = c.Call(ctx, p("type", "itv"), p("action", "get_ordered_list"),
				p("genre", genre), p("category", genre), p("sortby", "number"), p("p", strconv.Itoa(n))); err != nil {
				return nil
			}
		}
		pg := decodePage(payload)
		// Some portals ignore p and repeat the first page forever.
		if len(pg.Rows) == 0 || keep(pg.Rows) == 0 {
			return nil
		}
		per := pg.MaxPerPage
		if per <= 0 {
			per = len(pg.Rows)
		}
		if pg.Total > 0 && n*per >= pg.Total || n*per >= maxChannels {
			return nil
		}
	}
	return nil
}

func (c *Client) absolute(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	base, err := url.Parse(c.Session().Endpoint)
	if err != nil {
		return ref
	}
	u, err := base.Parse(ref)
	if err != nil {
		return ref
	}
	return u.String()
}

// ---------------------------------------------------------------- stream links

var solutionPrefix = regexp.MustCompile(`^\S+\s+(.+)$`)

// stripSolution removes the player hint ("ffmpeg ", "ffrt3 ", "auto ") when
// what follows looks like an address.
func stripSolution(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if m := solutionPrefix.FindStringSubmatch(cmd); m != nil {
		rest := strings.TrimSpace(m[1])
		if strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://") || strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "?") {
			return rest
		}
	}
	return cmd
}

func isLocalHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "localhost.localdomain" || h == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// NeedsLink decides whether a channel's cmd is a real address or a ticket
// that create_link must turn into one (IPTVnator's rule).
func NeedsLink(cmd string, tmpLink, loadBalancing, hasFlags bool) bool {
	if tmpLink || loadBalancing || !hasFlags {
		return true
	}
	u, err := url.Parse(stripSolution(cmd))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return true
	}
	return isLocalHost(u.Hostname())
}

// streamURL turns what the portal returned into an absolute address.
func (c *Client) streamURL(cmd, original string) string {
	s := stripSolution(cmd)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	endpoint, err := url.Parse(c.Session().Endpoint)
	if err != nil {
		return s
	}
	origin := endpoint.Scheme + "://" + endpoint.Host
	basePath := regexp.MustCompile(`(?i)(/portal\.php|/server/load\.php|/[^/]*\.php)$`).ReplaceAllString(endpoint.Path, "")
	switch {
	case strings.HasPrefix(s, "/"):
		return origin + basePath + s
	case strings.HasPrefix(s, "?"):
		orig := stripSolution(original)
		if i := strings.IndexAny(orig, "?#"); i >= 0 {
			orig = orig[:i]
		}
		if !strings.HasPrefix(orig, "http://") && !strings.HasPrefix(orig, "https://") {
			orig = origin + basePath + orig
		}
		return orig + s
	}
	return s
}

// Resolve returns a playable address for a channel, asking the portal for a
// fresh temporary link when the channel needs one.
func (c *Client) Resolve(ctx context.Context, ch Channel) (string, error) {
	if !ch.NeedsLink {
		if u := c.streamURL(ch.Cmd, ch.Cmd); strings.HasPrefix(u, "http") {
			return u, nil
		}
	}
	c.kickWatchdog(ctx)
	payload, err := c.Call(ctx, p("type", "itv"), p("action", "create_link"), p("cmd", ch.Cmd),
		p("disable_ad", "0"), p("download", "0"))
	if err != nil {
		return "", err
	}
	var link struct {
		Cmd   string `json:"cmd"`
		Error any    `json:"error"`
	}
	if json.Unmarshal(payload, &link) != nil {
		return "", fmt.Errorf("портал вернул непонятную ссылку")
	}
	if link.Error != nil && fmt.Sprint(link.Error) != "" && fmt.Sprint(link.Error) != "false" {
		return "", fmt.Errorf("портал не выдал ссылку: %v", link.Error)
	}
	if strings.TrimSpace(link.Cmd) == "" {
		return "", fmt.Errorf("портал не выдал ссылку на канал")
	}
	return c.streamURL(link.Cmd, ch.Cmd), nil
}

// kickWatchdog tells the portal the "set-top box" is alive. Some Ministra
// installs refuse streams until the session has been activated this way, and
// it keeps pinging while channels are being watched.
func (c *Client) kickWatchdog(ctx context.Context) {
	c.wdMu.Lock()
	c.lastResolve = time.Now()
	first := !c.watchdogLive
	c.watchdogLive = true
	c.wdMu.Unlock()
	if !first {
		return
	}
	ping := func(init string) {
		pctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c.Call(pctx, p("type", "watchdog"), p("action", "get_events"), p("event_active_id", "0"),
			p("cur_play_type", "1"), p("init", init))
	}
	ping("1")
	go func() {
		for {
			c.mu.Lock()
			every := c.watchdog
			c.mu.Unlock()
			time.Sleep(min(every, 2*time.Minute))
			c.wdMu.Lock()
			idle := time.Since(c.lastResolve) > 6*time.Hour
			if idle {
				c.watchdogLive = false
			}
			c.wdMu.Unlock()
			if idle {
				return
			}
			ping("0")
		}
	}()
}

// ---------------------------------------------------------------- guide

type Programme struct {
	Start, Stop time.Time
	Title, Desc string
}

func parseProgramme(r map[string]json.RawMessage, loc *time.Location) (Programme, bool) {
	pr := Programme{Title: field(r, "name"), Desc: field(r, "descr")}
	if s, e := flex(field(r, "start_timestamp")).int(), flex(field(r, "stop_timestamp")).int(); s > 1e9 && e > s {
		pr.Start, pr.Stop = time.Unix(int64(s), 0), time.Unix(int64(e), 0)
	} else {
		var err1, err2 error
		pr.Start, err1 = time.ParseInLocation("2006-01-02 15:04:05", field(r, "time"), loc)
		pr.Stop, err2 = time.ParseInLocation("2006-01-02 15:04:05", field(r, "time_to"), loc)
		if err1 != nil || err2 != nil {
			return pr, false
		}
	}
	return pr, pr.Title != "" && pr.Stop.After(pr.Start)
}

// Guide fetches the portal's programme guide for the next hours, keyed by
// channel id. Portals answer either a map of channel id → programmes or a
// flat list with ch_id on each entry.
func (c *Client) Guide(ctx context.Context, hours int) (map[string][]Programme, error) {
	payload, err := c.Call(ctx, p("type", "itv"), p("action", "get_epg_info"), p("period", strconv.Itoa(hours)))
	if err != nil {
		return nil, err
	}
	loc, _ := time.LoadLocation(timezone)
	out := map[string][]Programme{}
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	data := payload
	if json.Unmarshal(payload, &wrapped) == nil && wrapped.Data != nil {
		data = wrapped.Data
	}
	var byChannel map[string][]map[string]json.RawMessage
	if json.Unmarshal(data, &byChannel) == nil {
		for id, rows := range byChannel {
			for _, r := range rows {
				if pr, ok := parseProgramme(r, loc); ok {
					out[id] = append(out[id], pr)
				}
			}
		}
		return out, nil
	}
	var flat []map[string]json.RawMessage
	if json.Unmarshal(data, &flat) == nil {
		for _, r := range flat {
			if pr, ok := parseProgramme(r, loc); ok {
				id := field(r, "ch_id")
				out[id] = append(out[id], pr)
			}
		}
	}
	return out, nil
}

// ShortGuide is the per-channel fallback for portals whose bulk guide is
// empty for a channel.
func (c *Client) ShortGuide(ctx context.Context, channelID string, size int) ([]Programme, error) {
	payload, err := c.Call(ctx, p("type", "itv"), p("action", "get_short_epg"), p("ch_id", channelID), p("size", strconv.Itoa(size)))
	if err != nil {
		return nil, err
	}
	loc, _ := time.LoadLocation(timezone)
	var rows []map[string]json.RawMessage
	if json.Unmarshal(payload, &rows) != nil {
		var wrapped struct {
			Data []map[string]json.RawMessage `json:"data"`
		}
		json.Unmarshal(payload, &wrapped)
		rows = wrapped.Data
	}
	var out []Programme
	for _, r := range rows {
		if pr, ok := parseProgramme(r, loc); ok {
			out = append(out, pr)
		}
	}
	return out, nil
}
