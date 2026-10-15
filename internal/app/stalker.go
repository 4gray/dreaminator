package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dreaminator/internal/epg"
	"dreaminator/internal/fetch"
	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
)

// Stalker links are temporary, so bouquets point at Dreaminator itself
// (/stalker/<source>/<channel>); the resolver asks the portal for a fresh
// link at play time and redirects the player there. The MAG User-Agent rides
// along for players that honour the #User-Agent= suffix.

type stalkerEntry struct {
	fingerprint string
	client      *stalker.Client
}

func stalkerFingerprint(s Source) string {
	return strings.Join([]string{s.Type, s.Server, s.MAC, s.Serial, s.DeviceID, s.DeviceID2, s.Username, s.Password}, "\x00")
}

// stalkerClient returns the (cached) portal client for a source; callers
// hold a.mu.
func (a *App) stalkerClient(src *Source, s Settings) *stalker.Client {
	fp := stalkerFingerprint(*src)
	if e, ok := a.stalkers[src.ID]; ok && e.fingerprint == fp {
		return e.client
	}
	var sess stalker.Session
	if src.Stalker != nil {
		sess = *src.Stalker
	}
	id := stalker.Identity{MAC: src.MAC, Serial: src.Serial, DeviceID: src.DeviceID, DeviceID2: src.DeviceID2,
		Login: src.Username, Password: src.Password}
	c := stalker.New(src.Server, id, sess, fetch.Client(a.fetchOptions(s, 30*time.Second)))
	sourceID := src.ID
	c.OnSession(func(sess stalker.Session) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if s := a.source(sourceID); s != nil && stalkerFingerprint(*s) == fp {
			s.Stalker = &sess
			a.save()
		}
	})
	a.stalkers[src.ID] = &stalkerEntry{fingerprint: fp, client: c}
	return c
}

func (a *App) resolverURL(sourceID, channelID string) string {
	return fmt.Sprintf("http://127.0.0.1:%s/stalker/%s/%s#User-Agent=%s",
		a.cfg.Port, sourceID, url.PathEscape(channelID), stalker.UserAgent)
}

func (a *App) fetchStalker(ctx context.Context, src Source) (playlist.Result, error) {
	a.mu.Lock()
	cur := a.source(src.ID)
	if cur == nil {
		a.mu.Unlock()
		return playlist.Result{}, ErrNotFound
	}
	c := a.stalkerClient(cur, a.st.Settings)
	a.mu.Unlock()

	genres, err := c.Genres(ctx)
	if err != nil {
		return playlist.Result{}, err
	}
	titles := map[string]string{}
	for _, g := range genres {
		titles[g.ID] = g.Title
	}
	chans, err := c.Channels(ctx, genres)
	if err != nil {
		return playlist.Result{}, err
	}
	if len(chans) == 0 {
		return playlist.Result{}, fmt.Errorf("на портале нет ни одного телеканала")
	}
	res := playlist.Result{Channels: make([]playlist.Channel, 0, len(chans))}
	for _, ch := range chans {
		res.Channels = append(res.Channels, playlist.Channel{
			ID:        ch.ID,
			Name:      ch.Name,
			Group:     titles[ch.GenreID],
			Logo:      ch.Logo,
			TvgID:     ch.XMLTVID,
			URL:       a.resolverURL(src.ID, ch.ID),
			Cmd:       ch.Cmd,
			NeedsLink: ch.NeedsLink,
		})
	}
	return res, nil
}

// ResolveStalker returns a playable address for a Stalker channel right now.
func (a *App) ResolveStalker(ctx context.Context, sourceID, channelID string) (string, error) {
	a.mu.Lock()
	src := a.source(sourceID)
	ch := a.channel(sourceID, channelID)
	if src == nil || ch == nil || src.Type != "stalker" {
		a.mu.Unlock()
		return "", ErrNotFound
	}
	c := a.stalkerClient(src, a.st.Settings)
	sc := stalker.Channel{ID: ch.ID, Cmd: ch.Cmd, NeedsLink: ch.NeedsLink}
	a.mu.Unlock()

	u, err := c.Resolve(ctx, sc)
	if err != nil {
		a.log.Printf("stalker: канал %s: %v", channelID, err)
	}
	return u, err
}

// stalkerGuide writes the portal's own programme guide for the wanted
// channels: the bulk get_epg_info first, get_short_epg for the few channels
// a portal leaves out of it.
func (a *App) stalkerGuide(ctx context.Context, sourceID, prefix string, wants []epg.Want, w io.Writer,
	mapping map[string][]string, matched map[string]bool) (int, error) {
	a.mu.Lock()
	src := a.source(sourceID)
	if src == nil {
		a.mu.Unlock()
		return 0, ErrNotFound
	}
	c := a.stalkerClient(src, a.st.Settings)
	a.mu.Unlock()

	guide, err := c.Guide(ctx, 72)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-2 * time.Hour)
	written, fallbacks := 0, 0
	for _, want := range wants {
		progs := guide[want.TvgID]
		if len(progs) == 0 && fallbacks < 40 {
			fallbacks++
			progs, _ = c.ShortGuide(ctx, want.TvgID, 30)
		}
		if len(progs) == 0 {
			continue
		}
		oid := prefix + want.TvgID
		name := want.TvgID
		if len(want.Names) > 0 && want.Names[0] != "" {
			name = want.Names[0]
		}
		epg.WriteChannel(w, oid, name)
		for _, p := range progs {
			if p.Stop.Add(want.Shift).Before(cutoff) {
				continue
			}
			epg.WriteProgramme(w, oid, p.Start.Add(want.Shift), p.Stop.Add(want.Shift), p.Title, p.Desc)
			written++
		}
		mapping[oid] = append(mapping[oid], want.Ref)
		matched[want.Ref] = true
	}
	return written, nil
}

func (a *App) probeClient(s Settings) *http.Client {
	c := fetch.Client(a.fetchOptions(s, 0))
	c.Timeout = 0 // the probe has its own budget
	return c
}
