package playlist

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dreaminator/internal/fetch"
)

type Xtream struct {
	Server   string
	Username string
	Password string
	Format   string // "ts" or "m3u8"
}

// flexString accepts JSON strings, numbers and null: Xtream panels are
// inconsistent about the types of ids.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(b)
	return nil
}

// NormalizeServer accepts what people tend to paste: bare hosts, trailing
// slashes, or the full player_api.php / get.php address.
func NormalizeServer(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "?"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, "/")
	for _, suffix := range []string{"/player_api.php", "/get.php", "/xmltv.php"} {
		s = strings.TrimSuffix(s, suffix)
	}
	if s != "" && !strings.Contains(s, "://") {
		s = "http://" + s
	}
	return s
}

func (x Xtream) query() url.Values {
	return url.Values{"username": {x.Username}, "password": {x.Password}}
}

func (x Xtream) call(ctx context.Context, o fetch.Options, action string, out any) error {
	q := x.query()
	if action != "" {
		q.Set("action", action)
	}
	return x.get(ctx, o, q, out)
}

func (x Xtream) get(ctx context.Context, o fetch.Options, q url.Values, out any) error {
	rc, err := fetch.Open(ctx, o, NormalizeServer(x.Server)+"/player_api.php?"+q.Encode())
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := json.NewDecoder(rc).Decode(out); err != nil {
		return fmt.Errorf("сервер Xtream ответил непонятно (%v)", err)
	}
	return nil
}

func (x Xtream) EPGURL() string {
	return NormalizeServer(x.Server) + "/xmltv.php?" + x.query().Encode()
}

func (x Xtream) StreamURL(streamID string) string {
	ext := "ts"
	if x.Format == "m3u8" {
		ext = "m3u8"
	}
	return fmt.Sprintf("%s/live/%s/%s/%s.%s", NormalizeServer(x.Server),
		url.PathEscape(x.Username), url.PathEscape(x.Password), streamID, ext)
}

// XtreamAccount is the subscription state from player_api's user_info.
type XtreamAccount struct {
	Status         string     // as the panel says it: Active, Banned, Disabled, Expired…
	Active         bool       //
	Expires        *time.Time // nil with Unlimited
	Unlimited      bool       //
	Trial          bool       //
	ActiveCons     int        //
	MaxConnections int        //
}

func (f flexString) int() int {
	n, _ := strconv.Atoi(strings.TrimSpace(string(f)))
	return n
}

// Account logs in and reports the subscription. A rejected login is an
// error; an inactive subscription is not (the caller shows its status).
func (x Xtream) Account(ctx context.Context, o fetch.Options) (XtreamAccount, error) {
	var raw json.RawMessage
	if err := x.call(ctx, o, "", &raw); err != nil {
		return XtreamAccount{}, err
	}
	var auth struct {
		UserInfo *struct {
			Auth           flexString `json:"auth"`
			Status         string     `json:"status"`
			ExpDate        flexString `json:"exp_date"`
			IsTrial        flexString `json:"is_trial"`
			ActiveCons     flexString `json:"active_cons"`
			MaxConnections flexString `json:"max_connections"`
		} `json:"user_info"`
	}
	if json.Unmarshal(raw, &auth) != nil || auth.UserInfo == nil || auth.UserInfo.Auth != "1" {
		return XtreamAccount{}, errors.New("сервер не принял логин или пароль")
	}
	u := auth.UserInfo
	acc := XtreamAccount{
		Status:         u.Status,
		Active:         u.Status == "" || strings.EqualFold(u.Status, "active"),
		Trial:          u.IsTrial.int() == 1,
		ActiveCons:     u.ActiveCons.int(),
		MaxConnections: u.MaxConnections.int(),
	}
	if exp := u.ExpDate.int(); exp > 0 {
		t := time.Unix(int64(exp), 0)
		acc.Expires = &t
	} else {
		acc.Unlimited = true
	}
	return acc, nil
}

func (x Xtream) Fetch(ctx context.Context, o fetch.Options) (Result, error) {
	acc, err := x.Account(ctx, o)
	if err != nil {
		return Result{}, err
	}
	if !acc.Active {
		return Result{}, fmt.Errorf("подписка неактивна (статус: %s)", acc.Status)
	}

	var cats []struct {
		ID   flexString `json:"category_id"`
		Name string     `json:"category_name"`
	}
	// Categories are cosmetic; channels still work without them.
	_ = x.call(ctx, o, "get_live_categories", &cats)
	catName := map[flexString]string{}
	catOrder := map[flexString]int{}
	for i, c := range cats {
		catName[c.ID] = strings.TrimSpace(c.Name)
		catOrder[c.ID] = i
	}

	var streams []struct {
		ID    flexString `json:"stream_id"`
		Name  string     `json:"name"`
		Icon  string     `json:"stream_icon"`
		EPGID flexString `json:"epg_channel_id"`
		Cat   flexString `json:"category_id"`
	}
	if err := x.call(ctx, o, "get_live_streams", &streams); err != nil {
		return Result{}, err
	}
	sort.SliceStable(streams, func(i, j int) bool {
		oi, iok := catOrder[streams[i].Cat]
		oj, jok := catOrder[streams[j].Cat]
		if iok != jok {
			return iok
		}
		return oi < oj
	})

	res := Result{EPGURLs: []string{x.EPGURL()}}
	seen := map[string]bool{}
	for _, s := range streams {
		id := string(s.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		res.Channels = append(res.Channels, Channel{
			ID:    id,
			Name:  strings.TrimSpace(s.Name),
			Group: catName[s.Cat],
			Logo:  s.Icon,
			TvgID: string(s.EPGID),
			URL:   x.StreamURL(id),
		})
	}
	if len(res.Channels) == 0 {
		return res, errors.New("у аккаунта нет ни одного телеканала")
	}
	return res, nil
}

// ShortProgramme is one entry of a provider's short guide.
type ShortProgramme struct {
	Start, Stop time.Time
	Title, Desc string
}

// ShortEPG asks the panel what is on a channel now and next
// (get_short_epg). Titles and descriptions come base64-encoded; times are
// taken from the unix timestamps when the panel sends them.
func (x Xtream) ShortEPG(ctx context.Context, o fetch.Options, streamID string, limit int) ([]ShortProgramme, error) {
	q := x.query()
	q.Set("action", "get_short_epg")
	q.Set("stream_id", streamID)
	q.Set("limit", strconv.Itoa(limit))
	var raw json.RawMessage
	if err := x.get(ctx, o, q, &raw); err != nil {
		return nil, err
	}
	var reply struct {
		Listings []struct {
			Title   flexString `json:"title"`
			Desc    flexString `json:"description"`
			Start   string     `json:"start"`
			End     string     `json:"end"`
			StartTS flexString `json:"start_timestamp"`
			StopTS  flexString `json:"stop_timestamp"`
		} `json:"epg_listings"`
	}
	// A channel without a guide is answered with [] by some panels.
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("сервер Xtream ответил непонятно (%v)", err)
	}
	var out []ShortProgramme
	for _, l := range reply.Listings {
		p := ShortProgramme{Title: unbase64(string(l.Title)), Desc: unbase64(string(l.Desc))}
		start, end, ok := panelTimes(l.Start, l.End)
		if ts := l.StartTS.int(); ts > 0 {
			p.Start = time.Unix(int64(ts), 0)
			if stop := l.StopTS.int(); stop > ts {
				p.Stop = time.Unix(int64(stop), 0)
			} else if ok {
				p.Stop = p.Start.Add(end.Sub(start)) // the length, whatever zone the panel writes in
			}
		} else if ok {
			p.Start, p.Stop = start, end
		}
		if p.Title != "" && p.Stop.After(p.Start) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// panelTimes reads "2026-10-05 18:00:00" pairs; without timestamps the
// panel's zone is unknown and the receiver's is the best guess.
func panelTimes(start, end string) (time.Time, time.Time, bool) {
	const layout = "2006-01-02 15:04:05"
	s, err1 := time.ParseInLocation(layout, strings.TrimSpace(start), time.Local)
	e, err2 := time.ParseInLocation(layout, strings.TrimSpace(end), time.Local)
	return s, e, err1 == nil && err2 == nil && e.After(s)
}

// unbase64 decodes the panel's base64 texts; a panel that sends plain text
// gets it back as is.
func unbase64(s string) string {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil && utf8.Valid(b) {
			return strings.TrimSpace(string(b))
		}
	}
	return s
}
