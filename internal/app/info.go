package app

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// ---- where the tablet finds us ----

type ifaceAddr struct {
	Name string
	IP   net.IP
}

func ifaceRank(name string) int {
	switch {
	case strings.HasPrefix(name, "eth"), strings.HasPrefix(name, "en"):
		return 0
	case strings.HasPrefix(name, "wlan"), strings.HasPrefix(name, "wl"), strings.HasPrefix(name, "ra"):
		return 1
	}
	return 2
}

var virtualIfaces = []string{"lo", "docker", "veth", "br-", "tun", "tap", "tailscale", "utun", "wg", "zt"}

// pickAddresses orders the receiver's IPv4 addresses the way a person would
// look for them: wired first, then Wi-Fi; VPN and container bridges never.
func pickAddresses(addrs []ifaceAddr) []string {
	var keep []ifaceAddr
	for _, a := range addrs {
		ip := a.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		virtual := false
		for _, p := range virtualIfaces {
			if strings.HasPrefix(a.Name, p) {
				virtual = true
			}
		}
		if !virtual {
			keep = append(keep, ifaceAddr{a.Name, ip})
		}
	}
	sort.SliceStable(keep, func(i, j int) bool { return ifaceRank(keep[i].Name) < ifaceRank(keep[j].Name) })
	out := make([]string, len(keep))
	for i, a := range keep {
		out[i] = a.IP.String()
	}
	return out
}

func lanAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var all []ifaceAddr
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				all = append(all, ifaceAddr{iface.Name, n.IP})
			}
		}
	}
	return pickAddresses(all)
}

// TabletURLs are the addresses to open on the tablet, best first.
func (a *App) TabletURLs() []string {
	var urls []string
	for _, ip := range lanAddresses() {
		urls = append(urls, fmt.Sprintf("http://%s:%s", ip, a.cfg.Port))
	}
	return urls
}

// ---- compact status for the on-TV plugin ----

// Summary is what the Enigma2 plugin shows. Texts are ready to display so the
// plugin, which runs inside Enigma2, stays as simple as possible.
type Summary struct {
	Version         string   `json:"version"`
	URL             string   `json:"url"`
	URLs            []string `json:"urls"`
	Sources         int      `json:"sources"`
	Bouquets        int      `json:"bouquets"`
	Channels        int      `json:"channels"`
	Busy            bool     `json:"busy"`
	Refreshing      bool     `json:"refreshing"`
	LastRefreshText string   `json:"lastRefreshText"`
	Dirty           bool     `json:"dirty"`
	LastApplyText   string   `json:"lastApplyText"`
	LastApplyError  string   `json:"lastApplyError,omitempty"`
	EPGText         string   `json:"epgText"`
	Subscription    string   `json:"subscription,omitempty"` // nearest expiry
	Problems        []string `json:"problems"`
}

var ruMonths = []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

// humanWhen says when something happened the way the tablet UI does.
func humanWhen(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "только что"
	case d < time.Hour:
		return fmt.Sprintf("%d мин назад", int(d.Minutes()))
	}
	clock := t.Format("15:04")
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "сегодня в " + clock
	}
	if yy, ym, yd := now.AddDate(0, 0, -1).Date(); y1 == yy && m1 == ym && d1 == yd {
		return "вчера в " + clock
	}
	return fmt.Sprintf("%d %s в %s", d1, ruMonths[m1-1], clock)
}

func (a *App) Summary() Summary {
	urls := a.TabletURLs()
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	s := Summary{
		Version:        a.cfg.Version,
		URLs:           urls,
		Sources:        len(a.st.Sources),
		Bouquets:       len(a.st.Bouquets),
		Dirty:          a.st.Dirty,
		LastApplyError: a.st.LastApplyError,
		Refreshing:     a.refreshingAll.Load(),
		Problems:       []string{},
	}
	if len(urls) > 0 {
		s.URL = urls[0]
	}
	var newest, soonest *time.Time
	for _, src := range a.st.Sources {
		if src.Refreshing {
			s.Refreshing = true
		}
		if src.UpdatedAt != nil && (newest == nil || src.UpdatedAt.After(*newest)) {
			newest = src.UpdatedAt
		}
		if src.Error != "" {
			s.Problems = append(s.Problems, fmt.Sprintf("«%s»: %s", src.Name, src.Error))
		}
		warnings, expiry := accountWarnings(src, now)
		s.Problems = append(s.Problems, warnings...)
		if expiry != "" && (soonest == nil || src.Account.Expires.Before(*soonest)) {
			soonest = src.Account.Expires
			s.Subscription = expiry
			if len(a.st.Sources) > 1 {
				s.Subscription = fmt.Sprintf("«%s» %s", src.Name, expiry)
			}
		}
	}
	if newest != nil {
		s.LastRefreshText = humanWhen(*newest, now)
	}
	seen := map[string]bool{}
	for _, b := range a.st.Bouquets {
		for _, r := range b.Channels {
			if !seen[r.key()] && a.channel(r.Source, r.ID) != nil {
				seen[r.key()] = true
			}
		}
	}
	s.Channels = len(seen)
	if a.st.LastApply != nil {
		s.LastApplyText = humanWhen(*a.st.LastApply, now)
	}
	if a.st.LastApplyError != "" {
		s.Problems = append(s.Problems, "Отправка на ТВ: "+a.st.LastApplyError)
	}
	e := a.st.EPG
	switch {
	case e.Running:
		s.EPGText = "обновляется…"
	case e.UpdatedAt != nil && e.Wanted > 0:
		s.EPGText = fmt.Sprintf("есть у %d из %d каналов, обновлена %s", e.Matched, e.Wanted, humanWhen(*e.UpdatedAt, now))
	case e.UpdatedAt != nil:
		s.EPGText = "в списках пока нет каналов"
	default:
		s.EPGText = "ещё не загружалась"
	}
	if e.Error != "" {
		s.Problems = append(s.Problems, "Программа передач: "+e.Error)
	}
	s.Busy = s.Refreshing || e.Running
	return s
}

// ---- refresh everything on request ----

// RefreshAll re-downloads every playlist and pushes the result to the TV,
// the same as the nightly run but on demand (the plugin's yellow button).
// It returns false when a run is already in progress.
func (a *App) RefreshAll(ctx context.Context) bool {
	if !a.refreshingAll.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer a.refreshingAll.Store(false)
		a.mu.Lock()
		ids := make([]string, 0, len(a.st.Sources))
		for _, s := range a.st.Sources {
			ids = append(ids, s.ID)
		}
		a.mu.Unlock()
		for _, id := range ids {
			a.RefreshSource(ctx, id)
		}
		if err := a.Apply(ctx); err != nil {
			a.log.Printf("обновление по запросу: %v", err)
		}
	}()
	return true
}
