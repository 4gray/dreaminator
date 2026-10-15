package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/fetch"
	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
)

type SourceInput struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	EPGURL    string `json:"epgUrl"`
	Server    string `json:"server"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	MAC       string `json:"mac"`
	Serial    string `json:"serial"`
	DeviceID  string `json:"deviceId"`
	DeviceID2 string `json:"deviceId2"`
}

func validURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https" || p.Scheme == "file") && (p.Host != "" || p.Scheme == "file")
}

func (in *SourceInput) validate() error {
	for _, f := range []*string{&in.Name, &in.URL, &in.EPGURL, &in.Server, &in.Username, &in.Password, &in.MAC, &in.Serial, &in.DeviceID, &in.DeviceID2} {
		*f = strings.TrimSpace(*f)
	}
	switch in.Type {
	case "m3u":
		if !validURL(in.URL) {
			return errors.New("нужна ссылка на плейлист, начинающаяся с http:// или https://")
		}
		in.Server, in.Username, in.Password = "", "", ""
	case "xtream":
		in.Server = playlist.NormalizeServer(in.Server)
		if !validURL(in.Server) || in.Username == "" || in.Password == "" {
			return errors.New("для Xtream нужны адрес сервера, логин и пароль")
		}
		in.URL = ""
	case "file":
		// The playlist itself arrives through AddUploadedSource/ReplaceUpload.
		in.URL, in.Server, in.Username, in.Password = "", "", "", ""
	case "stalker":
		if _, err := stalker.Candidates(in.Server); err != nil || in.Server == "" {
			return errors.New("нужен адрес портала, например http://portal.example/stalker_portal/c/")
		}
		mac, err := stalker.NormalizeMAC(in.MAC)
		if err != nil {
			return err
		}
		in.MAC, in.URL = mac, ""
		if !strings.Contains(in.Server, "://") {
			in.Server = "http://" + in.Server
		}
	default:
		return errors.New("неизвестный тип плейлиста")
	}
	if urls := playlist.SplitURLs(in.EPGURL); len(urls) > 0 {
		for _, u := range urls {
			if !validURL(u) {
				return fmt.Errorf("ссылка на программу передач %q должна начинаться с http:// или https://", u)
			}
		}
		in.EPGURL = strings.Join(urls, ", ")
	}
	if in.Type != "stalker" {
		in.MAC, in.Serial, in.DeviceID, in.DeviceID2 = "", "", "", ""
	}
	if in.Name == "" {
		host := in.Server
		if in.Type == "m3u" {
			host = in.URL
		}
		if p, err := url.Parse(host); err == nil && p.Host != "" {
			in.Name = p.Hostname()
		} else {
			in.Name = "Плейлист"
		}
	}
	return nil
}

func (a *App) AddSource(in SourceInput) (*Source, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	s := &Source{ID: newID()}
	applyInput(s, in)
	a.st.Sources = append(a.st.Sources, s)
	a.save()
	cp := *s
	a.mu.Unlock()
	go a.RefreshSource(a.bg, s.ID)
	return &cp, nil
}

func applyInput(s *Source, in SourceInput) {
	before := stalkerFingerprint(*s)
	s.Name, s.Type, s.URL, s.EPGURL = in.Name, in.Type, in.URL, in.EPGURL
	s.Server, s.Username, s.Password = in.Server, in.Username, in.Password
	s.MAC, s.Serial, s.DeviceID, s.DeviceID2 = in.MAC, in.Serial, in.DeviceID, in.DeviceID2
	// A token belongs to one portal and one box identity.
	if stalkerFingerprint(*s) != before {
		s.Stalker = nil
	}
}

func (a *App) UpdateSource(id string, in SourceInput) (*Source, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	s := a.source(id)
	if s == nil {
		a.mu.Unlock()
		return nil, ErrNotFound
	}
	// An uploaded playlist stays an uploaded playlist: switching type would
	// leave the source without anything to read.
	if (s.Type == "file") != (in.Type == "file") {
		a.mu.Unlock()
		return nil, errors.New("тип плейлиста из файла менять нельзя — удалите его и добавьте заново")
	}
	applyInput(s, in)
	a.save()
	cp := *s
	a.mu.Unlock()
	go a.RefreshSource(a.bg, id)
	return &cp, nil
}

func (a *App) DeleteSource(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.st.Sources[:0]
	found := false
	for _, s := range a.st.Sources {
		if s.ID == id {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found {
		return ErrNotFound
	}
	a.st.Sources = kept
	for _, b := range a.st.Bouquets {
		refs := b.Channels[:0]
		for _, r := range b.Channels {
			if r.Source != id {
				refs = append(refs, r)
			}
		}
		if len(refs) != len(b.Channels) {
			a.touch()
		}
		b.Channels = refs
	}
	delete(a.channels, id)
	delete(a.index, id)
	delete(a.stalkers, id)
	os.Remove(a.cachePath(id))
	os.Remove(a.uploadPath(id))
	return a.save()
}

func (a *App) fetchOptions(s Settings, timeout time.Duration) fetch.Options {
	return fetch.Options{UserAgent: s.UserAgent, InsecureTLS: s.InsecureTLS, Timeout: timeout}
}

func (a *App) download(ctx context.Context, src Source, s Settings) (playlist.Result, error) {
	o := a.fetchOptions(s, 3*time.Minute)
	switch src.Type {
	case "m3u":
		rc, err := fetch.Open(ctx, o, src.URL)
		if err != nil {
			return playlist.Result{}, err
		}
		defer rc.Close()
		return playlist.ParseM3U(rc)
	case "file":
		rc, err := fetch.Open(ctx, o, "file://"+a.uploadPath(src.ID))
		if err != nil {
			return playlist.Result{}, errors.New("файл плейлиста не найден на ресивере — загрузите его заново")
		}
		defer rc.Close()
		return playlist.ParseM3U(rc)
	case "stalker":
		return a.fetchStalker(ctx, src)
	case "xtream":
		x := playlist.Xtream{Server: src.Server, Username: src.Username, Password: src.Password, Format: s.StreamFormat}
		return x.Fetch(ctx, o)
	}
	return playlist.Result{}, fmt.Errorf("неизвестный тип %q", src.Type)
}

// RefreshSource downloads a source again and swaps in the new channel list.
func (a *App) RefreshSource(ctx context.Context, id string) error {
	a.mu.Lock()
	src := a.source(id)
	if src == nil {
		a.mu.Unlock()
		return ErrNotFound
	}
	if src.Refreshing {
		a.mu.Unlock()
		return nil
	}
	src.Refreshing = true
	snapshot, settings := *src, a.st.Settings
	a.mu.Unlock()

	res, err := a.download(ctx, snapshot, settings)

	a.mu.Lock()
	defer a.mu.Unlock()
	src = a.source(id)
	if src == nil {
		return nil // deleted meanwhile
	}
	src.Refreshing = false
	if err != nil {
		src.Error = err.Error()
		a.log.Printf("плейлист %q: %v", src.Name, err)
		a.save()
		return err
	}
	now := time.Now()
	src.Error = ""
	src.UpdatedAt = &now
	src.ChannelCount = len(res.Channels)
	src.DetectedEPGURLs, src.DetectedEPGURL = res.EPGURLs, ""

	if a.channelsChanged(id, res.Channels) && a.sourceInUse(id) {
		a.touch()
	}
	a.setChannels(id, res.Channels)
	if data, err := json.Marshal(res.Channels); err == nil {
		if err := enigma.WriteFileAtomic(a.cachePath(id), data); err != nil {
			a.log.Printf("не удалось сохранить каналы: %v", err)
		}
	}
	a.log.Printf("плейлист %q обновлён: %d каналов", src.Name, len(res.Channels))
	return a.save()
}

func (a *App) channelsChanged(sourceID string, next []playlist.Channel) bool {
	prev := a.channels[sourceID]
	if len(prev) != len(next) {
		return true
	}
	for i := range prev {
		if prev[i] != next[i] {
			return true
		}
	}
	return false
}

func (a *App) sourceInUse(sourceID string) bool {
	for _, b := range a.st.Bouquets {
		for _, r := range b.Channels {
			if r.Source == sourceID {
				return true
			}
		}
	}
	return false
}

// ---- browsing ----

type Group struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

const noGroup = "Без группы"

func (a *App) Groups(sourceID string) ([]Group, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.source(sourceID) == nil {
		return nil, ErrNotFound
	}
	pos := map[string]int{}
	groups := []Group{}
	for _, c := range a.channels[sourceID] {
		g := c.Group
		if g == "" {
			g = noGroup
		}
		i, ok := pos[g]
		if !ok {
			i = len(groups)
			pos[g] = i
			groups = append(groups, Group{Name: g})
		}
		groups[i].Count++
	}
	return groups, nil
}

type ChannelView struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Logo   string `json:"logo,omitempty"`
}

type ChannelPage struct {
	Items []ChannelView `json:"items"`
	Total int           `json:"total"`
}

// Channels lists channels of one source (or all sources when sourceID is
// empty), optionally narrowed to a group and a search query.
func (a *App) Channels(sourceID, group, query string, limit int) ChannelPage {
	a.mu.Lock()
	defer a.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	page := ChannelPage{Items: []ChannelView{}}
	for _, s := range a.st.Sources {
		if sourceID != "" && s.ID != sourceID {
			continue
		}
		for _, c := range a.channels[s.ID] {
			g := c.Group
			if g == "" {
				g = noGroup
			}
			if group != "" && g != group {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(c.Name), q) {
				continue
			}
			page.Total++
			if limit <= 0 || len(page.Items) < limit {
				page.Items = append(page.Items, ChannelView{Source: s.ID, ID: c.ID, Name: c.Name, Group: g, Logo: c.Logo})
			}
		}
	}
	return page
}
