package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/epg"
	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
)

// Programme guide of the TV catalog. The service answers it, not Enigma2's
// EPG cache: channels outside the lists have nothing there, and EPGImport
// runs once a day. Where programmes come from, best first:
//   - channels in lists: an index of the epg.xml built for EPGImport
//     (titles and times in memory, descriptions read back from the file);
//   - Xtream and Stalker channels: the provider's short guide, asked when
//     the TV shows a channel and kept for 15 minutes;
//   - other M3U channels: a name-and-time index for 36 hours, made by the
//     same nightly pass over the playlist's XMLTV and kept on disk.

const (
	// The lists index keeps the past back to the build (epg.xml starts two
	// hours before it): today's earlier programmes for the day view, and for
	// the archive the hours since the build, which kept the past up to then,
	// even after a restart at noon or days off. The bound only matters for a
	// file built more than an archive ago.
	guideBehind        = (playlist.MaxCatchupDays + 1) * 24 * time.Hour
	guideAhead         = 36 * time.Hour // and of the future
	guideRefresh       = 12 * time.Hour // re-read epg.xml when less is left ahead
	guideRetry         = time.Hour      // between attempts to read it
	playlistGuideAhead = 34 * time.Hour // with 2 hours of the past: 36 hours
	shortGuideTTL      = 15 * time.Minute
	shortGuideSize     = 20      // programmes asked from Xtream and Stalker
	shortGuideBytes    = 1 << 20 // short guides kept in memory
	guideNowKeys       = 60      // channels per /now request
)

var errGuideProvider = errors.New("источник не отдал программу передач")

type guideIndex struct {
	mu           sync.Mutex
	lists        *epg.Guide
	listKeys     map[string]string // "source/channel" -> guide channel in epg.xml
	listsRunning int               // reads of epg.xml under way
	listsTried   time.Time
	listsGen     uint64 // bumped when epg.xml is replaced: older reads are dropped
	playlists    *epg.Guide
	playlistRefs map[string]epg.GuideRef
	short        map[string]*shortGuide
	shortBytes   int
	archive      *epg.Guide // past days of channels with an archive (guide_archive.go)
}

type shortGuide struct {
	fetched time.Time
	progs   []epg.Programme
	size    int
}

// GuideEntry is a programme as the TV gets it; descriptions only for now
// and next.
type GuideEntry struct {
	Title string `json:"title"`
	Start int64  `json:"start"`
	Stop  int64  `json:"stop"`
	Desc  string `json:"desc,omitempty"`
	Rec   bool   `json:"rec,omitempty"` // the provider keeps it: it plays from the archive
}

func guideEntry(p epg.Programme) GuideEntry {
	return GuideEntry{Title: p.Title, Start: p.Start.Unix(), Stop: p.Stop.Unix(), Desc: p.Desc}
}

// rowEntry is a programme for a list row: never a description.
func rowEntry(p epg.Programme) GuideEntry {
	return GuideEntry{Title: p.Title, Start: p.Start.Unix(), Stop: p.Stop.Unix()}
}

func (a *App) now() time.Time {
	if a.clock != nil {
		return a.clock()
	}
	return time.Now()
}

func (a *App) guideMapPath() string      { return filepath.Join(a.cfg.DataDir, "epg-map.json") }
func (a *App) playlistGuidePath() string { return filepath.Join(a.cfg.DataDir, "guide-playlists.gob") }

// ---------------------------------------------------------------- building

// LoadGuides reads the guides kept on disk: the playlists index and epg.xml.
func (a *App) LoadGuides() {
	if data, err := os.ReadFile(a.playlistGuidePath()); err == nil {
		g, refs, err := epg.DecodeGuide(bytes.NewReader(data))
		switch {
		case err != nil:
			a.log.Printf("программа передач плейлистов не прочитана: %v", err)
		case g.To.After(a.now().Add(-playlist.MaxCatchupDays * archiveDay)):
			// After days off it holds only the past: the pass that runs soon
			// after such a start keeps that in the archive.
			a.guide.mu.Lock()
			a.guide.playlists, a.guide.playlistRefs = g, refs
			a.guide.mu.Unlock()
		}
	}
	a.loadArchiveGuide()
	if gen, ok := a.startListsLoad(false); ok {
		a.loadListsGuide(gen)
	}
}

// startListsLoad claims the next read of epg.xml. A replaced file (fresh)
// starts a new generation, so a read of the old one still running is
// dropped when it ends; otherwise one read at a time.
func (a *App) startListsLoad(fresh bool) (uint64, bool) {
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	if fresh {
		gi.listsGen++
	} else if gi.listsRunning > 0 {
		return 0, false
	}
	gi.listsRunning++
	return gi.listsGen, true
}

// guideReplacing is called before epg.xml is replaced: reads of the old
// file that are still running must not land after the new one.
func (a *App) guideReplacing() {
	a.guide.mu.Lock()
	a.guide.listsGen++
	a.guide.mu.Unlock()
}

// loadListsGuide (re)reads epg.xml into the lists index.
func (a *App) loadListsGuide(gen uint64) {
	now := a.now()
	keys, err := a.readGuideMap()
	var g *epg.Guide
	if err == nil {
		g, err = epg.ReadGuide(a.EPGPath(), now.Add(-guideBehind), now.Add(guideAhead), nil)
	}
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	gi.listsRunning--
	if gen != gi.listsGen {
		return // epg.xml was replaced meanwhile; its own read follows
	}
	gi.listsTried = now
	if err != nil {
		if !os.IsNotExist(err) {
			a.log.Printf("программа передач для каталога: %v", err)
		}
		return
	}
	gi.lists, gi.listKeys = g, keys
}

// maybeReloadGuide re-reads epg.xml in the background when the index is
// missing or its window is running out (no new build for a day).
func (a *App) maybeReloadGuide() {
	gi := &a.guide
	gi.mu.Lock()
	now := a.now()
	stale := gi.lists == nil || now.After(gi.lists.To.Add(-guideRefresh))
	if !stale || gi.listsRunning > 0 || now.Sub(gi.listsTried) < guideRetry {
		gi.mu.Unlock()
		return
	}
	gi.listsRunning++
	gi.listsTried = now
	gen := gi.listsGen
	gi.mu.Unlock()
	a.bgWork.Add(1)
	go func() {
		defer a.bgWork.Done()
		a.loadListsGuide(gen)
	}()
}

// guideBuilt takes the playlists index of a finished pass, keeps it on
// disk (once a night is not often for the flash) and re-reads epg.xml.
func (a *App) guideBuilt(g *epg.Guide, refs map[string]epg.GuideRef) {
	var buf bytes.Buffer
	if err := g.Encode(&buf, refs); err == nil {
		if err := enigma.WriteFileAtomic(a.playlistGuidePath(), buf.Bytes()); err != nil {
			a.log.Printf("программа передач плейлистов не сохранена: %v", err)
		}
	}
	gi := &a.guide
	gi.mu.Lock()
	gi.playlists, gi.playlistRefs = g, refs
	gi.mu.Unlock()
	if gen, ok := a.startListsLoad(true); ok {
		a.loadListsGuide(gen)
	}
}

// keepPlaylistProgrammes carries the last pass's programmes over for
// playlist channels whose guide failed this time (a provider down for a
// night, a pass cut short by a restart), so their rows do not go blank.
func (a *App) keepPlaylistProgrammes(b *epg.GuideBuilder, refs map[string]epg.GuideRef, failed []epg.Want) {
	gi := &a.guide
	gi.mu.Lock()
	old, oldRefs := gi.playlists, gi.playlistRefs
	gi.mu.Unlock()
	if old == nil {
		return
	}
	copied := map[string]string{}
	for _, w := range failed {
		ref, ok := oldRefs[w.Ref]
		if _, done := refs[w.Ref]; done || !ok {
			continue
		}
		ch, ok := copied[ref.Channel]
		if !ok {
			ch = ref.Channel
			if !strings.HasPrefix(ch, "kept.") {
				ch = "kept." + ch // job numbers of this pass may mean other guides
			}
			copied[ref.Channel] = ch
			for _, p := range old.Between(ref.Channel, old.From, old.To) {
				b.Add(ch, p.Start.Unix(), p.Stop.Unix(), p.Title, 0)
			}
		}
		refs[w.Ref] = epg.GuideRef{Channel: ch, Shift: ref.Shift}
	}
}

func (a *App) writeGuideMap(byGuide map[string][]string) error {
	data, err := json.Marshal(byGuide)
	if err != nil {
		return err
	}
	return enigma.WriteFileAtomic(a.guideMapPath(), data)
}

var channelLine = regexp.MustCompile(`<channel id="([^"]+)">([^<]+)</channel>`)

// readGuideMap answers "source/channel" -> guide channel. Builds before the
// map existed are understood through EPGImport's channel file: its
// references are hashes of the same keys.
func (a *App) readGuideMap() (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(a.guideMapPath())
	if err == nil {
		var byGuide map[string][]string
		if err := json.Unmarshal(data, &byGuide); err != nil {
			return nil, err
		}
		for id, keys := range byGuide {
			for _, k := range keys {
				out[k] = id
			}
		}
		return out, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	f, err := os.Open(filepath.Join(a.cfg.EPGImportDir, "dreaminator.channels.xml"))
	if err != nil {
		return out, nil
	}
	defer f.Close()
	a.mu.Lock()
	byRef := map[string][]string{}
	for _, b := range a.st.Bouquets {
		for _, r := range b.Channels {
			byRef[enigma.EPGRef(r.key())] = append(byRef[enigma.EPGRef(r.key())], r.key())
		}
	}
	a.mu.Unlock()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := channelLine.FindStringSubmatch(sc.Text()); m != nil {
			for _, k := range byRef[m[2]] {
				out[k] = m[1]
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- lookups

// nowOn is the programme on air on a channel; callers hold gi.mu.
func (gi *guideIndex) nowOn(key string, t time.Time) (epg.Programme, bool) {
	if ch, ok := gi.listKeys[key]; ok && gi.lists.Has(ch) {
		return gi.lists.At(ch, t)
	}
	if sg := gi.short[key]; sg != nil && t.Sub(sg.fetched) < shortGuideTTL {
		for _, p := range sg.progs {
			if !p.Start.After(t) && p.Stop.After(t) {
				return p, true
			}
		}
		return epg.Programme{}, false
	}
	if ref, ok := gi.playlistRefs[key]; ok {
		if p, ok := gi.playlists.At(ref.Channel, t.Add(-ref.Shift)); ok {
			p.Start, p.Stop = p.Start.Add(ref.Shift), p.Stop.Add(ref.Shift)
			return p, true
		}
	}
	return epg.Programme{}, false
}

// CatalogCards is Cards with what is on now on each channel, when known.
// Only the title and times: rows stay light for the receiver's Python.
func (a *App) CatalogCards(id string, p stalker.CatalogPage) CatalogCards {
	cards := Cards(p)
	if len(p.Items) == 0 || p.Items[0].Kind != "channel" {
		return cards
	}
	a.mu.Lock()
	for i, item := range p.Items {
		if ch := a.channel(id, item.ID); item.Kind == "channel" && ch != nil && ch.Catchup != nil {
			cards.Items[i].Archive = ch.Catchup.Days
		}
	}
	a.mu.Unlock()
	a.maybeReloadGuide()
	now := a.now()
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	for i, item := range p.Items {
		if item.Kind != "channel" {
			continue
		}
		if pr, ok := gi.nowOn(id+"/"+item.ID, now); ok {
			e := rowEntry(pr)
			cards.Items[i].Now = &e
		}
	}
	return cards
}

// CatalogNow refreshes rows on screen: what is on now on the channels
// behind these navigation keys. Unknown keys are left out.
func (a *App) CatalogNow(id string, keys []string) (map[string]GuideEntry, error) {
	if len(keys) > guideNowKeys {
		return nil, ErrCatalogInput
	}
	a.mu.Lock()
	cache, _, err := a.catalogLocked(id)
	channels := map[string]string{}
	if err == nil {
		for _, k := range keys {
			if rec, ok := cache.items[k]; ok && rec.item.Kind == "channel" {
				channels[k] = rec.item.ID
			}
		}
	}
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	a.maybeReloadGuide()
	now := a.now()
	out := map[string]GuideEntry{}
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	for k, ch := range channels {
		if p, ok := gi.nowOn(id+"/"+ch, now); ok {
			out[k] = rowEntry(p)
		}
	}
	return out, nil
}

// channelGuide is everything known about one channel's programmes.
type channelGuide struct {
	progs    []epg.Programme // sorted
	describe func(epg.Programme) string
	origin   string            // lists | provider | playlist | archive
	catchup  *playlist.Catchup // the channel's archive, nil without one
}

// channelGuide is liveGuide with the kept past of a channel with an archive.
func (a *App) channelGuide(ctx context.Context, sourceID, channelID string) (channelGuide, error) {
	cg, err := a.liveGuide(ctx, sourceID, channelID)
	if err != nil {
		return cg, err
	}
	return a.withArchive(cg, sourceID, channelID), nil
}

func (a *App) liveGuide(ctx context.Context, sourceID, channelID string) (channelGuide, error) {
	key := sourceID + "/" + channelID
	a.maybeReloadGuide()
	gi := &a.guide
	gi.mu.Lock()
	if ch, ok := gi.listKeys[key]; ok && gi.lists.Has(ch) {
		g := gi.lists
		gi.mu.Unlock()
		return channelGuide{progs: g.Between(ch, g.From, g.To), describe: g.Describe, origin: "lists"}, nil
	}
	if sg := gi.short[key]; sg != nil && a.now().Sub(sg.fetched) < shortGuideTTL {
		progs := sg.progs
		gi.mu.Unlock()
		return channelGuide{progs: progs, origin: "provider"}, nil
	}
	ref, inPlaylist := gi.playlistRefs[key]
	playlists := gi.playlists
	gi.mu.Unlock()

	a.mu.Lock()
	src, ch := a.source(sourceID), a.channel(sourceID, channelID)
	if src == nil || ch == nil {
		a.mu.Unlock()
		return channelGuide{}, ErrCatalogExpired
	}
	srcType := src.Type
	var fetchShort func(context.Context) ([]epg.Programme, error)
	switch srcType {
	case "xtream":
		x := playlist.Xtream{Server: src.Server, Username: src.Username, Password: src.Password}
		o := a.fetchOptions(a.st.Settings, 15*time.Second)
		fetchShort = func(ctx context.Context) ([]epg.Programme, error) {
			list, err := x.ShortEPG(ctx, o, channelID, shortGuideSize)
			out := make([]epg.Programme, 0, len(list))
			for _, p := range list {
				out = append(out, epg.Programme{Start: p.Start, Stop: p.Stop, Title: p.Title, Desc: p.Desc})
			}
			return out, err
		}
	case "stalker":
		c := a.stalkerClient(src, a.st.Settings)
		fetchShort = func(ctx context.Context) ([]epg.Programme, error) {
			list, err := c.ShortGuide(ctx, channelID, shortGuideSize)
			out := make([]epg.Programme, 0, len(list))
			for _, p := range list {
				out = append(out, epg.Programme{Start: p.Start, Stop: p.Stop, Title: p.Title, Desc: p.Desc})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
			return out, err
		}
	}
	a.mu.Unlock()

	if fetchShort != nil {
		progs, err := fetchShort(ctx)
		if err != nil {
			a.log.Printf("программа передач канала %s: %v", key, err)
			return channelGuide{}, errGuideProvider
		}
		a.keepShortGuide(key, progs)
		return channelGuide{progs: progs, origin: "provider"}, nil
	}
	if inPlaylist {
		progs := playlists.Between(ref.Channel, playlists.From, playlists.To)
		for i := range progs {
			progs[i].Start, progs[i].Stop = progs[i].Start.Add(ref.Shift), progs[i].Stop.Add(ref.Shift)
		}
		return channelGuide{progs: progs, origin: "playlist"}, nil
	}
	return channelGuide{}, nil
}

// keepShortGuide caches a provider's answer (an empty one too: asking again
// within 15 minutes would not change it), within a memory budget.
func (a *App) keepShortGuide(key string, progs []epg.Programme) {
	size := len(key) + 64
	for _, p := range progs {
		size += len(p.Title) + len(p.Desc) + 64
	}
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	if gi.short == nil {
		gi.short = map[string]*shortGuide{}
	}
	if old := gi.short[key]; old != nil {
		gi.shortBytes -= old.size
	}
	gi.short[key] = &shortGuide{fetched: a.now(), progs: progs, size: size}
	gi.shortBytes += size
	for gi.shortBytes > shortGuideBytes && len(gi.short) > 1 {
		oldest := ""
		for k, sg := range gi.short {
			if k != key && (oldest == "" || sg.fetched.Before(gi.short[oldest].fetched)) {
				oldest = k
			}
		}
		gi.shortBytes -= gi.short[oldest].size
		delete(gi.short, oldest)
	}
}

// CatalogGuide is a channel's programme for the TV: now and next with their
// descriptions, the days that have programmes, and with Day set the
// programmes of that day (titles and times).
type CatalogGuide struct {
	Now        *GuideEntry  `json:"now,omitempty"`
	Next       *GuideEntry  `json:"next,omitempty"`
	Day        *int         `json:"day,omitempty"`
	Date       string       `json:"date,omitempty"`
	Programmes []GuideEntry `json:"programmes,omitempty"`
	Days       []int        `json:"days"`
	Origin     string       `json:"origin,omitempty"`
	Archive    int          `json:"archive,omitempty"` // days the provider keeps
}

// CatalogGuide answers /epg. Days are counted in zone: the TV sends its
// own offset, so "today" is the TV's today whatever zone data Go has.
func (a *App) CatalogGuide(ctx context.Context, id, key string, day *int, zone *time.Location) (CatalogGuide, error) {
	empty := CatalogGuide{Days: []int{}}
	if day != nil && (*day < -playlist.MaxCatchupDays || *day > 14) {
		return empty, ErrCatalogInput
	}
	item, err := a.catalogEntry(id, key)
	if err != nil {
		return empty, err
	}
	if item.Kind != "channel" {
		return empty, ErrCatalogInput
	}
	cg, err := a.channelGuide(ctx, id, item.ID)
	if err != nil {
		return empty, err
	}
	if zone == nil {
		zone = time.Local
	}
	now := a.now().In(zone)
	out := CatalogGuide{Days: []int{}, Origin: cg.origin}
	if cg.catchup != nil {
		out.Archive = cg.catchup.Days
	}
	describe := func(p epg.Programme) *GuideEntry {
		if p.Desc == "" && cg.describe != nil {
			p.Desc = cg.describe(p)
		}
		e := guideEntry(p)
		e.Rec = recorded(cg.catchup, p, now)
		return &e
	}
	for i, p := range cg.progs {
		if p.Stop.After(now) && !p.Start.After(now) {
			out.Now = describe(p)
			if i+1 < len(cg.progs) {
				out.Next = describe(cg.progs[i+1])
			}
			break
		}
		if p.Start.After(now) {
			out.Next = describe(p)
			break
		}
	}
	seen := map[int]bool{}
	for _, p := range cg.progs {
		if d := dayOffset(now, p.Start.In(zone)); !seen[d] {
			seen[d] = true
			out.Days = append(out.Days, d)
		}
	}
	sort.Ints(out.Days)
	if day != nil {
		y, m, d := now.Date()
		start := time.Date(y, m, d+*day, 0, 0, 0, 0, zone)
		end := time.Date(y, m, d+*day+1, 0, 0, 0, 0, zone)
		out.Day, out.Date = day, start.Format("2006-01-02")
		out.Programmes = []GuideEntry{}
		for _, p := range cg.progs {
			if p.Stop.After(start) && p.Start.Before(end) {
				e := rowEntry(p)
				e.Rec = recorded(cg.catchup, p, now)
				out.Programmes = append(out.Programmes, e)
			}
		}
	}
	return out, nil
}

// dayOffset counts calendar days from now to t (both in the TV's zone).
func dayOffset(now, t time.Time) int {
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	a := time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	b := time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// GuideStats is the guides' share of the service's memory, for /api/debug.
func (a *App) GuideStats() map[string]any {
	gi := &a.guide
	gi.mu.Lock()
	defer gi.mu.Unlock()
	return map[string]any{
		"listsChannels":      gi.lists.Channels(),
		"listsProgrammes":    gi.lists.Programmes(),
		"listsKB":            gi.lists.Bytes() / 1024,
		"playlistChannels":   len(gi.playlistRefs),
		"playlistProgrammes": gi.playlists.Programmes(),
		"playlistKB":         gi.playlists.Bytes() / 1024,
		"shortGuides":        len(gi.short),
		"shortKB":            gi.shortBytes / 1024,
		"archiveChannels":    gi.archive.Channels(),
		"archiveProgrammes":  gi.archive.Programmes(),
		"archiveKB":          gi.archive.Bytes() / 1024,
	}
}
