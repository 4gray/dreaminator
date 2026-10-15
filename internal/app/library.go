package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/stalker"
)

// The library is what the TV catalog remembers about the viewer: favorite
// films and series, and the recently started rows. Favorite channels are not
// here: they are the channel list "Избранное", the same list the tablet
// edits, so the TV and the receiver's channel list have one set of
// favorites. The library lives in its own file (library.json): it changes
// far more often than state.json, which is large.

// FavoritesList is the channel list the TV adds channels to. It is found by
// name; when the viewer renamed or deleted it, adding a channel makes it anew.
const FavoritesList = "Избранное"

const (
	libraryWriteEvery = time.Minute // flash writes, at most
	// A burst of changes (several favorites in a row) becomes one write.
	libraryDebounce = 5 * time.Second
	recentMax       = 30
	favoritesMax    = 500
)

// LibraryEntry is one remembered row. Item holds what reopening a film or a
// series needs without its list; channels are found by id in the channel
// list instead.
type LibraryEntry struct {
	ID      string             `json:"id"` // persistent catalog id
	Source  string             `json:"source"`
	Section string             `json:"type"` // itv | vod | series
	Kind    string             `json:"kind"` // channel | movie | series
	Name    string             `json:"name"`
	At      time.Time          `json:"at"`
	Item    *stalker.SavedItem `json:"item,omitempty"`
	Episode string             `json:"episode,omitempty"` // recents: the episode of the series last started
}

type libraryFile struct {
	Version   int            `json:"version"`
	Favorites []LibraryEntry `json:"favorites"` // films and series, newest first
	Recent    []LibraryEntry `json:"recent"`    // channels, films and series, newest first
}

type library struct {
	libraryFile
	dirty   bool
	written time.Time
	due     time.Time // when the scheduled write runs
	timer   *time.Timer
}

func (a *App) libraryPath() string { return filepath.Join(a.cfg.DataDir, "library.json") }

// loadLibrary reads library.json (or its previous version after a power
// cut) and drops rows of sources that no longer exist.
func (a *App) loadLibrary() error {
	f := libraryFile{}
	if err := loadRecovering(a, a.libraryPath(), "библиотека", &f); err != nil {
		return err
	}
	a.lib.libraryFile = f
	a.lib.Version = 1
	known := map[string]bool{}
	for _, s := range a.st.Sources {
		known[s.ID] = true
	}
	before := len(a.lib.Favorites) + len(a.lib.Recent)
	a.lib.Favorites = keepEntries(a.lib.Favorites, func(e LibraryEntry) bool { return known[e.Source] })
	a.lib.Recent = keepEntries(a.lib.Recent, func(e LibraryEntry) bool { return known[e.Source] })
	if len(a.lib.Favorites)+len(a.lib.Recent) != before {
		a.libraryChangedLocked()
	}
	return nil
}

func keepEntries(list []LibraryEntry, keep func(LibraryEntry) bool) []LibraryEntry {
	out := make([]LibraryEntry, 0, len(list))
	for _, e := range list {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out
}

// libraryChangedLocked schedules a write: shortly after a quiet minute,
// otherwise when the minute since the last write is over.
func (a *App) libraryChangedLocked() {
	a.lib.dirty = true
	if a.lib.timer != nil {
		return
	}
	wait := max(libraryDebounce, time.Until(a.lib.written.Add(libraryWriteEvery)))
	a.lib.due = time.Now().Add(wait)
	a.lib.timer = time.AfterFunc(wait, func() { a.SaveLibrary() })
}

// SaveLibrary writes library.json now if it changed: on its schedule, when
// the TV catalog closes and when the service stops.
func (a *App) SaveLibrary() error {
	a.libWriteMu.Lock()
	defer a.libWriteMu.Unlock()
	a.mu.Lock()
	if a.lib.timer != nil {
		a.lib.timer.Stop()
		a.lib.timer = nil
	}
	if !a.lib.dirty {
		a.mu.Unlock()
		return nil
	}
	data, err := json.Marshal(&a.lib.libraryFile)
	a.lib.dirty = false
	a.lib.written = time.Now()
	a.mu.Unlock()
	if err == nil {
		err = enigma.WriteFileAtomicBackup(a.libraryPath(), data)
	}
	if err != nil {
		a.log.Printf("не удалось сохранить библиотеку: %v", err)
		a.mu.Lock()
		a.libraryChangedLocked() // try again in a minute
		a.mu.Unlock()
	}
	return err
}

// favoritesBouquetLocked is the channel list called "Избранное", if any.
func (a *App) favoritesBouquetLocked() *Bouquet {
	for _, b := range a.st.Bouquets {
		if strings.EqualFold(strings.TrimSpace(b.Name), FavoritesList) {
			return b
		}
	}
	return nil
}

// libraryItemsLocked lists the library rows of one source that can be
// opened now: favorite channels still in the playlist, remembered films and
// series. Each carries its persistent id.
func (a *App) libraryItemsLocked(source string) []stalker.CatalogItem {
	var out []stalker.CatalogItem
	if b := a.favoritesBouquetLocked(); b != nil {
		for _, r := range b.Channels {
			if r.Source != source {
				continue
			}
			if ch := a.channel(source, r.ID); ch != nil {
				out = append(out, channelItem(source, *ch, 0))
			}
		}
	}
	for _, list := range [][]LibraryEntry{a.lib.Favorites, a.lib.Recent} {
		for _, e := range list {
			if e.Source != source {
				continue
			}
			if item, ok := a.entryItemLocked(e); ok {
				out = append(out, item)
			}
		}
	}
	return out
}

// entryItemLocked is the catalog row a library entry stands for; false when
// its channel left the playlist.
func (a *App) entryItemLocked(e LibraryEntry) (stalker.CatalogItem, bool) {
	if e.Section == "itv" {
		_, _, rest, ok := parseCatalogID(e.ID)
		if !ok || len(rest) != 1 {
			return stalker.CatalogItem{}, false
		}
		ch := a.channel(e.Source, rest[0])
		if ch == nil {
			return stalker.CatalogItem{}, false
		}
		return channelItem(e.Source, *ch, 0), true
	}
	if e.Item == nil {
		return stalker.CatalogItem{}, false
	}
	item := e.Item.Item()
	item.CatalogID = e.ID
	return item, true
}

func libraryEntry(source string, item stalker.CatalogItem) LibraryEntry {
	e := LibraryEntry{ID: item.CatalogID, Source: source, Section: item.Type, Kind: item.Kind, Name: item.Name, At: time.Now()}
	if item.Kind != "channel" {
		saved := item.Saved()
		e.Item = &saved
	}
	return e
}

// noteRecentLocked makes a started row the newest recent: the channel, the
// film, or for an episode its series (with the episode noted).
func (a *App) noteRecentLocked(source string, cache *catalogStore, rec catalogRecord) {
	item := rec.item
	var e LibraryEntry
	switch {
	case item.CatalogID == "":
		return
	case item.Kind == "channel" || item.Kind == "movie" && rec.parent == "":
		e = libraryEntry(source, item)
	case item.Kind == "episode":
		season, ok := a.catalogRecordLocked(source, cache, rec.parent)
		if !ok {
			return
		}
		series, ok := a.catalogRecordLocked(source, cache, season.parent)
		if !ok || series.parent != "" || series.item.Kind != "series" {
			return
		}
		e = libraryEntry(source, series.item)
		e.Episode = item.CatalogID
	default:
		return
	}
	recent := make([]LibraryEntry, 0, recentMax)
	recent = append(recent, e)
	for _, old := range a.lib.Recent {
		if old.ID != e.ID && len(recent) < recentMax {
			recent = append(recent, old)
		}
	}
	a.lib.Recent = recent
	a.libraryChangedLocked()
}

// refreshLibraryLocked updates remembered films and series that a fresh
// list shows again: a new poster or command reaches the library too.
func (a *App) refreshLibraryLocked(source string, items []stalker.CatalogItem) {
	if len(a.lib.Favorites)+len(a.lib.Recent) == 0 {
		return
	}
	seen := map[string]int{}
	for i, item := range items {
		if item.Kind == "movie" || item.Kind == "series" {
			seen[item.CatalogID] = i
		}
	}
	changed := false
	for _, list := range [][]LibraryEntry{a.lib.Favorites, a.lib.Recent} {
		for i := range list {
			e := &list[i]
			n, ok := seen[e.ID]
			if !ok || e.Source != source || e.Item == nil {
				continue
			}
			if saved := items[n].Saved(); !saved.Same(*e.Item) {
				e.Item, e.Name, changed = &saved, items[n].Name, true
			}
		}
	}
	if changed {
		a.libraryChangedLocked()
	}
}

// forgetSourceLocked drops the library rows of a deleted source; its
// channels leave the lists with the rest of its channel refs.
func (a *App) forgetSourceLocked(id string) {
	before := len(a.lib.Favorites) + len(a.lib.Recent)
	other := func(e LibraryEntry) bool { return e.Source != id }
	a.lib.Favorites = keepEntries(a.lib.Favorites, other)
	a.lib.Recent = keepEntries(a.lib.Recent, other)
	if len(a.lib.Favorites)+len(a.lib.Recent) != before {
		a.libraryChangedLocked()
	}
}

// ---- favorites ----

// FavoriteResult tells the TV where a favorite went and what the next send
// to the receiver will carry.
type FavoriteResult struct {
	Fav     bool   `json:"fav"`
	Channel bool   `json:"channel"`           // a channel: it is in the channel list List
	List    string `json:"list,omitempty"`    // that list's name
	Created bool   `json:"created,omitempty"` // the list was missing and was made again
	// Unsent: the tablet had changes not yet on the TV; sending this
	// channel (when the catalog closes) sends them too.
	Unsent bool `json:"unsent,omitempty"`
}

var errFavoritesFull = fmt.Errorf("в избранном уже %d фильмов и сериалов — уберите что-нибудь", favoritesMax)

// SetFavorite adds a row to the favorites or removes it. The row is named by
// its navigation key or, once that is gone, by its persistent id. Channels go to
// the channel list "Избранное", films and series to the library; seasons
// and episodes cannot be favorites.
func (a *App) SetFavorite(source, key, entry string, on bool) (FavoriteResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cache, _, err := a.catalogLocked(source)
	if err != nil {
		return FavoriteResult{}, err
	}
	rec, ok := a.catalogRecordLocked(source, cache, key)
	if !ok && entry != "" {
		_, rec, ok = a.catalogRecordByIDLocked(source, cache, entry)
	}
	id := entry
	if ok {
		id = rec.item.CatalogID
	}
	src, section, rest, valid := parseCatalogID(id)
	if !valid || src != source || len(rest) != 1 {
		if !ok && entry == "" {
			return FavoriteResult{}, ErrCatalogExpired
		}
		return FavoriteResult{}, ErrCatalogInput
	}
	if section == "itv" {
		return a.setChannelFavoriteLocked(source, rest[0], on)
	}
	pos := -1
	for i, e := range a.lib.Favorites {
		if e.ID == id {
			pos = i
		}
	}
	switch {
	case !on:
		if pos >= 0 {
			a.lib.Favorites = append(a.lib.Favorites[:pos:pos], a.lib.Favorites[pos+1:]...)
			a.libraryChangedLocked()
		}
		return FavoriteResult{}, nil
	case pos >= 0:
		return FavoriteResult{Fav: true}, nil
	case !ok:
		return FavoriteResult{}, ErrCatalogExpired
	case rec.item.Kind != "movie" && rec.item.Kind != "series":
		return FavoriteResult{}, ErrCatalogInput
	case len(a.lib.Favorites) >= favoritesMax:
		return FavoriteResult{}, errFavoritesFull
	}
	a.lib.Favorites = append([]LibraryEntry{libraryEntry(source, rec.item)}, a.lib.Favorites...)
	a.libraryChangedLocked()
	return FavoriteResult{Fav: true}, nil
}

func (a *App) setChannelFavoriteLocked(source, channel string, on bool) (FavoriteResult, error) {
	res := FavoriteResult{Channel: true, List: FavoritesList, Fav: on}
	b := a.favoritesBouquetLocked()
	pos := -1
	if b != nil {
		res.List = b.Name
		for i, r := range b.Channels {
			if r.Source == source && r.ID == channel {
				pos = i
			}
		}
	}
	if on == (pos >= 0) {
		return res, nil
	}
	res.Unsent = a.st.Dirty
	if on {
		ch := a.channel(source, channel)
		if ch == nil {
			return FavoriteResult{}, ErrCatalogExpired
		}
		if b == nil {
			b = &Bouquet{ID: newID(), Name: FavoritesList, Channels: []ChannelRef{}}
			a.st.Bouquets = append([]*Bouquet{b}, a.st.Bouquets...)
			res.Created = true
		}
		b.Channels = append(b.Channels, ChannelRef{Source: source, ID: channel, Name: ch.Name})
	} else {
		b.Channels = append(b.Channels[:pos:pos], b.Channels[pos+1:]...)
	}
	a.touch()
	return res, a.save()
}

// favoriteIDsLocked holds the persistent ids of every favorite.
func (a *App) favoriteIDsLocked() map[string]bool {
	out := map[string]bool{}
	if b := a.favoritesBouquetLocked(); b != nil {
		for _, r := range b.Channels {
			out[catalogID(r.Source, "itv", r.ID)] = true
		}
	}
	for _, e := range a.lib.Favorites {
		out[e.ID] = true
	}
	return out
}

// MarkLibrary flags the favorites among listed rows (the star on the TV).
// Lists are cached; favorites change, so the flags are set per answer.
func (a *App) MarkLibrary(cards *CatalogCards) {
	if len(cards.Items) == 0 {
		return
	}
	a.mu.Lock()
	fav := a.favoriteIDsLocked()
	a.mu.Unlock()
	for i := range cards.Items {
		cards.Items[i].Fav = fav[cards.Items[i].ID]
	}
}

// ---- the library on the TV: "Избранное" and "Недавние" ----

// HomeCategory is a category of a library list, with its number of rows.
type HomeCategory struct {
	ID    string `json:"id"` // itv | video
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CatalogHome is the summary behind the rail's "Избранное" and "Недавние".
type CatalogHome struct {
	Favorites []HomeCategory `json:"favorites"`
	Recent    []HomeCategory `json:"recent"`
	List      string         `json:"list"` // the channel list that holds favorite channels
}

var homeLists = map[string]bool{"favorites": true, "recent": true}
var homeCategoryNames = map[string]string{"itv": "Каналы", "video": "Фильмы и сериалы"}

type homeRow struct {
	source  string
	item    stalker.CatalogItem
	missing bool
}

func (a *App) CatalogHome() CatalogHome {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := CatalogHome{List: FavoritesList}
	if b := a.favoritesBouquetLocked(); b != nil {
		out.List = b.Name
	}
	for list := range homeLists {
		cats := []HomeCategory{}
		for _, id := range []string{"itv", "video"} {
			cats = append(cats, HomeCategory{ID: id, Name: homeCategoryNames[id], Count: len(a.homeRowsLocked(list, id))})
		}
		if list == "favorites" {
			out.Favorites = cats
		} else {
			out.Recent = cats
		}
	}
	return out
}

// homeRowsLocked lists one category of a library list in its order:
// favorite channels as the channel list orders them, the rest newest first.
func (a *App) homeRowsLocked(list, category string) []homeRow {
	rows := []homeRow{}
	add := func(source string, item stalker.CatalogItem, missing bool) {
		rows = append(rows, homeRow{source: source, item: item, missing: missing})
	}
	entries := a.lib.Recent
	if list == "favorites" {
		entries = a.lib.Favorites
		if b := a.favoritesBouquetLocked(); b != nil && category == "itv" {
			for _, r := range b.Channels {
				if ch := a.channel(r.Source, r.ID); ch != nil {
					add(r.Source, channelItem(r.Source, *ch, 0), false)
				} else if a.source(r.Source) != nil {
					add(r.Source, stalker.CatalogItem{ID: r.ID, Type: "itv", Kind: "channel", Name: r.Name, CatalogID: catalogID(r.Source, "itv", r.ID)}, true)
				}
			}
		}
	}
	for _, e := range entries {
		if (e.Section == "itv") != (category == "itv") || a.source(e.Source) == nil {
			continue
		}
		if item, ok := a.entryItemLocked(e); ok {
			add(e.Source, item, false)
		} else if _, _, rest, ok := parseCatalogID(e.ID); ok && e.Section == "itv" && len(rest) == 1 {
			add(e.Source, stalker.CatalogItem{ID: rest[0], Type: "itv", Kind: "channel", Name: e.Name, CatalogID: e.ID}, true)
		}
	}
	return rows
}

// CatalogHomeItems serves one page of a library list. Rows come from every
// source and say which; their keys work with the source's catalog calls
// (card, seasons, pictures, play), like the keys of a listed row.
func (a *App) CatalogHomeItems(list, category string, page int) (cards CatalogCards, err error) {
	empty := CatalogCards{Items: []CatalogCard{}, Page: page}
	if !homeLists[list] || homeCategoryNames[category] == "" || page < 1 || page > 1000 {
		return empty, ErrCatalogInput
	}
	if category == "itv" {
		defer func() { a.homeNow(cards.Items) }()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := a.homeRowsLocked(list, category)
	fav := a.favoriteIDsLocked()
	first := (page - 1) * catalogPageSize
	out := CatalogCards{Items: []CatalogCard{}, Page: page, PageSize: catalogPageSize, Total: len(rows), HasMore: first+catalogPageSize < len(rows)}
	for i := first; i < len(rows) && i < first+catalogPageSize; i++ {
		r := rows[i]
		item := r.item
		key := catalogKey(r.source, item.Type, "", item)
		if !r.missing {
			// A row already listed keeps its fuller entry (description, number).
			if cache, _, err := a.catalogLocked(r.source); err == nil {
				a.rememberLocked(cache, key, item)
			}
		}
		card := CatalogCard{Key: key, ID: item.CatalogID, Name: item.Name, Kind: item.Kind, Year: item.Year, Duration: item.Duration,
			HasPoster: item.HasPoster && !r.missing, Fav: fav[item.CatalogID], Source: r.source, Section: item.Type, Missing: r.missing}
		if len(item.Ratings) > 0 {
			rating := item.Ratings[0]
			card.Rating = &rating
		}
		if category == "itv" {
			card.Number = fmt.Sprint(i + 1)
		}
		out.Items = append(out.Items, card)
	}
	return out, nil
}

// homeNow adds what is on now to favorite and recent channels, like the
// rows of a source's Live TV (guide.go). Called without a.mu.
func (a *App) homeNow(cards []CatalogCard) {
	if len(cards) == 0 {
		return
	}
	a.mu.Lock()
	for i := range cards {
		_, _, rest, ok := parseCatalogID(cards[i].ID)
		if cards[i].Kind != "channel" || !ok || len(rest) != 1 {
			continue
		}
		if ch := a.channel(cards[i].Source, rest[0]); ch != nil && ch.Catchup != nil {
			cards[i].Archive = ch.Catchup.Days
		}
	}
	a.mu.Unlock()
	a.maybeReloadGuide()
	now := a.now()
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	for i := range cards {
		_, _, rest, ok := parseCatalogID(cards[i].ID)
		if cards[i].Missing || cards[i].Kind != "channel" || !ok || len(rest) != 1 {
			continue
		}
		if pr, ok := gi.nowOn(cards[i].Source+"/"+rest[0], now); ok {
			e := rowEntry(pr)
			cards[i].Now = &e
		}
	}
}
