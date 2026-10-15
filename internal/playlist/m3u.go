// Package playlist turns M3U playlists and Xtream Codes accounts into a flat
// list of live channels.
package playlist

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Channel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group,omitempty"`
	Logo  string `json:"logo,omitempty"`
	TvgID string `json:"tvgId,omitempty"`
	// TvgName is the guide's name for the channel when it differs from the
	// display name (tvg-name); it often matches XMLTV better.
	TvgName string `json:"tvgName,omitempty"`
	// TvgShift moves the guide by this many hours (tvg-shift), e.g. 2 for a
	// "+2" timeshift channel that shares the original channel's guide.
	TvgShift float64 `json:"tvgShift,omitempty"`
	URL      string  `json:"url"`
	// Stalker channels: the portal command behind URL (which points at
	// Dreaminator's own resolver) and whether it needs create_link.
	Cmd       string `json:"cmd,omitempty"`
	NeedsLink bool   `json:"needsLink,omitempty"`
	// Catchup is the channel's archive (catchup.go); it plays only from
	// the TV catalog, bouquets keep the live address.
	Catchup *Catchup `json:"catchup,omitempty"`
}

type Result struct {
	Channels []Channel
	// EPGURLs are the guides advertised by the playlist itself (url-tvg,
	// often several) or by the Xtream provider, in order of preference.
	EPGURLs []string
}

var urlSep = regexp.MustCompile(`[,\s]+`)

// SplitURLs splits a list of guide addresses separated by commas, spaces or
// new lines, dropping duplicates.
func SplitURLs(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range urlSep.Split(strings.TrimSpace(s), -1) {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

func parseShift(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(s, ",", ".")), 64)
	if err != nil || v < -24 || v > 24 {
		return 0
	}
	return v
}

var attrRe = regexp.MustCompile(`([A-Za-z0-9_-]+)=(?:"([^"]*)"|'([^']*)'|([^\s"',]+))`)

func parseAttrs(s string) map[string]string {
	attrs := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(s, -1) {
		attrs[strings.ToLower(m[1])] = strings.TrimSpace(m[2] + m[3] + m[4])
	}
	return attrs
}

// parseExtinf reads an #EXTINF line; header holds the #EXTM3U attributes,
// the playlist's defaults.
func parseExtinf(line string, defaultShift float64, header map[string]string) Channel {
	body := strings.TrimPrefix(line, "#EXTINF")
	body = strings.TrimPrefix(body, ":")
	// The display name follows the first comma that is not inside quotes.
	inQuote, idx := false, -1
	for i, r := range body {
		if r == '"' {
			inQuote = !inQuote
		} else if r == ',' && !inQuote {
			idx = i
			break
		}
	}
	meta, name := body, ""
	if idx >= 0 {
		meta, name = body[:idx], strings.TrimSpace(body[idx+1:])
	}
	a := parseAttrs(meta)
	ch := Channel{Name: name, TvgID: a["tvg-id"], TvgName: a["tvg-name"], Logo: a["tvg-logo"], Group: a["group-title"], TvgShift: defaultShift}
	if v, ok := a["tvg-shift"]; ok {
		ch.TvgShift = parseShift(v)
	}
	ch.Catchup = parseCatchup(a, header)
	if ch.Name == "" {
		ch.Name = ch.TvgName
	}
	if ch.TvgName == ch.Name {
		ch.TvgName = ""
	}
	return ch
}

// isVOD filters out movies and series that some providers mix into the live
// playlist: they do not belong in Enigma2 bouquets.
func isVOD(u string) bool {
	l := strings.ToLower(u)
	if i := strings.IndexAny(l, "?#"); i >= 0 {
		l = l[:i]
	}
	if strings.Contains(l, "/movie/") || strings.Contains(l, "/series/") {
		return true
	}
	for _, ext := range []string{".mp4", ".mkv", ".avi", ".mov"} {
		if strings.HasSuffix(l, ext) {
			return true
		}
	}
	return false
}

// idGen derives stable channel IDs so favourites survive playlist refreshes
// even when stream URLs (tokens) change.
type idGen map[string]int

func (g idGen) next(ch Channel) string {
	sum := sha1.Sum([]byte(strings.ToLower(ch.TvgID + "|" + ch.Name)))
	base := hex.EncodeToString(sum[:6])
	g[base]++
	if n := g[base]; n > 1 {
		return fmt.Sprintf("%s-%d", base, n)
	}
	return base
}

func ParseM3U(r io.Reader) (Result, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var (
		res        Result
		pending    *Channel
		extgrp     string
		ids        = idGen{}
		first      = true
		sawHeader  bool
		sawEntries bool
		shift      float64 // playlist-wide tvg-shift from the #EXTM3U header
		header     = map[string]string{}
	)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXTM3U"):
			sawHeader = true
			a := parseAttrs(line)
			header = a
			var urls []string
			for _, k := range []string{"url-tvg", "x-tvg-url", "tvg-url"} {
				urls = append(urls, SplitURLs(a[k])...)
			}
			res.EPGURLs = SplitURLs(strings.Join(urls, " "))
			shift = parseShift(a["tvg-shift"])
		case strings.HasPrefix(line, "#EXTINF"):
			ch := parseExtinf(line, shift, header)
			pending = &ch
			sawEntries = true
		case strings.HasPrefix(line, "#EXTGRP:"):
			extgrp = strings.TrimSpace(strings.TrimPrefix(line, "#EXTGRP:"))
		case strings.HasPrefix(line, "#"):
		default:
			ch := Channel{}
			if pending != nil {
				ch = *pending
			}
			ch.URL = line
			if ch.Group == "" {
				ch.Group = extgrp
			}
			if ch.Name == "" {
				ch.Name = line
			}
			pending, extgrp = nil, ""
			if !strings.Contains(line, "://") || isVOD(line) {
				continue
			}
			ch.ID = ids.next(ch)
			res.Channels = append(res.Channels, ch)
		}
	}
	if err := sc.Err(); err != nil {
		return res, fmt.Errorf("не удалось прочитать плейлист: %w", err)
	}
	if !sawHeader && !sawEntries {
		return res, errors.New("это не плейлист M3U — проверьте ссылку или файл")
	}
	if len(res.Channels) == 0 {
		return res, errors.New("в плейлисте не найдено ни одного телеканала")
	}
	return res, nil
}
