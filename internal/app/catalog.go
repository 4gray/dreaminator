package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"dreaminator/internal/stalker"
)

var ErrCatalogExpired = errors.New("каталог обновился — вернитесь к категориям и откройте список заново")
var ErrCatalogInput = errors.New("неверный запрос каталога")

const catalogPageSize = 50
const catalogTTL = 10 * time.Minute
const catalogMaxItems = 2048
const catalogMaxBytes = 4 << 20

// CatalogSource is a source the TV can browse. Stalker portals have the
// whole catalog; playlists (M3U, uploaded files, Xtream channel lists) only
// Live TV, built from the channels the service already keeps.
type CatalogSource struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Sections []string `json:"sections"`
}
type CatalogPlayback struct {
	URL         string            `json:"url"`
	Name        string            `json:"name"`
	ServiceType int               `json:"serviceType"`
	Headers     map[string]string `json:"headers"`
}
type catalogRecord struct {
	item   stalker.CatalogItem
	parent string
}
type catalogStore struct {
	fingerprint   string
	created       time.Time
	items         map[string]catalogRecord
	pages         map[string]stalker.CatalogPage
	categories    map[string][]stalker.CatalogCategory
	posters       map[string]Artwork
	search        map[string]int // section -> how the portal searches (catalog_search.go)
	backdrops     map[string]Artwork
	library       map[string]catalogRecord // rows brought back from the library (catalog_id.go)
	libraryBytes  int
	backdropBytes int
	posterBytes   int
	itemBytes     int
	pageBytes     int
	categoryBytes int
}

func newCatalogStore(fp string) *catalogStore {
	return &catalogStore{fingerprint: fp, created: time.Now(), items: map[string]catalogRecord{}, pages: map[string]stalker.CatalogPage{}, categories: map[string][]stalker.CatalogCategory{}, posters: map[string]Artwork{}, search: map[string]int{}, backdrops: map[string]Artwork{}, library: map[string]catalogRecord{}}
}

func (a *App) CatalogSources() []CatalogSource {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []CatalogSource{}
	for _, s := range a.st.Sources {
		out = append(out, catalogSource(s))
	}
	return out
}

func catalogSource(s *Source) CatalogSource {
	sections := []string{"itv"}
	if s.Type == "stalker" {
		sections = []string{"itv", "vod", "series"}
	}
	return CatalogSource{ID: s.ID, Name: s.Name, Type: s.Type, Sections: sections}
}

// catalogFingerprint changes when the source points somewhere else; cached
// navigation keys of the old identity then expire.
func catalogFingerprint(s Source) string {
	if s.Type == "stalker" {
		return stalkerFingerprint(s)
	}
	return strings.Join([]string{s.Type, s.URL, s.Server, s.Username, s.Password}, "\x00")
}

// Called under a.mu. Source identity and generation also protect in-flight
// requests. The portal client is nil for playlists: they have Live TV only.
func (a *App) catalogLocked(id string) (*catalogStore, *stalker.Client, error) {
	s := a.source(id)
	if s == nil {
		return nil, nil, ErrNotFound
	}
	if a.catalogs == nil {
		a.catalogs = map[string]*catalogStore{}
	}
	fp := catalogFingerprint(*s)
	cache := a.catalogs[id]
	if cache == nil || cache.fingerprint != fp {
		cache = newCatalogStore(fp)
		a.catalogs[id] = cache
	} else if time.Since(cache.created) > catalogTTL {
		// Browsing identities survive a long movie. Only fetched pages/art expire;
		// source changes and bounded eviction still invalidate old navigation keys.
		cache.pages = map[string]stalker.CatalogPage{}
		cache.categories = map[string][]stalker.CatalogCategory{}
		cache.posters = map[string]Artwork{}
		cache.pageBytes = 0
		cache.categoryBytes = 0
		cache.posterBytes = 0
		cache.created = time.Now()
	}
	if s.Type != "stalker" {
		return cache, nil, nil
	}
	return cache, a.stalkerClient(s, a.st.Settings), nil
}
func (a *App) catalogCurrent(id string, c *catalogStore) bool {
	s := a.source(id)
	return s != nil && catalogFingerprint(*s) == c.fingerprint && a.catalogs[id] == c
}
func catalogSection(s string) bool { return s == "itv" || s == "vod" || s == "series" }

// Refresh Live TV from the provider outside the application mutex.
func (a *App) refreshCatalogLive(ctx context.Context, id string) error {
	a.mu.Lock()
	src := a.source(id)
	stalkerSource := src != nil && src.Type == "stalker"
	a.mu.Unlock()
	if src == nil {
		return ErrNotFound
	}
	if err := a.RefreshSource(ctx, id); err != nil {
		if stalkerSource {
			return errors.New("не удалось обновить телеканалы портала")
		}
		return errors.New("не удалось обновить плейлист — показан сохранённый список")
	}
	return nil
}
func cacheCategories(cache *catalogStore, section string, cats []stalker.CatalogCategory) error {
	size := 0
	for _, c := range cats {
		size += len(c.ID) + len(c.Name) + 64
	}
	if size > 1<<20 {
		return errors.New("список категорий слишком большой")
	}
	if cache.categoryBytes+size > 1<<20 {
		cache.categories = map[string][]stalker.CatalogCategory{}
		cache.categoryBytes = 0
	}
	if cache.itemBytes+cache.pageBytes+cache.categoryBytes+size > catalogMaxBytes {
		cache.items = map[string]catalogRecord{}
		cache.pages = map[string]stalker.CatalogPage{}
		cache.itemBytes = 0
		cache.pageBytes = 0
	}
	cache.categories[section] = cats
	cache.categoryBytes += size
	return nil
}
func (a *App) CatalogCategories(ctx context.Context, id, section string, refresh bool) ([]stalker.CatalogCategory, error) {
	if !catalogSection(section) {
		return nil, ErrCatalogInput
	}
	if refresh && section == "itv" {
		if err := a.refreshCatalogLive(ctx, id); err != nil {
			return nil, err
		}
	}
	a.mu.Lock()
	cache, client, err := a.catalogLocked(id)
	if err != nil {
		a.mu.Unlock()
		return nil, err
	}
	if refresh {
		cache = newCatalogStore(cache.fingerprint)
		a.catalogs[id] = cache
	}
	if section != "itv" && client == nil {
		a.mu.Unlock()
		return nil, stalker.ErrCatalogUnsupported
	}
	if cats, ok := cache.categories[section]; ok {
		a.mu.Unlock()
		return cats, nil
	}
	if section == "itv" {
		cats := []stalker.CatalogCategory{{ID: "", Name: "Все каналы"}}
		groups := map[string]bool{}
		for _, ch := range a.channels[id] {
			if ch.Group != "" {
				groups[ch.Group] = true
			}
		}
		names := []string{}
		for name := range groups {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			cats = append(cats, stalker.CatalogCategory{ID: name, Name: name})
		}
		err := cacheCategories(cache, section, cats)
		a.mu.Unlock()
		return cats, err
	}
	a.mu.Unlock()
	cats, err := client.CatalogCategories(ctx, section)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.catalogCurrent(id, cache) {
		return nil, ErrCatalogExpired
	}
	err = cacheCategories(cache, section, cats)
	return cats, err
}
func (a *App) CatalogItems(ctx context.Context, id, section, category, parent string, page int, refresh bool) (stalker.CatalogPage, error) {
	return a.catalogPage(ctx, id, pageRequest{section: section, category: category, parent: parent, page: page, refresh: refresh})
}

// CatalogItemsSorted lists a category in the viewer's order (see
// stalker.CatalogSorts; "" is newest first). Live TV keeps channel numbers.
func (a *App) CatalogItemsSorted(ctx context.Context, id, section, category, parent, sort string, page int, refresh bool) (stalker.CatalogPage, error) {
	if sort != "" && !stalker.CatalogSorts[sort] {
		return stalker.CatalogPage{Items: []stalker.CatalogItem{}, Page: page}, ErrCatalogInput
	}
	return a.catalogPage(ctx, id, pageRequest{section: section, category: category, parent: parent, sort: sort, page: page, refresh: refresh})
}

type pageRequest struct {
	section, category, parent, search, sort string
	page                                    int
	refresh                                 bool
}

// catalogPage serves one page of a category, a season, or a provider-side
// search. Every entry it returns gets a navigation key in the cache.
func (a *App) catalogPage(ctx context.Context, id string, r pageRequest) (stalker.CatalogPage, error) {
	section, category, parent, search, page, refresh := r.section, r.category, r.parent, r.search, r.page, r.refresh
	if parent != "" || section == "itv" {
		r.sort = "" // one natural order inside a series, and channel numbers
	}
	empty := stalker.CatalogPage{Items: []stalker.CatalogItem{}, Page: page}
	if !catalogSection(section) || page < 1 || page > 10000 {
		return empty, ErrCatalogInput
	}
	if refresh && section == "itv" {
		if err := a.refreshCatalogLive(ctx, id); err != nil {
			return empty, err
		}
	}
	a.mu.Lock()
	cache, client, err := a.catalogLocked(id)
	if err != nil {
		a.mu.Unlock()
		return empty, err
	}
	if section != "itv" && client == nil {
		a.mu.Unlock()
		return empty, stalker.ErrCatalogUnsupported
	}
	var par *stalker.CatalogItem
	if parent != "" {
		rec, ok := a.catalogRecordLocked(id, cache, parent)
		if !ok || rec.item.Type != section {
			a.mu.Unlock()
			return empty, ErrCatalogExpired
		}
		cp := rec.item
		par = &cp
	}
	if refresh {
		fresh := newCatalogStore(cache.fingerprint)
		// Retain only the navigation ancestry; replace all fetched pages and artwork.
		for key := parent; key != ""; {
			rec, ok := cache.items[key]
			if !ok {
				break
			}
			fresh.items[key] = rec
			fresh.itemBytes += rec.item.CacheSize()
			key = rec.parent
		}
		cache = fresh
		a.catalogs[id] = cache
	}
	query := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d", section, category, parent, search, r.sort, page)
	if p, ok := cache.pages[query]; ok {
		a.mu.Unlock()
		return p, nil
	}
	var result stalker.CatalogPage
	if section == "itv" {
		items := []stalker.CatalogItem{}
		offset := (page - 1) * catalogPageSize
		count := 0
		words := searchWords(search)
		for _, ch := range a.channels[id] {
			if category != "" && category != ch.Group || !matchesWords(ch.Name, words) {
				continue
			}
			if count >= offset && len(items) < catalogPageSize {
				items = append(items, channelItem(id, ch, count+1))
			}
			count++
		}
		result = stalker.CatalogPage{Items: items, Page: page, HasMore: count > offset+len(items), PageSize: catalogPageSize, Total: count}
	}
	a.mu.Unlock()
	if section != "itv" {
		result, err = client.CatalogPage(ctx, stalker.CatalogQuery{Type: section, Category: category, Page: page, Parent: par, Search: search, Sort: r.sort})
		if err != nil {
			return empty, err
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.catalogCurrent(id, cache) {
		return empty, ErrCatalogExpired
	}
	if result.Items == nil {
		result.Items = []stalker.CatalogItem{}
	}
	for i := range result.Items {
		item := &result.Items[i]
		item.Key = catalogKey(id, section, parent, *item)
		item.CatalogID = rowCatalogID(id, section, par, *item)
	}
	if parent == "" && section != "itv" {
		a.refreshLibraryLocked(id, result.Items)
	}
	// Portals sometimes ignore p; never offer an endless loop of the same page.
	if page > 1 {
		for oldQuery, p := range cache.pages {
			prefix := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00", section, category, parent, search, r.sort)
			if strings.HasPrefix(oldQuery, prefix) && sameCatalogPage(p, result) {
				result.Items = []stalker.CatalogItem{}
				result.HasMore = false
				break
			}
		}
	}
	pageBytes := 0
	for _, item := range result.Items {
		pageBytes += item.CacheSize()
	}
	// Reject a single oversized page instead of retaining it beyond the budget.
	ancestry := map[string]catalogRecord{}
	ancestryBytes := 0
	for key := parent; key != ""; {
		rec, ok := cache.items[key]
		if !ok {
			break
		}
		ancestry[key] = rec
		ancestryBytes += rec.item.CacheSize()
		key = rec.parent
	}
	if len(result.Items)+len(ancestry) > catalogMaxItems || 2*pageBytes+ancestryBytes > catalogMaxBytes {
		return empty, errors.New("страница каталога слишком большая для ресивера")
	}
	if len(cache.pages) >= 32 {
		cache.pages = map[string]stalker.CatalogPage{}
		cache.pageBytes = 0
	}
	for _, item := range result.Items {
		if prev, ok := cache.items[item.Key]; ok {
			cache.itemBytes -= prev.item.CacheSize()
		}
		cache.itemBytes += item.CacheSize()
		cache.items[item.Key] = catalogRecord{item: item, parent: parent}
	}
	cache.pages[query] = result
	cache.pageBytes += pageBytes
	if len(cache.items) > catalogMaxItems || cache.itemBytes+cache.pageBytes+cache.categoryBytes > catalogMaxBytes {
		for _, item := range result.Items {
			ancestry[item.Key] = cache.items[item.Key]
		}
		cache.items = ancestry
		cache.pages = map[string]stalker.CatalogPage{query: result}
		cache.categories = map[string][]stalker.CatalogCategory{}
		cache.categoryBytes = 0
		cache.pageBytes = pageBytes
		cache.itemBytes = ancestryBytes + pageBytes
	}
	return result, nil
}
func sameCatalogPage(a, b stalker.CatalogPage) bool {
	if len(a.Items) == 0 || len(a.Items) != len(b.Items) {
		return false
	}
	for i := range a.Items {
		if a.Items[i].Key != b.Items[i].Key {
			return false
		}
	}
	return true
}
func (a *App) CatalogPlay(ctx context.Context, id, key string) (CatalogPlayback, error) {
	return a.CatalogPlayEntry(ctx, id, key, "")
}

// CatalogPlayEntry starts a row by its navigation key and, once the key is no
// longer known (the list was refreshed meanwhile), by its persistent id. The
// key comes first: it names exactly one row, while some portals number
// several episodes alike. A started row becomes the newest of the recents.
func (a *App) CatalogPlayEntry(ctx context.Context, id, key, entry string) (CatalogPlayback, error) {
	a.mu.Lock()
	cache, client, err := a.catalogLocked(id)
	if err != nil {
		a.mu.Unlock()
		return CatalogPlayback{}, err
	}
	rec, ok := a.catalogRecordLocked(id, cache, key)
	if !ok && entry != "" {
		_, rec, ok = a.catalogRecordByIDLocked(id, cache, entry)
	}
	if ok && client == nil {
		// Playlist channels play their own address, the same service the
		// channel lists write into bouquets.
		ch := a.channel(id, rec.item.ID)
		serviceType := a.st.Settings.ServiceType
		if rec.item.Kind == "channel" && ch != nil {
			a.noteRecentLocked(id, cache, rec)
		}
		a.mu.Unlock()
		if rec.item.Kind != "channel" {
			return CatalogPlayback{}, ErrCatalogInput
		}
		if ch == nil {
			return CatalogPlayback{}, ErrCatalogExpired
		}
		return CatalogPlayback{URL: ch.URL, Name: ch.Name, ServiceType: serviceType, Headers: map[string]string{}}, nil
	}
	a.mu.Unlock()
	if !ok {
		return CatalogPlayback{}, ErrCatalogExpired
	}
	item := rec.item
	if item.Kind != "movie" && item.Kind != "episode" && item.Kind != "channel" {
		return CatalogPlayback{}, ErrCatalogInput
	}
	var u string
	if item.Kind == "channel" {
		u, err = client.Resolve(ctx, stalker.Channel{ID: item.ID, Cmd: item.Cmd, NeedsLink: item.NeedsLink})
	} else {
		u, err = client.ResolveCatalog(ctx, item)
	}
	if err != nil {
		return CatalogPlayback{}, errors.New("не удалось получить ссылку на видео — проверьте подключение и подписку")
	}
	a.mu.Lock()
	valid := a.catalogCurrent(id, cache)
	if valid {
		a.noteRecentLocked(id, cache, rec)
	}
	a.mu.Unlock()
	if !valid {
		return CatalogPlayback{}, ErrCatalogExpired
	}
	return CatalogPlayback{URL: u, Name: item.Name, ServiceType: 4097, Headers: client.PlaybackHeaders(u)}, nil
}
