// Package epg shrinks a provider's XMLTV guide down to the channels that are
// actually in the user's bouquets, so EPGImport on a slow receiver only has a
// few megabytes to chew through.
package epg

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	Header = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tv generator-info-name=\"dreaminator\">\n"
	Footer = "</tv>\n"
)

type Want struct {
	Ref   string // Enigma2 EPG reference of our service
	TvgID string
	Names []string      // display name, tvg-name…: any of them may match
	Shift time.Duration // tvg-shift: move the guide for timeshift channels
}

// variant is one output channel made from an XMLTV channel: timeshift
// channels sharing a guide get their own copy with moved times.
type variant struct {
	oid   string
	shift time.Duration
}

func shiftSuffix(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return fmt.Sprintf("@%+g", d.Hours())
}

// shiftTime moves an XMLTV timestamp, keeping its timezone.
func shiftTime(s string, d time.Duration) string {
	if d == 0 {
		return s
	}
	t, ok := parseTime(s)
	if !ok {
		return s
	}
	return t.Add(d).Format("20060102150405 -0700")
}

type Result struct {
	Map        map[string][]string // output channel id -> refs
	Channels   int
	Programmes int
}

var noise = map[string]bool{"hd": true, "fhd": true, "uhd": true, "sd": true, "4k": true, "orig": true, "hevc": true, "h265": true}

// qualityTag matches "(1080p)", "[Not 24/7]", "[Geo-blocked]" and similar
// annotations some playlists append to channel names.
var qualityTag = regexp.MustCompile(`\[[^\]]*\]|\(\s*\d{3,4}[pi]\s*\)|\b\d{3,4}[pi]\b`)

// Normalize makes channel names comparable: "Первый канал HD" and
// "первый-канал" both become "первыйканал".
func Normalize(name string) string {
	name = qualityTag.ReplaceAllString(name, " ")
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			if t := cur.String(); !noise[t] {
				tokens = append(tokens, t)
			}
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(name) {
		switch {
		case r == 'ё':
			cur.WriteRune('е')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur.WriteRune(r)
		case r == '+':
			flush()
			tokens = append(tokens, "+")
		default:
			flush()
		}
	}
	flush()
	return strings.Join(tokens, "")
}

type xText struct {
	Lang string `xml:"lang,attr"`
	Text string `xml:",chardata"`
}

type xChannel struct {
	ID    string   `xml:"id,attr"`
	Names []string `xml:"display-name"`
}

type xProgramme struct {
	Start      string  `xml:"start,attr"`
	Stop       string  `xml:"stop,attr"`
	Titles     []xText `xml:"title"`
	SubTitles  []xText `xml:"sub-title"`
	Descs      []xText `xml:"desc"`
	Categories []xText `xml:"category"`
}

// WriteChannel and WriteProgramme emit XMLTV for guides that do not come
// from XMLTV (Stalker portals).
func WriteChannel(w io.Writer, id, name string) {
	fmt.Fprintf(w, "<channel id=\"%s\"><display-name>%s</display-name></channel>\n", esc(id), esc(name))
}

func WriteProgramme(w io.Writer, channel string, start, stop time.Time, title, desc string) {
	const layout = "20060102150405 -0700"
	fmt.Fprintf(w, `<programme start="%s" stop="%s" channel="%s">`, start.UTC().Format(layout), stop.UTC().Format(layout), esc(channel))
	writeTexts(w, "title", []xText{{Text: title}})
	writeTexts(w, "desc", []xText{{Text: desc}})
	io.WriteString(w, "</programme>\n")
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(strings.TrimSpace(s)))
	return b.String()
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"20060102150405 -0700", "20060102150405"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func writeTexts(w io.Writer, tag string, items []xText) {
	for _, t := range items {
		if strings.TrimSpace(t.Text) == "" {
			continue
		}
		if t.Lang != "" {
			fmt.Fprintf(w, `<%s lang="%s">%s</%s>`, tag, esc(t.Lang), esc(t.Text), tag)
		} else {
			fmt.Fprintf(w, `<%s>%s</%s>`, tag, esc(t.Text), tag)
		}
	}
}

// Index asks Filter to also index channels into a guide while it streams
// the file. Wants are matched like the written ones (tvg-id first, then
// names); every programme of a matched channel that overlaps the builder's
// window goes into the guide under prefix+XMLTV id, and Refs tells, for
// each Want.Ref, which guide channel it uses and with what shift.
type Index struct {
	Wants   []Want
	Builder *GuideBuilder
	Refs    map[string]GuideRef
}

// GuideRef is where a channel's programmes are in a guide: timeshift
// channels share their original's programmes and move them by Shift.
type GuideRef struct {
	Channel string
	Shift   time.Duration
}

// matcher pairs XMLTV channels with wanted ones: an exact tvg-id match
// always wins, name matching is a fallback for playlists without (or with
// foreign) tvg-ids, and every want is matched at most once.
type matcher struct {
	wants   []Want
	byID    map[string][]int
	byName  map[string][]int
	claimed map[int]bool
}

func newMatcher(wants []Want) *matcher {
	m := &matcher{wants: wants, byID: map[string][]int{}, byName: map[string][]int{}, claimed: map[int]bool{}}
	for i, wt := range wants {
		if id := strings.ToLower(strings.TrimSpace(wt.TvgID)); id != "" {
			m.byID[id] = append(m.byID[id], i)
		}
		seen := map[string]bool{}
		for _, name := range wt.Names {
			if n := Normalize(name); n != "" && !seen[n] {
				seen[n] = true
				m.byName[n] = append(m.byName[n], i)
			}
		}
	}
	return m
}

func (m *matcher) match(ch xChannel, names []string) []int {
	var matched []int
	for _, i := range m.byID[strings.ToLower(strings.TrimSpace(ch.ID))] {
		matched = append(matched, i)
		m.claimed[i] = true
	}
	for _, n := range names {
		for _, i := range m.byName[n] {
			if !m.claimed[i] {
				matched = append(matched, i)
				m.claimed[i] = true
			}
		}
	}
	return matched
}

// Filter streams an XMLTV document from r and writes the <channel> and
// <programme> elements we need to w, renaming channel ids with prefix so
// several guides can be merged into one file.
func Filter(r io.Reader, prefix string, wants []Want, w io.Writer, now time.Time) (Result, error) {
	return FilterIndex(r, prefix, wants, w, now, nil)
}

// FilterIndex is Filter that also fills the indexes in the same pass (the
// catalog's 36 hours and the archive's past days); nil ones are skipped.
func FilterIndex(r io.Reader, prefix string, wants []Want, w io.Writer, now time.Time, indexes ...*Index) (Result, error) {
	written := newMatcher(wants)
	type indexing struct {
		*Index
		m       *matcher
		collect map[string]bool // XMLTV channel ids whose programmes go into this guide
	}
	var idxs []indexing
	collect := map[string]bool{} // into any of them
	for _, idx := range indexes {
		if idx == nil {
			continue
		}
		if idx.Refs == nil {
			idx.Refs = map[string]GuideRef{}
		}
		idxs = append(idxs, indexing{idx, newMatcher(idx.Wants), map[string]bool{}})
	}

	res := Result{Map: map[string][]string{}}
	variants := map[string][]variant{} // XMLTV channel id -> output channels
	seen := map[string]bool{}
	cutoff := now.Add(-2 * time.Hour)

	d := xml.NewDecoder(r)
	d.Strict = false
	d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, fmt.Errorf("файл программы повреждён: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "tv":
			// descend into the root element
		case "channel":
			var ch xChannel
			if err := d.DecodeElement(&ch, &se); err != nil {
				return res, err
			}
			names := make([]string, len(ch.Names))
			for i, n := range ch.Names {
				names[i] = Normalize(n)
			}
			name := ch.ID
			if len(ch.Names) > 0 {
				name = ch.Names[0]
			}
			for _, i := range written.match(ch, names) {
				v := variant{oid: prefix + ch.ID + shiftSuffix(wants[i].Shift), shift: wants[i].Shift}
				if !seen[v.oid] {
					seen[v.oid] = true
					variants[ch.ID] = append(variants[ch.ID], v)
					fmt.Fprintf(w, "<channel id=\"%s\"><display-name>%s</display-name></channel>\n", esc(v.oid), esc(name))
					res.Channels++
				}
				res.Map[v.oid] = append(res.Map[v.oid], wants[i].Ref)
			}
			for _, idx := range idxs {
				for _, i := range idx.m.match(ch, names) {
					collect[ch.ID], idx.collect[ch.ID] = true, true
					idx.Refs[idx.Wants[i].Ref] = GuideRef{Channel: prefix + ch.ID, Shift: idx.Wants[i].Shift}
				}
			}
		case "programme":
			var channel string
			for _, a := range se.Attr {
				if a.Name.Local == "channel" {
					channel = a.Value
				}
			}
			vs := variants[channel]
			var into []*GuideBuilder // indexes this programme goes into
			var from, to int64
			if collect[channel] {
				// Most of a week-long guide is outside the indexes' windows:
				// the times in the start tag decide before anything is decoded.
				var startAttr, stopAttr string
				for _, a := range se.Attr {
					switch a.Name.Local {
					case "start":
						startAttr = a.Value
					case "stop":
						stopAttr = a.Value
					}
				}
				var okStart, okStop bool
				from, okStart = parseStamp([]byte(startAttr))
				to, okStop = parseStamp([]byte(stopAttr))
				for _, idx := range idxs {
					if okStart && okStop && idx.collect[channel] && idx.Builder.inWindow(from, to) {
						into = append(into, idx.Builder)
					}
				}
			}
			if len(vs) == 0 && len(into) == 0 {
				if err := d.Skip(); err != nil {
					return res, err
				}
				continue
			}
			if len(vs) == 0 {
				// Only the title goes into the indexes.
				var t struct {
					Titles []xText `xml:"title"`
				}
				if err := d.DecodeElement(&t, &se); err != nil {
					return res, err
				}
				for _, b := range into {
					b.Add(prefix+channel, from, to, firstText(t.Titles), 0)
				}
				continue
			}
			var p xProgramme
			if err := d.DecodeElement(&p, &se); err != nil {
				return res, err
			}
			for _, b := range into {
				b.Add(prefix+channel, from, to, firstText(p.Titles), 0)
			}
			stop, stopOK := parseTime(p.Stop)
			for _, v := range vs {
				if stopOK && stop.Add(v.shift).Before(cutoff) {
					continue
				}
				fmt.Fprintf(w, `<programme start="%s" stop="%s" channel="%s">`,
					esc(shiftTime(p.Start, v.shift)), esc(shiftTime(p.Stop, v.shift)), esc(v.oid))
				writeTexts(w, "title", p.Titles)
				writeTexts(w, "sub-title", p.SubTitles)
				writeTexts(w, "desc", p.Descs)
				writeTexts(w, "category", p.Categories)
				io.WriteString(w, "</programme>\n")
				res.Programmes++
			}
		default:
			if err := d.Skip(); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func firstText(items []xText) string {
	for _, t := range items {
		if s := strings.TrimSpace(t.Text); s != "" {
			return s
		}
	}
	return ""
}
