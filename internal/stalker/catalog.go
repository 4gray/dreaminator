package stalker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"
)

type CatalogCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CatalogItem struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description,omitempty"`
	Poster       string   `json:"-"`
	HasPoster    bool     `json:"hasPoster,omitempty"`
	Year         string   `json:"year,omitempty"`
	Duration     string   `json:"duration,omitempty"`
	OriginalName string   `json:"originalName,omitempty"`
	Genres       string   `json:"genres,omitempty"`
	Ratings      []Rating `json:"ratings,omitempty"`
	Age          string   `json:"age,omitempty"`
	Director     string   `json:"director,omitempty"`
	Actors       string   `json:"actors,omitempty"`
	Country      string   `json:"country,omitempty"`
	Number       string   `json:"number,omitempty"` // season or episode number shown to the viewer
	TMDBID       string   `json:"-"`                // explicit TMDB id when the portal carries one
	Type         string   `json:"-"`
	ID           string   `json:"-"`
	MovieID      string   `json:"-"`
	SeasonID     string   `json:"-"`
	Episode      string   `json:"-"`
	Cmd          string   `json:"-"`
	NeedsLink    bool     `json:"-"`
	CatalogID    string   `json:"-"` // persistent identity, set by the service (see app.catalogID)
	embedded     []string
	lazy         bool
}

// Rating keeps its source: a provider's IMDb copy is not a TMDB score.
type Rating struct {
	Source string `json:"source"` // imdb | kinopoisk | tmdb
	Value  string `json:"value"`
	Origin string `json:"origin"` // provider | tmdb
}

type CatalogQuery struct {
	Type, Category string
	Page           int
	Parent         *CatalogItem
	Search         string
	Sort           string // added | top | rating | name; root lists only
}

// CatalogSorts are the orders Ministra's get_ordered_list understands.
// "added" lists new arrivals first; omitting sortby is not "newest first" on
// every portal (some fall back to names).
var CatalogSorts = map[string]bool{"added": true, "top": true, "rating": true, "name": true}

type CatalogPage struct {
	Items    []CatalogItem `json:"items"`
	Page     int           `json:"page"`
	HasMore  bool          `json:"hasMore"`
	PageSize int           `json:"pageSize,omitempty"`
	Total    int           `json:"total,omitempty"` // 0 when the portal does not say
}

// ErrCatalogUnsupported identifies an absent provider catalog section.
var ErrCatalogUnsupported = errors.New("этот раздел не поддерживается порталом")

// ErrCatalogNetwork identifies a transport failure without exposing its URL.
var ErrCatalogNetwork = errors.New("нет связи с порталом")

const maxCatalogResponseBytes = 8 << 20
const catalogEpisodePageSize = 50

var errCatalogResponseTooLarge = errors.New("ответ каталога слишком большой")

var errCatalog = errors.New("портал не вернул каталог")
var errCatalogLink = errors.New("портал не выдал ссылку для воспроизведения")

func catalogError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, ErrAuth) {
		return ErrAuth
	}
	var networkErr net.Error
	if errors.Is(err, ErrCatalogNetwork) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) || errors.As(err, &networkErr) {
		return ErrCatalogNetwork
	}
	return errCatalog
}

// catalogRows accepts raw arrays and the paginated data wrapper. Error bodies
// never become empty successful catalogs, and never leave this boundary.
func catalogRows(payload json.RawMessage) (page, error) {
	if len(payload) > maxCatalogResponseBytes {
		return page{}, errCatalog
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(payload, &rows) == nil && strings.TrimSpace(string(payload)) != "null" {
		if err := validateCatalogRows(rows); err != nil {
			return page{}, err
		}
		return page{Rows: rows}, nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(payload, &obj) != nil || obj == nil {
		return page{}, errCatalog
	}
	if e := field(obj, "error"); e != "" && e != "0" && e != "false" {
		if strings.Contains(strings.ToLower(e), "unknown action") || strings.Contains(strings.ToLower(e), "not supported") {
			return page{}, ErrCatalogUnsupported
		}
		return page{}, errCatalog
	}
	data, ok := obj["data"]
	if !ok {
		return page{}, errCatalog
	}
	if string(data) != "null" && json.Unmarshal(data, &rows) != nil {
		return page{}, errCatalog
	}
	if err := validateCatalogRows(rows); err != nil {
		return page{}, err
	}
	return page{Rows: rows, Total: flex(field(obj, "total_items")).int(), MaxPerPage: flex(field(obj, "max_page_items")).int()}, nil
}

// Limits keep one cached page and any generated embedded episodes bounded.
func validateCatalogRows(rows []map[string]json.RawMessage) error {
	if len(rows) > 5000 {
		return errCatalog
	}
	for _, row := range rows {
		for _, value := range row {
			if len(value) > 64<<10 {
				return errCatalog
			}
		}
		if raw, ok := row["series"]; ok && len(raw) > 0 && raw[0] == '[' {
			var episodes []json.RawMessage
			if json.Unmarshal(raw, &episodes) != nil || len(episodes) > 5000 {
				return errCatalog
			}
		}
	}
	return nil
}

func (c *Client) CatalogCategories(ctx context.Context, section string) ([]CatalogCategory, error) {
	if section != "vod" && section != "series" {
		return nil, ErrCatalogUnsupported
	}
	payload, err := c.Call(ctx, p("type", section), p("action", "get_categories"))
	if err != nil {
		return nil, catalogError(err)
	}
	pg, err := catalogRows(payload)
	if err != nil {
		return nil, err
	}
	out := make([]CatalogCategory, 0, len(pg.Rows))
	for _, r := range pg.Rows {
		id := field(r, "id", "category_id")
		if id == "" {
			continue
		}
		out = append(out, CatalogCategory{ID: id, Name: field(r, "title", "name", "category_name")})
	}
	return out, nil
}

func (c *Client) CatalogPage(ctx context.Context, q CatalogQuery) (CatalogPage, error) {
	n := max(1, q.Page)
	out := CatalogPage{Items: []CatalogItem{}, Page: n}
	if q.Type != "vod" && q.Type != "series" {
		return out, ErrCatalogUnsupported
	}
	if n > 100000 {
		return out, errCatalog
	}
	parent := q.Parent
	if parent != nil {
		if parent.Type != q.Type || (parent.Kind != "series" && parent.Kind != "season") {
			return out, errCatalog
		}
		// Embedded VOD has one synthetic season, retaining its original command.
		if parent.Kind == "series" && parent.Type == "vod" && !parent.lazy && len(parent.embedded) > 0 {
			if n > 1 {
				return out, nil
			}
			season := *parent
			season.Key = ""
			season.Kind = "season"
			season.Name = "Эпизоды"
			season.Number = ""
			season.MovieID = parent.ID
			season.SeasonID = parent.ID
			out.Items = append(out.Items, season)
			out.PageSize, out.Total = 1, 1
			return out, nil
		}
		// Regular-series roots fetch seasons even when they carry series[].
		if len(parent.embedded) > 0 && parent.Kind == "season" {
			first := (n - 1) * catalogEpisodePageSize
			if first >= len(parent.embedded) {
				return out, nil
			}
			last := min(first+catalogEpisodePageSize, len(parent.embedded))
			out.HasMore = last < len(parent.embedded)
			out.PageSize, out.Total = catalogEpisodePageSize, len(parent.embedded)
			for _, episode := range parent.embedded[first:last] {
				item := *parent
				item.Key = ""
				item.Kind = "episode"
				item.Episode = episode
				item.Name = "Эпизод " + episode
				item.Number = episode
				item.Description = ""
				item.embedded = nil
				item.NeedsLink = true
				if item.MovieID == "" {
					item.MovieID = parent.ID
				}
				if parent.Kind == "season" {
					item.SeasonID = parent.ID
				}
				item.ID = parent.ID + ":" + episode
				out.Items = append(out.Items, item)
			}
			return out, nil
		}
	}
	// Seasons and episodes keep the portal's natural order; the viewer's
	// choice applies to the lists of films and series.
	sortby := "name"
	if parent == nil {
		sortby = "added"
		if CatalogSorts[q.Sort] {
			sortby = q.Sort
		}
	}
	params := []param{p("type", q.Type), p("action", "get_ordered_list"), p("p", strconv.Itoa(n)), p("sortby", sortby)}
	if parent == nil {
		category := q.Category
		if category == "" || q.Search != "" {
			category = "*"
		}
		params = append(params, p("category", category))
		if q.Search != "" {
			params = append(params, p("search", q.Search))
		}
	} else {
		movie := parent.ID
		if parent.Kind == "season" {
			movie = parent.MovieID
		}
		if q.Type == "series" {
			movie, _, _ = strings.Cut(movie, ":")
		}
		params = append(params, p("movie_id", movie))
		if parent.Kind == "season" {
			params = append(params, p("season_id", parent.ID))
		}
	}
	payload, err := c.Call(ctx, params...)
	if err != nil {
		return out, catalogError(err)
	}
	pg, err := catalogRows(payload)
	if err != nil {
		return out, err
	}
	for _, row := range pg.Rows {
		if parent != nil && parent.lazy {
			flag := "is_season"
			if parent.Kind == "season" {
				flag = "is_episode"
			}
			if !flex(field(row, flag)).truthy() {
				continue
			}
		}
		item := c.catalogItem(row, q.Type, parent)
		if item.ID != "" && item.Name != "" {
			out.Items = append(out.Items, item)
		}
	}
	if len(pg.Rows) > 0 {
		per := pg.MaxPerPage
		if per <= 0 {
			per = len(pg.Rows)
		}
		out.HasMore = pg.Total > 0 && n <= (pg.Total-1)/per || pg.Total <= 0 && pg.MaxPerPage > 0 && len(pg.Rows) >= pg.MaxPerPage
		out.PageSize = per
		// Lazy seasons drop rows that are not seasons, so only root totals are exact.
		if parent == nil && pg.Total > 0 {
			out.Total = pg.Total
		}
	}
	return out, nil
}

func (c *Client) catalogItem(row map[string]json.RawMessage, section string, parent *CatalogItem) CatalogItem {
	item := CatalogItem{ID: field(row, "id", "stream_id"), Name: field(row, "name", "title", "o_name"), Type: section, Kind: "movie", Description: field(row, "description", "descr", "plot"), Year: field(row, "year"), Duration: field(row, "duration", "time"), Cmd: field(row, "cmd"), NeedsLink: true}
	item.Poster = c.absolute(field(row, "screenshot_uri", "cover", "movie_image", "poster", "logo"))
	catalogMetadata(&item, row)
	var series []flex
	if json.Unmarshal(row["series"], &series) == nil {
		for _, v := range series {
			if v.int() > 0 {
				item.embedded = append(item.embedded, string(v))
			}
		}
	}
	if parent == nil {
		if section == "series" || len(item.embedded) > 0 || flex(field(row, "is_series")).truthy() {
			item.Kind = "series"
		}
		item.lazy = section == "vod" && flex(field(row, "is_series")).truthy()
	} else {
		item.MovieID = parent.MovieID
		if item.MovieID == "" {
			item.MovieID = parent.ID
		}
		item.lazy = parent.lazy
		if item.Poster == "" {
			item.Poster = parent.Poster
		}
		if parent.Kind == "series" {
			item.Kind = "season"
			item.SeasonID = item.ID
			item.Number = field(row, "season_number")
			if parent.lazy {
				if videoID := field(row, "video_id"); videoID != "" {
					item.MovieID = videoID
				}
			}
		} else {
			item.Kind = "episode"
			item.SeasonID = parent.ID
			item.Episode = field(row, "series_number", "episode_num", "episode_number")
			item.Number = item.Episode
			if item.Cmd == "" {
				item.Cmd = "/media/file_" + item.ID + ".mpg"
			}
		}
	}
	return item
}

// ResolveCatalog mints a new link for every click. The episode selector always
// reaches create_link, including when its parent has a static-looking command.
func (c *Client) ResolveCatalog(ctx context.Context, item CatalogItem) (string, error) {
	if (item.Kind != "movie" && item.Kind != "episode") || (item.Type != "vod" && item.Type != "series") || item.Cmd == "" {
		return "", errCatalogLink
	}
	section := item.Type
	if item.Episode != "" {
		section = "vod"
	}
	// buildQuery deliberately leaves percent escapes intact for MAG commands.
	// Escape once here so literal plus signs and existing escapes survive parsing.
	params := []param{p("type", section), p("action", "create_link"), p("cmd", url.QueryEscape(item.Cmd)), p("disable_ad", "0"), p("download", "0")}
	if item.Episode != "" {
		params = append(params, p("series", item.Episode))
	}
	payload, err := c.Call(ctx, params...)
	if err != nil {
		return "", catalogError(err)
	}
	var row map[string]json.RawMessage
	if json.Unmarshal(payload, &row) != nil {
		return "", errCatalogLink
	}
	if e := field(row, "error"); e != "" && e != "0" && e != "false" {
		return "", errCatalogLink
	}
	cmd := field(row, "cmd")
	if cmd == "" {
		return "", errCatalogLink
	}
	stream := c.streamURL(cmd, item.Cmd)
	// Scheme-relative commands must resolve to their actual host, not become a
	// double-slash path on the portal.
	if strings.HasPrefix(stripSolution(cmd), "//") {
		base, _ := url.Parse(c.Session().Endpoint)
		stream = base.Scheme + ":" + stripSolution(cmd)
	}
	u, err := url.Parse(stream)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(stream, "\r\n\t ") {
		return "", errCatalogLink
	}
	return u.String(), nil
}

// PlaybackHeaders only shares the portal session with its own hostname, and
// never downgrades credentials established over HTTPS.
func (c *Client) PlaybackHeaders(streamURL string) map[string]string {
	headers := map[string]string{"User-Agent": UserAgent}
	sess := c.Session()
	base, err := url.Parse(sess.Endpoint)
	if err != nil || base.Hostname() == "" {
		return headers
	}
	stream, err := url.Parse(streamURL)
	if err != nil || stream.User != nil || (stream.Scheme != "http" && stream.Scheme != "https") || !strings.EqualFold(stream.Hostname(), base.Hostname()) || base.Scheme == "https" && stream.Scheme != "https" {
		return headers
	}
	headers["Cookie"] = fmt.Sprintf("mac=%s; stb_lang=en; timezone=%s", c.ID.MAC, timezone)
	if !sess.Simple && sess.Token != "" {
		headers["Authorization"] = "Bearer " + sess.Token
	}
	return headers
}

// catalogMetadata copies the descriptive fields a Ministra VOD row usually
// carries. Values are display text only; none of them takes part in playback.
func catalogMetadata(item *CatalogItem, row map[string]json.RawMessage) {
	item.HasPoster = item.Poster != ""
	if orig := field(row, "o_name", "original_name"); orig != "" && !strings.EqualFold(orig, item.Name) {
		item.OriginalName = clip(orig, 200)
	}
	item.Genres = clip(field(row, "genres_str", "genre_str"), 200)
	if id := field(row, "tmdb_id", "tmdb"); id != "" && id != "0" && strings.Trim(id, "0123456789") == "" {
		item.TMDBID = id
	}
	item.Age = clip(field(row, "age", "rating_mpaa"), 16)
	item.Director = clip(field(row, "director"), 200)
	item.Actors = clip(field(row, "actors"), 300)
	item.Country = clip(field(row, "country"), 100)
	item.Description = clip(item.Description, 2000)
	if y := item.Year; len(y) > 4 && y[4] == '-' {
		item.Year = y[:4] // "2019-05-01"
	}
	if item.Year == "0" {
		item.Year = ""
	}
	if item.Duration == "0" {
		item.Duration = ""
	}
	for _, r := range [][2]string{{"imdb", "rating_imdb"}, {"kinopoisk", "rating_kinopoisk"}} {
		if v := ratingValue(field(row, r[1])); v != "" {
			item.Ratings = append(item.Ratings, Rating{Source: r[0], Value: v, Origin: "provider"})
		}
	}
}

// ratingValue keeps one decimal of a 0–10 score; zero means "not rated".
func ratingValue(s string) string {
	f, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(s), ",", ".", 1), 64)
	if err != nil || f <= 0 || f > 10 {
		return ""
	}
	return strconv.FormatFloat(float64(int(f*10+0.5))/10, 'f', 1, 64)
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

// CacheSize estimates retained metadata, including unexported episode lists.
// Shared strings are deliberately counted per item to keep the budget conservative.
func (item CatalogItem) CacheSize() int {
	size := int(unsafe.Sizeof(item)) + cap(item.embedded)*int(unsafe.Sizeof("")) + cap(item.Ratings)*int(unsafe.Sizeof(Rating{}))
	for _, value := range []string{item.Key, item.Name, item.Kind, item.Description, item.Poster, item.Year, item.Duration, item.Type, item.ID, item.MovieID, item.SeasonID, item.Episode, item.Cmd, item.OriginalName, item.Genres, item.Age, item.Director, item.Actors, item.Country, item.Number, item.TMDBID, item.CatalogID} {
		size += len(value)
	}
	for _, r := range item.Ratings {
		size += len(r.Source) + len(r.Value) + len(r.Origin)
	}
	for _, value := range item.embedded {
		size += len(value)
	}
	return size
}
