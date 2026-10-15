package playlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

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

func (x Xtream) Fetch(ctx context.Context, o fetch.Options) (Result, error) {
	var raw json.RawMessage
	if err := x.call(ctx, o, "", &raw); err != nil {
		return Result{}, err
	}
	var auth struct {
		UserInfo *struct {
			Auth   flexString `json:"auth"`
			Status string     `json:"status"`
		} `json:"user_info"`
	}
	if json.Unmarshal(raw, &auth) != nil || auth.UserInfo == nil || auth.UserInfo.Auth != "1" {
		return Result{}, errors.New("сервер не принял логин или пароль")
	}
	if st := strings.ToLower(auth.UserInfo.Status); st != "" && st != "active" {
		return Result{}, fmt.Errorf("подписка неактивна (статус: %s)", auth.UserInfo.Status)
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
