package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/epg"
	"dreaminator/internal/playlist"
)

// Archive (catch-up) of the TV catalog: past programmes of playlist channels
// whose provider keeps recordings (catchup.go says how to ask for one).
//
// The guide indexes keep two or three hours of the past, and the providers'
// guides little more: epg.one, Edem's guide, holds only yesterday and today.
// So the past is kept by the service itself. Every nightly pass reads the
// programmes of channels with an archive back to the deepest archive and
// adds them to what earlier passes kept; channels the pass found nothing
// for keep theirs. guide-archive.gob holds titles and times, no
// descriptions (the TV shows those for now and next only), and is written
// once a night.

const archiveDay = 24 * time.Hour

var ErrCatalogArchive = errors.New("этой передачи нет в архиве")

func (a *App) archiveGuidePath() string { return filepath.Join(a.cfg.DataDir, "guide-archive.gob") }

// newArchiveIndex collects the past of channels with an archive during a
// nightly pass, whatever guide and list they are in. The window is in the
// guide's own times: a channel with tvg-shift="-4" aired at 01:00 what the
// guide lists at 05:00, so it reaches as far as the largest shift.
func newArchiveIndex(now time.Time, jobs []epgJob) *epg.Index {
	var shift time.Duration
	for _, j := range jobs {
		for _, w := range j.Archive {
			shift = max(shift, w.Shift, -w.Shift)
		}
	}
	return &epg.Index{Builder: epg.NewGuideBuilder(now.Add(-playlist.MaxCatchupDays*archiveDay-shift), now.Add(shift)), Refs: map[string]epg.GuideRef{}}
}

// loadArchiveGuide reads what earlier passes kept.
func (a *App) loadArchiveGuide() {
	data, err := os.ReadFile(a.archiveGuidePath())
	if err != nil {
		if !os.IsNotExist(err) {
			a.log.Printf("архив программы не прочитан: %v", err)
		}
		return
	}
	g, _, err := epg.DecodeGuide(bytes.NewReader(data))
	if err != nil {
		a.log.Printf("архив программы не прочитан: %v", err)
		return
	}
	a.guide.mu.Lock()
	a.guide.archive = g
	a.guide.mu.Unlock()
}

// archiveDepths are the channels that have an archive now, by
// "source/channel", with its depth.
func (a *App) archiveDepths() map[string]time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]time.Duration{}
	for _, src := range a.st.Sources {
		for _, ch := range a.channels[src.ID] {
			if ch.Catchup != nil {
				out[src.ID+"/"+ch.ID] = time.Duration(ch.Catchup.Days) * archiveDay
			}
		}
	}
	return out
}

// archiveBuilt merges a pass's past programmes into the kept archive; it
// runs before the pass replaces the live indexes. The archive guide is kept
// by "source/channel" with tvg-shift applied, so it does not depend on which
// guide or job number found a channel. For each channel the pass's
// programmes win; the live indexes of the previous pass (the hours since
// it, when this night's guide failed or was cut off), then the kept archive
// fill the time around them; nothing goes deeper than the channel's archive.
func (a *App) archiveBuilt(idx *epg.Index, now time.Time) {
	fresh := idx.Builder.Build()
	depths := a.archiveDepths()
	gi := &a.guide
	gi.mu.Lock()
	old, lists, listKeys, playlists, playlistRefs := gi.archive, gi.lists, gi.listKeys, gi.playlists, gi.playlistRefs
	gi.mu.Unlock()

	b := epg.NewGuideBuilder(now.Add(-playlist.MaxCatchupDays*archiveDay), now)
	for key, depth := range depths {
		from := now.Add(-depth)
		// What the layers added so far covers [lo, hi); a lower layer fills
		// only before and after it: a guide cut off halfway leaves the rest
		// to the live indexes and the kept archive.
		var lo, hi time.Time
		layer := func(progs []epg.Programme, shift time.Duration) {
			var first, last time.Time
			for _, p := range progs {
				start, stop := p.Start.Add(shift), p.Stop.Add(shift)
				if !lo.IsZero() && stop.After(lo) && start.Before(hi) {
					continue
				}
				if b.Add(key, start.Unix(), stop.Unix(), p.Title, 0) {
					if first.IsZero() || start.Before(first) {
						first = start
					}
					if stop.After(last) {
						last = stop
					}
				}
			}
			if first.IsZero() {
				return
			}
			if lo.IsZero() || first.Before(lo) {
				lo = first
			}
			if last.After(hi) {
				hi = last
			}
		}
		if ref, ok := idx.Refs[key]; ok {
			layer(fresh.Between(ref.Channel, from.Add(-ref.Shift), now.Add(-ref.Shift)), ref.Shift)
		}
		if ch, ok := listKeys[key]; ok && lists.Has(ch) {
			layer(lists.Between(ch, from, now), 0)
		} else if ref, ok := playlistRefs[key]; ok {
			layer(playlists.Between(ref.Channel, from.Add(-ref.Shift), now.Add(-ref.Shift)), ref.Shift)
		}
		layer(old.Between(key, from, now), 0)
	}
	g := b.Build()
	var buf bytes.Buffer
	if err := g.Encode(&buf, nil); err == nil {
		if err := enigma.WriteFileAtomic(a.archiveGuidePath(), buf.Bytes()); err != nil {
			a.log.Printf("архив программы не сохранён: %v", err)
		}
	}
	gi.mu.Lock()
	gi.archive = g
	gi.mu.Unlock()
}

// withArchive puts a channel's kept past around what the live guides know.
func (a *App) withArchive(cg channelGuide, sourceID, channelID string) channelGuide {
	a.mu.Lock()
	var c *playlist.Catchup
	if ch := a.channel(sourceID, channelID); ch != nil && ch.Catchup != nil {
		cc := *ch.Catchup
		c = &cc
	}
	a.mu.Unlock()
	if c == nil {
		return cg
	}
	cg.catchup = c
	now := a.now()
	gi := &a.guide
	gi.mu.Lock()
	past := gi.archive.Between(sourceID+"/"+channelID, now.Add(-time.Duration(c.Days)*archiveDay), now)
	gi.mu.Unlock()
	// The live guide wins where it has programmes; the archive fills before
	// it and, when the live guide stops short of now, after it.
	var before, after []epg.Programme
	for _, p := range past {
		switch {
		case len(cg.progs) == 0 || !p.Stop.After(cg.progs[0].Start):
			before = append(before, p)
		case !p.Start.Before(cg.progs[len(cg.progs)-1].Stop):
			after = append(after, p)
		}
	}
	if len(before)+len(after) == 0 {
		return cg
	}
	progs := make([]epg.Programme, 0, len(before)+len(cg.progs)+len(after))
	cg.progs = append(append(append(progs, before...), cg.progs...), after...)
	if cg.origin == "" {
		cg.origin = "archive"
	}
	return cg
}

// recorded tells whether the provider keeps p: it has begun (one on air
// plays from its start) and not before the archive's depth.
func recorded(c *playlist.Catchup, p epg.Programme, now time.Time) bool {
	return c != nil && !p.Start.After(now) && !p.Start.Before(now.Add(-time.Duration(c.Days)*archiveDay))
}

// ArchivePlayback is a recording's address and the programme it starts with.
type ArchivePlayback struct {
	CatalogPlayback
	Archive GuideEntry `json:"archive"`
}

// CatalogPlayArchive starts a past programme (or the one on air from its
// beginning) on a playlist channel. The programme is looked up by its start
// in the guide: the TV names it, the service decides where it ends.
func (a *App) CatalogPlayArchive(ctx context.Context, id, key, entry string, start int64) (ArchivePlayback, error) {
	a.mu.Lock()
	cache, client, err := a.catalogLocked(id)
	if err != nil {
		a.mu.Unlock()
		return ArchivePlayback{}, err
	}
	rec, ok := a.catalogRecordLocked(id, cache, key)
	if !ok && entry != "" {
		_, rec, ok = a.catalogRecordByIDLocked(id, cache, entry)
	}
	if !ok {
		a.mu.Unlock()
		return ArchivePlayback{}, ErrCatalogExpired
	}
	if client != nil || rec.item.Kind != "channel" {
		a.mu.Unlock()
		return ArchivePlayback{}, ErrCatalogArchive
	}
	ch := a.channel(id, rec.item.ID)
	if ch == nil {
		a.mu.Unlock()
		return ArchivePlayback{}, ErrCatalogExpired
	}
	channel, serviceType := *ch, a.st.Settings.ServiceType
	a.mu.Unlock()
	if channel.Catchup == nil {
		return ArchivePlayback{}, ErrCatalogArchive
	}
	cg, err := a.channelGuide(ctx, id, channel.ID)
	if err != nil {
		return ArchivePlayback{}, err
	}
	now := a.now()
	for _, p := range cg.progs {
		if p.Start.Unix() != start {
			continue
		}
		if !recorded(channel.Catchup, p, now) {
			break
		}
		stop := p.Stop
		if stop.After(now) {
			stop = now
		}
		u, err := channel.Catchup.URL(channel.URL, p.Start, stop, now)
		if err != nil {
			return ArchivePlayback{}, ErrCatalogArchive
		}
		a.mu.Lock()
		if a.catalogCurrent(id, cache) {
			a.noteRecentLocked(id, cache, rec)
		}
		a.mu.Unlock()
		return ArchivePlayback{
			CatalogPlayback: CatalogPlayback{URL: u, Name: channel.Name, ServiceType: serviceType, Headers: map[string]string{}},
			Archive:         rowEntry(p),
		}, nil
	}
	return ArchivePlayback{}, ErrCatalogArchive
}
