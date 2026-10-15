package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dreaminator/internal/playlist"
	"dreaminator/internal/probe"
	"dreaminator/internal/stalker"
)

// Account is the last known subscription state of a source.
type Account struct {
	CheckedAt      *time.Time `json:"checkedAt,omitempty"`
	Active         *bool      `json:"active,omitempty"`
	Status         string     `json:"status,omitempty"` // the provider's own word for it
	Expires        *time.Time `json:"expires,omitempty"`
	Unlimited      bool       `json:"unlimited,omitempty"`
	RawExpiry      string     `json:"rawExpiry,omitempty"`
	Plan           string     `json:"plan,omitempty"`
	Trial          bool       `json:"trial,omitempty"`
	Connections    int        `json:"connections,omitempty"`
	MaxConnections int        `json:"maxConnections,omitempty"`
	Note           string     `json:"note,omitempty"`
	Error          string     `json:"error,omitempty"`
}

func ruDate(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), ruMonths[t.Month()-1], t.Year())
}

// fetchAccount asks the provider about the subscription. Only Xtream and
// Stalker have one; for them it is a single cheap request.
func (a *App) fetchAccount(ctx context.Context, src Source, s Settings) *Account {
	now := time.Now()
	acc := &Account{CheckedAt: &now}
	switch src.Type {
	case "xtream":
		x := playlist.Xtream{Server: src.Server, Username: src.Username, Password: src.Password}
		xa, err := x.Account(ctx, a.fetchOptions(s, 30*time.Second))
		if err != nil {
			acc.Error = err.Error()
			return acc
		}
		acc.Active, acc.Status = &xa.Active, xa.Status
		acc.Expires, acc.Unlimited, acc.Trial = xa.Expires, xa.Unlimited, xa.Trial
		acc.Connections, acc.MaxConnections = xa.ActiveCons, xa.MaxConnections
	case "stalker":
		a.mu.Lock()
		cur := a.source(src.ID)
		if cur == nil {
			a.mu.Unlock()
			return nil
		}
		c := a.stalkerClient(cur, s)
		a.mu.Unlock()
		sa, err := c.Account(ctx)
		if err != nil {
			acc.Error = err.Error()
			return acc
		}
		acc.Active, acc.Plan, acc.Status = sa.Active, sa.Plan, sa.Login
		acc.Expires, acc.Unlimited, acc.RawExpiry = sa.Expires, sa.Unlimited, sa.RawExpiry
	default:
		return nil
	}
	return acc
}

func (a *App) storeAccount(id string, acc *Account) {
	if acc == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if src := a.source(id); src != nil {
		src.Account = acc
		a.save()
	}
}

// CheckSource is the "Проверить" button: the subscription for Xtream and
// Stalker, a fresh download for M3U links (there is nothing else to ask).
func (a *App) CheckSource(ctx context.Context, id string) (*Account, error) {
	a.mu.Lock()
	src := a.source(id)
	if src == nil {
		a.mu.Unlock()
		return nil, ErrNotFound
	}
	snapshot, s := *src, a.st.Settings
	a.mu.Unlock()

	switch snapshot.Type {
	case "xtream", "stalker":
		acc := a.fetchAccount(ctx, snapshot, s)
		a.storeAccount(id, acc)
		return acc, nil
	}
	now := time.Now()
	acc := &Account{CheckedAt: &now}
	if err := a.RefreshSource(ctx, id); err != nil {
		acc.Error = err.Error()
	} else {
		a.mu.Lock()
		if src := a.source(id); src != nil {
			what := "Ссылка открывается"
			if snapshot.Type == "file" {
				what = "Файл на ресивере читается"
			}
			acc.Note = fmt.Sprintf("%s, %d каналов", what, src.ChannelCount)
		}
		a.mu.Unlock()
	}
	a.storeAccount(id, acc)
	return acc, nil
}

// accountWarnings feeds the plugin and the summary: expired or soon to
// expire subscriptions, and inactive ones.
func accountWarnings(src *Source, now time.Time) (warnings []string, expiry string) {
	acc := src.Account
	if acc == nil || acc.Error != "" {
		return nil, ""
	}
	if acc.Active != nil && !*acc.Active {
		status := acc.Status
		if status == "" {
			status = "неактивна"
		}
		warnings = append(warnings, fmt.Sprintf("Подписка «%s»: %s", src.Name, status))
	}
	if acc.Expires != nil {
		days := int(acc.Expires.Sub(now).Hours() / 24)
		switch {
		case acc.Expires.Before(now):
			warnings = append(warnings, fmt.Sprintf("Подписка «%s» закончилась %s", src.Name, ruDate(*acc.Expires)))
		case days <= 7:
			warnings = append(warnings, fmt.Sprintf("Подписка «%s» заканчивается через %d дн. (%s)", src.Name, days, ruDate(*acc.Expires)))
		}
		expiry = "до " + ruDate(*acc.Expires)
	}
	return warnings, expiry
}

// ---- stream check ----

// ProbeChannel opens a channel the way the receiver would and reports what
// it finds. It uses one connection to the provider for a few seconds.
func (a *App) ProbeChannel(ctx context.Context, sourceID, channelID string) (probe.Result, error) {
	a.mu.Lock()
	src := a.source(sourceID)
	ch := a.channel(sourceID, channelID)
	if src == nil || ch == nil {
		a.mu.Unlock()
		return probe.Result{}, ErrNotFound
	}
	s := a.st.Settings
	isStalker := src.Type == "stalker"
	target := ch.URL
	a.mu.Unlock()

	ua := s.UserAgent
	if isStalker {
		ua = stalker.UserAgent
		u, err := a.ResolveStalker(ctx, sourceID, channelID)
		if err != nil {
			return probe.Result{Error: "портал не выдал ссылку: " + err.Error()}, nil
		}
		target = u
	}
	r := probe.Probe(ctx, target, probe.Options{Client: a.probeClient(s), UserAgent: ua})
	if s.ServiceType == 4097 && strings.Contains(r.Format, "fMP4") {
		r.Warnings = append(r.Warnings, "Сейчас выбран стандартный проигрыватель GStreamer (4097).")
	}
	return r, nil
}
