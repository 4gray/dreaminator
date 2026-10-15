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

// Filter streams an XMLTV document from r and writes the <channel> and
// <programme> elements we need to w, renaming channel ids with prefix so
// several guides can be merged into one file.
func Filter(r io.Reader, prefix string, wants []Want, w io.Writer, now time.Time) (Result, error) {
	byID := map[string][]int{}
	byName := map[string][]int{}
	for i, wt := range wants {
		if id := strings.ToLower(strings.TrimSpace(wt.TvgID)); id != "" {
			byID[id] = append(byID[id], i)
		}
		seen := map[string]bool{}
		for _, name := range wt.Names {
			if n := Normalize(name); n != "" && !seen[n] {
				seen[n] = true
				byName[n] = append(byName[n], i)
			}
		}
	}

	res := Result{Map: map[string][]string{}}
	variants := map[string][]variant{} // XMLTV channel id -> output channels
	written := map[string]bool{}
	claimed := map[int]bool{}
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
			var matched []int
			// An exact tvg-id match always wins; name matching is a fallback
			// for playlists without (or with foreign) tvg-ids.
			for _, i := range byID[strings.ToLower(strings.TrimSpace(ch.ID))] {
				matched = append(matched, i)
				claimed[i] = true
			}
			for _, n := range ch.Names {
				for _, i := range byName[Normalize(n)] {
					if !claimed[i] {
						matched = append(matched, i)
						claimed[i] = true
					}
				}
			}
			name := ch.ID
			if len(ch.Names) > 0 {
				name = ch.Names[0]
			}
			for _, i := range matched {
				v := variant{oid: prefix + ch.ID + shiftSuffix(wants[i].Shift), shift: wants[i].Shift}
				if !written[v.oid] {
					written[v.oid] = true
					variants[ch.ID] = append(variants[ch.ID], v)
					fmt.Fprintf(w, "<channel id=\"%s\"><display-name>%s</display-name></channel>\n", esc(v.oid), esc(name))
					res.Channels++
				}
				res.Map[v.oid] = append(res.Map[v.oid], wants[i].Ref)
			}
		case "programme":
			var channel string
			for _, a := range se.Attr {
				if a.Name.Local == "channel" {
					channel = a.Value
				}
			}
			vs, ok := variants[channel]
			if !ok {
				if err := d.Skip(); err != nil {
					return res, err
				}
				continue
			}
			var p xProgramme
			if err := d.DecodeElement(&p, &se); err != nil {
				return res, err
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
