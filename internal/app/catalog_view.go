package app

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"dreaminator/internal/stalker"
)

// CatalogCard is what a grid or a list row shows. Long texts (description,
// cast) stay for the card screen: the receiver's Python spends tens of
// milliseconds per kilobyte of JSON, and that time is taken from the remote.
type CatalogCard struct {
	Key       string          `json:"key"`
	ID        string          `json:"id,omitempty"` // persistent id (catalog_id.go)
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Year      string          `json:"year,omitempty"`
	Duration  string          `json:"duration,omitempty"`
	Rating    *stalker.Rating `json:"rating,omitempty"`
	HasPoster bool            `json:"hasPoster,omitempty"`
	Number    string          `json:"number,omitempty"`
	Now       *GuideEntry     `json:"now,omitempty"` // channels: what is on, when known (guide.go)
	Fav       bool            `json:"fav,omitempty"`
	Archive   int             `json:"archive,omitempty"` // channels: days of recordings (guide_archive.go)
	// Library rows only (GET /api/catalog/home/items): rows of every source.
	Source  string `json:"source,omitempty"`
	Section string `json:"type,omitempty"`
	Missing bool   `json:"missing,omitempty"` // no longer in the playlist
}

type CatalogCards struct {
	Items    []CatalogCard `json:"items"`
	Page     int           `json:"page"`
	HasMore  bool          `json:"hasMore"`
	PageSize int           `json:"pageSize,omitempty"`
	Total    int           `json:"total,omitempty"`
}

func Cards(p stalker.CatalogPage) CatalogCards {
	out := CatalogCards{Items: make([]CatalogCard, 0, len(p.Items)), Page: p.Page, HasMore: p.HasMore, PageSize: p.PageSize, Total: p.Total}
	for _, item := range p.Items {
		card := CatalogCard{Key: item.Key, ID: item.CatalogID, Name: item.Name, Kind: item.Kind, Year: item.Year, Duration: item.Duration, HasPoster: item.HasPoster, Number: item.Number}
		if len(item.Ratings) > 0 {
			r := item.Ratings[0]
			card.Rating = &r
		}
		out.Items = append(out.Items, card)
	}
	return out
}

// CatalogDetails is the card screen: everything the provider said about one
// entry, by its navigation key, plus TMDB facts when the entry is matched.
type CatalogDetails struct {
	stalker.CatalogItem
	Section     string `json:"type"`
	HasBackdrop bool   `json:"hasBackdrop"`
}

// catalogEntry finds a navigation key; callers must not hold a.mu.
func (a *App) catalogEntry(id, key string) (stalker.CatalogItem, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cache, _, err := a.catalogLocked(id)
	if err != nil {
		return stalker.CatalogItem{}, err
	}
	rec, ok := a.catalogRecordLocked(id, cache, key)
	if !ok {
		return stalker.CatalogItem{}, ErrCatalogExpired
	}
	return rec.item, nil
}

func (a *App) CatalogDetails(ctx context.Context, id, key string) (CatalogDetails, error) {
	item, err := a.catalogEntry(id, key)
	if err != nil {
		return CatalogDetails{}, err
	}
	return CatalogDetails{CatalogItem: item, Section: item.Type}, nil
}

// CatalogEpisode decorates an episode the portal lists (never adds one).
type CatalogEpisode struct {
	Number   int    `json:"number"`
	Name     string `json:"name,omitempty"`
	Overview string `json:"overview,omitempty"`
	Runtime  int    `json:"runtime,omitempty"`
	HasStill bool   `json:"hasStill,omitempty"`
}

// CatalogTMDB returns TMDB facts for a card, or nil. It is a separate call
// so the card shows the portal's data at once; a TMDB problem only means
// "no TMDB facts", never an error on the TV.
func (a *App) CatalogTMDB(ctx context.Context, id, key string) (*TMDBInfo, error) {
	item, err := a.catalogEntry(id, key)
	if err != nil {
		return nil, err
	}
	info, _ := a.TMDBFor(ctx, id, item)
	return info, nil
}

// CatalogEpisodes returns TMDB names for the episodes of one portal season,
// sorted by number. The TV applies them only to episodes the portal lists.
func (a *App) CatalogEpisodes(ctx context.Context, id, key string) ([]CatalogEpisode, error) {
	season, err := a.catalogEntry(id, key)
	if err != nil {
		return nil, err
	}
	out := []CatalogEpisode{}
	if season.Kind != "season" {
		return out, nil
	}
	series := a.catalogParent(id, key)
	eps, _ := a.TMDBEpisodes(ctx, id, season, seasonNumber(season, series))
	for _, e := range eps {
		out = append(out, CatalogEpisode{Number: e.Number, Name: e.Name, Overview: e.Overview, Runtime: e.Runtime, HasStill: e.Still != ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (a *App) catalogParent(id, key string) stalker.CatalogItem {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.catalogs[id]; c != nil {
		if rec, ok := a.catalogRecordLocked(id, c, key); ok {
			parent, _ := a.catalogRecordLocked(id, c, rec.parent)
			return parent.item
		}
	}
	return stalker.CatalogItem{}
}

var seasonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:^|[^\pL\d])s(\d{1,2})(?:e\d+)?(?:$|[^\pL\d])`),
	regexp.MustCompile(`(?i)(\d{1,2})\s*-?\s*(?:й\s*)?(?:сезон|season)`),
	regexp.MustCompile(`(?i)(?:сезон|season)\s*(\d{1,2})`),
}

// seasonNumber maps a portal season to TMDB's numbering: the portal's own
// number, then "Сезон 2" / "S02" / "(3 сезон)" in the season or series name.
func seasonNumber(season, series stalker.CatalogItem) int {
	if n, err := strconv.Atoi(strings.TrimSpace(season.Number)); err == nil && n > 0 {
		return n
	}
	for _, name := range []string{season.Name, series.Name, series.OriginalName} {
		for _, re := range seasonPatterns {
			if m := re.FindStringSubmatch(name); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	return 1
}

const backdropCacheBytes = 3 << 19 // 1.5 MiB of shaded backdrops

// CatalogImage is the artwork of a card: the portal's poster (TMDB's when
// the portal has none), the shaded TMDB backdrop, an actor's portrait
// (n = billing index) or an episode still (key = the episode).
func (a *App) CatalogImage(ctx context.Context, id, key, kind string, n int) (Artwork, error) {
	switch kind {
	case "cast":
		item, err := a.catalogEntry(id, key)
		if err != nil {
			return Artwork{}, err
		}
		return a.TMDBCastPhoto(ctx, id, item, n)
	case "still":
		episode, err := a.catalogEntry(id, key)
		if err != nil {
			return Artwork{}, err
		}
		if episode.Kind != "episode" {
			return Artwork{}, ErrNotFound
		}
		season := a.catalogParent(id, key)
		series := a.catalogParent(id, season.Key)
		return a.TMDBStill(ctx, id, episode, seasonNumber(season, series))
	case "", "poster":
		art, err := a.CatalogPoster(ctx, id, key)
		if err == nil || errors.Is(err, ErrCatalogExpired) {
			return art, err
		}
		item, ierr := a.catalogEntry(id, key)
		if ierr != nil {
			return Artwork{}, ierr
		}
		if tm, terr := a.TMDBArtwork(ctx, id, item, "poster"); terr == nil {
			return tm, nil
		}
		return Artwork{}, err
	case "backdrop":
		return a.catalogBackdrop(ctx, id, key)
	}
	return Artwork{}, ErrCatalogInput
}

func (a *App) catalogBackdrop(ctx context.Context, id, key string) (Artwork, error) {
	item, err := a.catalogEntry(id, key)
	if err != nil {
		return Artwork{}, err
	}
	a.mu.Lock()
	cache := a.catalogs[id]
	if cache != nil {
		if art, ok := cache.backdrops[key]; ok {
			a.mu.Unlock()
			return art, nil
		}
	}
	a.mu.Unlock()
	raw, err := a.TMDBArtwork(ctx, id, item, "backdrop")
	if err != nil {
		return Artwork{}, ErrNotFound
	}
	// Decoding is the slow path on the receiver: one picture at a time.
	a.catalogPosterMu.Lock()
	art, err := shadeBackdrop(raw.Data)
	a.catalogPosterMu.Unlock()
	if err != nil {
		return Artwork{}, ErrNotFound
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if cache != nil && a.catalogs[id] == cache {
		if cache.backdropBytes+len(art.Data) > backdropCacheBytes {
			cache.backdrops = map[string]Artwork{}
			cache.backdropBytes = 0
		}
		if _, ok := cache.backdrops[key]; !ok {
			cache.backdrops[key] = art
			cache.backdropBytes += len(art.Data)
		}
	}
	return art, nil
}
