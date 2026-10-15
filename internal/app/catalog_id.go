package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"

	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
)

// Every catalog row carries two identities. The navigation key is opaque and
// lives as long as the cached list it came from (a refreshed list answers
// HTTP 410). The persistent id survives refreshes and restarts, so the
// library (favorites, recents) and playback by id rely on it:
//
//	<source>/<type>/<provider id>                           channel, film, series
//	<source>/<type>/<series id>/<season id>                 season
//	<source>/<type>/<series id>/<season id>/<episode no.>   episode
//
// type is the catalog section (itv, vod, series); every part is
// path-escaped, so provider ids with slashes stay one part.
func catalogID(parts ...string) string {
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// parseCatalogID splits a persistent id; rest holds the provider parts.
func parseCatalogID(id string) (source, section string, rest []string, ok bool) {
	parts := strings.Split(id, "/")
	if len(parts) < 3 || len(parts) > 5 {
		return "", "", nil, false
	}
	for i, p := range parts {
		v, err := url.PathUnescape(p)
		if err != nil || v == "" {
			return "", "", nil, false
		}
		parts[i] = v
	}
	if !catalogSection(parts[1]) {
		return "", "", nil, false
	}
	return parts[0], parts[1], parts[2:], true
}

// rowCatalogID is the persistent id of an entry listed under parent (nil
// for a category or a search).
func rowCatalogID(source, section string, parent *stalker.CatalogItem, item stalker.CatalogItem) string {
	if parent == nil {
		return catalogID(source, section, item.ID)
	}
	base := parent.CatalogID
	if base == "" {
		base = catalogID(source, section, parent.ID)
	}
	part := item.ID
	if parent.Kind == "season" && item.Episode != "" {
		part = item.Episode
	}
	return base + "/" + url.PathEscape(part)
}

// catalogKey is the navigation key of an entry: the same entry in the same
// place always gets the same key, so a favorite opened from the library and
// the same row in its category are one entry of the cache.
func catalogKey(source, section, parent string, item stalker.CatalogItem) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{source, section, parent, item.Kind, item.ID, item.Episode}, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// channelItem is a Live TV row; number is its position in the list shown.
func channelItem(source string, ch playlist.Channel, number int) stalker.CatalogItem {
	item := stalker.CatalogItem{ID: ch.ID, Type: "itv", Kind: "channel", Name: ch.Name, Poster: ch.Logo, HasPoster: ch.Logo != "",
		Cmd: ch.Cmd, NeedsLink: ch.NeedsLink, CatalogID: catalogID(source, "itv", ch.ID)}
	if number > 0 {
		item.Number = strconv.Itoa(number)
	}
	return item
}

// catalogRecordLocked finds a navigation key among the cached lists and,
// once they were dropped (a refresh, the cache budget), among the library
// rows: favorites and recents stay openable while the lists come and go.
// Called under a.mu.
func (a *App) catalogRecordLocked(source string, cache *catalogStore, key string) (catalogRecord, bool) {
	if rec, ok := cache.items[key]; ok {
		return rec, true
	}
	if rec, ok := cache.library[key]; ok {
		return rec, true
	}
	if key == "" {
		return catalogRecord{}, false
	}
	for _, item := range a.libraryItemsLocked(source) {
		if catalogKey(source, item.Type, "", item) == key {
			return a.rememberLocked(cache, key, item), true
		}
	}
	return catalogRecord{}, false
}

// catalogRecordByIDLocked finds a row by its persistent id: among the cached
// lists, then in the channel list (Live TV), then in the library.
func (a *App) catalogRecordByIDLocked(source string, cache *catalogStore, id string) (string, catalogRecord, bool) {
	for _, m := range []map[string]catalogRecord{cache.items, cache.library} {
		for key, rec := range m {
			if rec.item.CatalogID == id {
				return key, rec, true
			}
		}
	}
	src, section, rest, ok := parseCatalogID(id)
	if !ok || src != source || len(rest) != 1 {
		return "", catalogRecord{}, false // seasons and episodes live in their lists only
	}
	if section == "itv" {
		if ch := a.channel(source, rest[0]); ch != nil {
			item := channelItem(source, *ch, 0)
			key := catalogKey(source, "itv", "", item)
			return key, a.rememberLocked(cache, key, item), true
		}
		return "", catalogRecord{}, false
	}
	for _, item := range a.libraryItemsLocked(source) {
		if item.CatalogID == id {
			key := catalogKey(source, item.Type, "", item)
			return key, a.rememberLocked(cache, key, item), true
		}
	}
	return "", catalogRecord{}, false
}

// Library rows brought back into a cache have their own budget: they are
// rebuilt from the library on demand, so a full set is simply dropped.
const (
	libraryCacheItems = 1024
	libraryCacheBytes = 1 << 20
)

// rememberLocked puts a library row back into the cache as a root entry, so
// its card, seasons and pictures work like those of a listed row. A listed
// row (cache.items) always wins over its library copy.
func (a *App) rememberLocked(cache *catalogStore, key string, item stalker.CatalogItem) catalogRecord {
	if rec, ok := cache.items[key]; ok {
		return rec
	}
	item.Key = key
	size := item.CacheSize()
	if prev, ok := cache.library[key]; ok {
		cache.libraryBytes -= prev.item.CacheSize()
	} else if len(cache.library) >= libraryCacheItems || cache.libraryBytes+size > libraryCacheBytes {
		cache.library = map[string]catalogRecord{}
		cache.libraryBytes = 0
	}
	rec := catalogRecord{item: item}
	cache.library[key] = rec
	cache.libraryBytes += size
	return rec
}
