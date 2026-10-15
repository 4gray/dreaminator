package playlist

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Catchup is a channel's archive (catch-up): how a past programme is asked
// from the provider and how far back it goes. The rules follow Kodi's
// pvr.iptvsimple, which most playlists are written for.
type Catchup struct {
	Mode   string `json:"mode"`             // shift, append, default, flussonic, xc
	Days   int    `json:"days"`             // depth of the archive
	Source string `json:"source,omitempty"` // catchup-source: a template
}

const (
	// MaxCatchupDays bounds a playlist's claim: the guide kept for the
	// archive costs memory and flash per day.
	MaxCatchupDays = 14
	// defaultCatchupDays is pvr.iptvsimple's depth for a channel that names
	// a mode but no depth.
	defaultCatchupDays = 5
)

// catchupModes are the modes a playlist may name, by their aliases.
var catchupModes = map[string]string{
	"default": "default", "append": "append",
	"shift": "shift", "timeshift": "shift",
	"flussonic": "flussonic", "flussonic-hls": "flussonic", "flussonic-ts": "flussonic", "fs": "flussonic",
	"xc": "xc",
}

// parseCatchup reads a channel's archive from its attributes, over the
// playlist's defaults from the #EXTM3U header. Playlists that only give a
// depth (Edem: tvg-rec="7"; VipLime adds timeshift="3") answer ?utc=&lutc=,
// so a depth without a mode means shift. nil: no archive, or a mode we do
// not know.
func parseCatchup(a, header map[string]string) *Catchup {
	get := func(keys ...string) string {
		for _, m := range []map[string]string{a, header} {
			for _, k := range keys {
				if v := strings.TrimSpace(m[k]); v != "" {
					return v
				}
			}
		}
		return ""
	}
	mode := strings.ToLower(get("catchup", "catchup-type"))
	days := 0
	if v := get("catchup-days", "tvg-rec", "timeshift"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil // tvg-rec="0": this channel has no archive
		}
		days = n
	}
	if mode == "" {
		if days == 0 {
			return nil
		}
		mode = "shift"
	}
	m, ok := catchupModes[mode]
	if !ok {
		return nil // "vod" and the like: not an archive we can play
	}
	if days == 0 {
		days = defaultCatchupDays
	}
	if days > MaxCatchupDays {
		days = MaxCatchupDays
	}
	return &Catchup{Mode: m, Days: days, Source: get("catchup-source")}
}

var ErrCatchup = errors.New("у канала нет архива в понятном формате")

// URL is the address of what was on air from start to stop on a channel
// whose live stream is stream; now is the moment of the request. For a
// programme still on air the caller passes now as stop.
func (c *Catchup) URL(stream string, start, stop, now time.Time) (string, error) {
	if c == nil {
		return "", ErrCatchup
	}
	switch c.Mode {
	case "default":
		if c.Source != "" {
			return fillCatchup(c.Source, start, stop, now), nil
		}
		return shiftURL(stream, start, now), nil
	case "append":
		if c.Source == "" {
			return shiftURL(stream, start, now), nil
		}
		base, rest := splitTail(stream)
		return base + fillCatchup(c.Source, start, stop, now) + rest, nil
	case "shift":
		return shiftURL(stream, start, now), nil
	case "flussonic", "xc":
		base, rest := splitTail(stream)
		build := flussonicURL
		if c.Mode == "xc" {
			build = xcURL
		}
		u, err := build(base, start, stop)
		return u + rest, err
	}
	return "", ErrCatchup
}

// shiftURL asks for the archive from start; lutc is the moment of the
// request, as in pvr.iptvsimple. Edem reads it so (lutc = the programme's
// end gave an empty list on the DM520, 2026-10-07): the recording plays on
// like a delayed live stream. VipLime answers a recording up to lutc and
// cuts the list at six hours itself (4320 segments, 380 KB).
func shiftURL(stream string, start, now time.Time) string {
	base, rest := splitTail(stream)
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sutc=%d&lutc=%d%s", base, sep, start.Unix(), now.Unix(), rest)
}

// splitTail cuts what follows the address itself: a fragment (Enigma2's
// "#User-Agent=…") or Kodi's "|User-Agent=…". Archive parameters go before
// it, or the player would drop them with it and play the live stream.
func splitTail(stream string) (base, rest string) {
	if i := strings.IndexAny(stream, "#|"); i >= 0 {
		return stream[:i], stream[i:]
	}
	return stream, ""
}

// fluss: the stream's host, the channel path, the file name and the query.
var fluss = regexp.MustCompile(`^(https?://[^/]+)/(.*)/([^/]*?)(mpegts|\.m3u8)(\?.*)?$`)

func flussonicURL(stream string, start, stop time.Time) (string, error) {
	m := fluss.FindStringSubmatch(stream)
	if m == nil {
		return "", ErrCatchup
	}
	host, channel, name, kind, query := m[1], m[2], m[3], m[4], m[5]
	if kind == "mpegts" {
		return fmt.Sprintf("%s/%s/timeshift_abs-%d.ts%s", host, channel, start.Unix(), query), nil
	}
	if name == "" {
		name = "index"
	}
	return fmt.Sprintf("%s/%s/%s-%d-%d.m3u8%s", host, channel, name, start.Unix(), int64(stop.Sub(start).Seconds()), query), nil
}

// xc: an Xtream Codes live address, /live/user/pass/id.ext or /user/pass/id.
var xc = regexp.MustCompile(`^(https?://[^/]+)/(?:live/)?([^/]+)/([^/]+)/(\d+)(?:\.[a-z0-9]+)?(\?.*)?$`)

func xcURL(stream string, start, stop time.Time) (string, error) {
	m := xc.FindStringSubmatch(stream)
	if m == nil {
		return "", ErrCatchup
	}
	minutes := int(stop.Sub(start).Minutes() + 0.5)
	return fmt.Sprintf("%s/timeshift/%s/%s/%d/%s/%s.ts%s", m[1], m[2], m[3], minutes, start.UTC().Format("2006-01-02:15-04"), m[4], m[5]), nil
}

// placeholder: {utc}, ${start}, {duration:60}, {utc:Y-m-d}… as in pvr.iptvsimple.
var placeholder = regexp.MustCompile(`\$?\{([A-Za-z]+)(?::([^}]*))?\}`)

// fillCatchup puts the times into a catchup-source template. Calendar
// placeholders are in UTC, as pvr.iptvsimple writes them.
func fillCatchup(tmpl string, start, stop, now time.Time) string {
	dur := int64(stop.Sub(start).Seconds())
	return placeholder.ReplaceAllStringFunc(tmpl, func(s string) string {
		m := placeholder.FindStringSubmatch(s)
		name, arg := m[1], m[2]
		var t time.Time
		switch name {
		case "utc", "start":
			t = start
		case "utcend", "end":
			t = stop
		case "lutc", "now", "timestamp":
			t = now
		case "duration":
			return strconv.FormatInt(dur/divisor(arg), 10)
		case "offset":
			return strconv.FormatInt(int64(now.Sub(start).Seconds())/divisor(arg), 10)
		case "Y", "m", "d", "H", "M", "S":
			return calendar(start, name)
		default:
			return s
		}
		if arg == "" {
			return strconv.FormatInt(t.Unix(), 10)
		}
		var b strings.Builder
		for _, r := range arg {
			if strings.ContainsRune("YmdHMS", r) {
				b.WriteString(calendar(t, string(r)))
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	})
}

func divisor(arg string) int64 {
	if n, err := strconv.ParseInt(arg, 10, 64); err == nil && n > 0 {
		return n
	}
	return 1
}

func calendar(t time.Time, part string) string {
	t = t.UTC()
	switch part {
	case "Y":
		return strconv.Itoa(t.Year())
	case "m":
		return fmt.Sprintf("%02d", int(t.Month()))
	case "d":
		return fmt.Sprintf("%02d", t.Day())
	case "H":
		return fmt.Sprintf("%02d", t.Hour())
	case "M":
		return fmt.Sprintf("%02d", t.Minute())
	case "S":
		return fmt.Sprintf("%02d", t.Second())
	}
	return ""
}
