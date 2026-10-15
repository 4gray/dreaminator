package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"dreaminator/internal/stalker"
)

// CatalogSearchResult says how far a search reached, so the TV can tell
// "nothing found" apart from "only the pages you opened were searched".
type CatalogSearchResult struct {
	CatalogCards
	Query    string `json:"query"`
	Scope    string `json:"scope"`              // server | channels | loaded
	Searched int    `json:"searched,omitempty"` // entries looked through when scope is "loaded"
}

const searchPageSize = 50

// A term no title contains: a portal that answers it with results ignores
// the search parameter and returns its whole catalog instead.
const searchProbe = "zqxjvq9k"

const (
	searchUnknown = iota
	searchServer
	searchLocal
)

// CatalogSearch looks for titles in one section of one portal. Live TV is
// filtered over the full channel list; films and series are searched by the
// portal when it supports it, otherwise among the pages already loaded —
// never by walking the whole portal.
func (a *App) CatalogSearch(ctx context.Context, id, section, query string, page int) (CatalogSearchResult, error) {
	q := strings.Join(strings.Fields(query), " ")
	n := utf8.RuneCountInString(q)
	if !catalogSection(section) || n < 2 || n > 60 || page < 1 || page > 1000 {
		return CatalogSearchResult{}, ErrCatalogInput
	}
	out := CatalogSearchResult{Query: q}
	if section == "itv" {
		res, err := a.catalogPage(ctx, id, pageRequest{section: section, search: q, page: page})
		out.CatalogCards, out.Scope = a.CatalogCards(id, res), "channels"
		return out, err
	}
	if a.searchMode(id, section) != searchLocal {
		res, err := a.catalogPage(ctx, id, pageRequest{section: section, search: q, page: page})
		switch {
		case errors.Is(err, stalker.ErrCatalogUnsupported):
			a.setSearchMode(id, section, searchLocal)
		case err != nil:
			return out, err
		default:
			if a.searchMode(id, section) == searchUnknown {
				a.setSearchMode(id, section, a.probeSearch(ctx, id, section, len(res.Items) > 0))
			}
			if a.searchMode(id, section) != searchLocal {
				out.CatalogCards, out.Scope = Cards(res), "server"
				return out, nil
			}
		}
	}
	return a.localSearch(id, section, q, page)
}

// probeSearch decides whether the portal honours "search". Results for a
// real query prove nothing on their own: some portals ignore the parameter.
func (a *App) probeSearch(ctx context.Context, id, section string, found bool) int {
	if !found {
		return searchServer // an ignoring portal would have listed everything
	}
	probe, err := a.catalogPage(ctx, id, pageRequest{section: section, search: searchProbe, page: 1})
	if err != nil {
		return searchUnknown
	}
	if len(probe.Items) > 0 {
		return searchLocal
	}
	return searchServer
}

func (a *App) searchMode(id, section string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.catalogs[id]; c != nil {
		return c.search[section]
	}
	return searchUnknown
}

func (a *App) setSearchMode(id, section string, mode int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.catalogs[id]; c != nil {
		c.search[section] = mode
	}
}

// localSearch filters the entries the viewer has already loaded.
func (a *App) localSearch(id, section, q string, page int) (CatalogSearchResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cache, _, err := a.catalogLocked(id)
	if err != nil {
		return CatalogSearchResult{}, err
	}
	words := searchWords(q)
	found := []stalker.CatalogItem{}
	searched := 0
	for _, rec := range cache.items {
		item := rec.item
		if item.Type != section || rec.parent != "" || item.Kind != "movie" && item.Kind != "series" {
			continue
		}
		searched++
		if matchesWords(item.Name, words) || matchesWords(item.OriginalName, words) {
			found = append(found, item)
		}
	}
	sort.Slice(found, func(i, j int) bool { return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name) })
	first := (page - 1) * searchPageSize
	pg := stalker.CatalogPage{Items: []stalker.CatalogItem{}, Page: page, PageSize: searchPageSize, Total: len(found)}
	if first < len(found) {
		pg.Items = found[first:min(first+searchPageSize, len(found))]
		pg.HasMore = first+searchPageSize < len(found)
	}
	return CatalogSearchResult{CatalogCards: Cards(pg), Query: q, Scope: "loaded", Searched: searched}, nil
}

// searchWords folds case, ё and punctuation so "Матрица: перезагрузка" and
// "матрица перезагрузка" find the same titles.
func searchWords(s string) []string {
	return strings.Fields(strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		switch {
		case r == 'ё':
			return 'е'
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			return r
		}
		return ' '
	}, s))
}

func matchesWords(name string, words []string) bool {
	if len(words) == 0 {
		return true
	}
	have := " " + strings.Join(searchWords(name), " ")
	for _, w := range words {
		if !strings.Contains(have, " "+w) {
			return false
		}
	}
	return true
}
