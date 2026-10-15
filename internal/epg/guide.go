package epg

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"errors"
	"html"
	"io"
	"os"
	"sort"
	"time"
	"unicode/utf8"
)

// Programme is one entry of a guide as the TV shows it.
type Programme struct {
	Start, Stop time.Time
	Title       string
	Desc        string
	pos         uint32 // where the programme sits in the guide's file (0: nowhere)
}

// maxTitle bounds a stored title; a longer one is cut at a rune boundary.
const maxTitle = 160

// Guide is a compact index of the programmes in a time window: start, stop
// and title, grouped by guide channel and sorted by start. Descriptions stay
// out of memory: a guide read from our own XMLTV file remembers where each
// programme sits, and the few descriptions the TV asks for (now and next)
// are read back from that file.
type Guide struct {
	From, To time.Time
	chans    map[string]span
	progs    []entry
	text     []byte // titles, each stored once
	file     string
	size     int64
	mod      time.Time
}

type span struct{ first, n uint32 }

type entry struct {
	start, stop uint32 // unix seconds
	title       uint32 // offset in text
	pos         uint32
	tlen        uint16
}

// GuideBuilder collects programmes in any order and builds a Guide.
type GuideBuilder struct {
	from, to int64
	ids      map[string]uint32
	names    []string
	items    []building
	titles   map[string]uint32
	text     []byte
}

type building struct {
	ch uint32
	entry
}

func NewGuideBuilder(from, to time.Time) *GuideBuilder {
	return &GuideBuilder{from: from.Unix(), to: to.Unix(), ids: map[string]uint32{}, titles: map[string]uint32{}}
}

// Add records a programme of channel ch if it overlaps the window. pos is
// its offset in the file the guide is read from, 0 when there is none.
func (b *GuideBuilder) Add(ch string, start, stop int64, title string, pos int64) bool {
	return b.add([]byte(ch), []byte(title), start, stop, pos)
}

func (b *GuideBuilder) inWindow(start, stop int64) bool {
	return stop > start && stop > b.from && start < b.to && start >= 0 && stop < 1<<32
}

// add takes bytes so a guide read line by line allocates only for channels
// and titles it has not seen yet.
func (b *GuideBuilder) add(ch, title []byte, start, stop, pos int64) bool {
	if !b.inWindow(start, stop) || len(title) == 0 || len(ch) == 0 {
		return false
	}
	id, ok := b.ids[string(ch)]
	if !ok {
		id = uint32(len(b.names))
		name := string(ch)
		b.ids[name] = id
		b.names = append(b.names, name)
	}
	if len(title) > maxTitle {
		cut := maxTitle
		for cut > 0 && !utf8.RuneStart(title[cut]) {
			cut--
		}
		title = title[:cut]
	}
	off, ok := b.titles[string(title)]
	if !ok {
		off = uint32(len(b.text))
		b.text = append(b.text, title...)
		b.titles[string(title)] = off
	}
	e := entry{start: uint32(start), stop: uint32(stop), title: off, tlen: uint16(len(title))}
	if pos > 0 && pos < 1<<32 {
		e.pos = uint32(pos)
	}
	b.items = append(b.items, building{ch: id, entry: e})
	return true
}

// Build sorts what was added. Overlapping programmes (guides that repeat an
// entry or overrun the next one) are trimmed, so each channel's programmes
// follow each other.
func (b *GuideBuilder) Build() *Guide {
	items := b.items
	sort.Slice(items, func(i, j int) bool {
		if items[i].ch != items[j].ch {
			return items[i].ch < items[j].ch
		}
		return items[i].start < items[j].start
	})
	g := &Guide{From: time.Unix(b.from, 0), To: time.Unix(b.to, 0), chans: make(map[string]span, len(b.names)),
		progs: make([]entry, 0, len(items)), text: b.text}
	for i := 0; i < len(items); {
		ch, first := items[i].ch, len(g.progs)
		for ; i < len(items) && items[i].ch == ch; i++ {
			e := items[i].entry
			if n := len(g.progs); n > first && e.start < g.progs[n-1].stop {
				if e.start == g.progs[n-1].start {
					continue
				}
				g.progs[n-1].stop = e.start
			}
			g.progs = append(g.progs, e)
		}
		g.chans[b.names[ch]] = span{uint32(first), uint32(len(g.progs) - first)}
	}
	*b = GuideBuilder{}
	return g
}

func (g *Guide) list(ch string) []entry {
	if g == nil {
		return nil
	}
	s, ok := g.chans[ch]
	if !ok {
		return nil
	}
	return g.progs[s.first : s.first+s.n]
}

func (g *Guide) programme(e entry) Programme {
	return Programme{Start: time.Unix(int64(e.start), 0), Stop: time.Unix(int64(e.stop), 0),
		Title: string(g.text[e.title : e.title+uint32(e.tlen)]), pos: e.pos}
}

// Has tells whether the guide knows any programme of the channel.
func (g *Guide) Has(ch string) bool { return len(g.list(ch)) > 0 }

// At is the programme on air at t.
func (g *Guide) At(ch string, t time.Time) (Programme, bool) {
	list, u := g.list(ch), t.Unix()
	i := sort.Search(len(list), func(i int) bool { return int64(list[i].start) > u }) - 1
	if i < 0 || int64(list[i].stop) <= u {
		return Programme{}, false
	}
	return g.programme(list[i]), true
}

// After is the first programme that starts at or after t.
func (g *Guide) After(ch string, t time.Time) (Programme, bool) {
	list, u := g.list(ch), t.Unix()
	i := sort.Search(len(list), func(i int) bool { return int64(list[i].start) >= u })
	if i == len(list) {
		return Programme{}, false
	}
	return g.programme(list[i]), true
}

// Between lists the programmes that overlap [from, to).
func (g *Guide) Between(ch string, from, to time.Time) []Programme {
	list, f, t := g.list(ch), from.Unix(), to.Unix()
	i := sort.Search(len(list), func(i int) bool { return int64(list[i].stop) > f })
	var out []Programme
	for ; i < len(list) && int64(list[i].start) < t; i++ {
		out = append(out, g.programme(list[i]))
	}
	return out
}

// Extent is the time the channel's programmes cover.
func (g *Guide) Extent(ch string) (first, last time.Time, ok bool) {
	list := g.list(ch)
	if len(list) == 0 {
		return first, last, false
	}
	return time.Unix(int64(list[0].start), 0), time.Unix(int64(list[len(list)-1].stop), 0), true
}

// Channels, Programmes and Bytes describe the guide for /api/debug.
func (g *Guide) Channels() int {
	if g == nil {
		return 0
	}
	return len(g.chans)
}

func (g *Guide) Programmes() int {
	if g == nil {
		return 0
	}
	return len(g.progs)
}

// Bytes is the approximate memory the guide holds.
func (g *Guide) Bytes() int {
	if g == nil {
		return 0
	}
	n := cap(g.progs)*20 + cap(g.text)
	for ch := range g.chans {
		n += len(ch) + 48 // key, span and map overhead
	}
	return n
}

// ---------------------------------------------------------------- on disk

// savedGuide is a guide between restarts. Playlist guides come from the
// nightly pass over a provider's XMLTV and are written once per pass;
// descriptions are not kept.
type savedGuide struct {
	Version   int
	From, To  int64
	Channels  []string
	Counts    []uint32
	Starts    []uint32
	Durations []uint32
	Titles    []uint32
	Lens      []uint16
	Text      []byte
	Refs      map[string]GuideRef
}

const savedGuideVersion = 1

// Encode writes the guide and the channels that use it.
func (g *Guide) Encode(w io.Writer, refs map[string]GuideRef) error {
	type ch struct {
		name string
		s    span
	}
	chans := make([]ch, 0, len(g.chans))
	for name, s := range g.chans {
		chans = append(chans, ch{name, s})
	}
	sort.Slice(chans, func(i, j int) bool { return chans[i].s.first < chans[j].s.first })
	out := savedGuide{Version: savedGuideVersion, From: g.From.Unix(), To: g.To.Unix(), Text: g.text, Refs: refs}
	for _, c := range chans {
		out.Channels = append(out.Channels, c.name)
		out.Counts = append(out.Counts, c.s.n)
		for _, e := range g.progs[c.s.first : c.s.first+c.s.n] {
			out.Starts = append(out.Starts, e.start)
			out.Durations = append(out.Durations, e.stop-e.start)
			out.Titles = append(out.Titles, e.title)
			out.Lens = append(out.Lens, e.tlen)
		}
	}
	return gob.NewEncoder(w).Encode(&out)
}

var errSavedGuide = errors.New("сохранённая программа передач повреждена")

// DecodeGuide reads what Encode wrote.
func DecodeGuide(r io.Reader) (*Guide, map[string]GuideRef, error) {
	var in savedGuide
	if err := gob.NewDecoder(r).Decode(&in); err != nil {
		return nil, nil, err
	}
	n := len(in.Starts)
	if in.Version != savedGuideVersion || len(in.Counts) != len(in.Channels) || len(in.Durations) != n || len(in.Titles) != n || len(in.Lens) != n {
		return nil, nil, errSavedGuide
	}
	g := &Guide{From: time.Unix(in.From, 0), To: time.Unix(in.To, 0), chans: make(map[string]span, len(in.Channels)),
		progs: make([]entry, n), text: in.Text}
	for i := range g.progs {
		if int(in.Titles[i])+int(in.Lens[i]) > len(in.Text) {
			return nil, nil, errSavedGuide
		}
		g.progs[i] = entry{start: in.Starts[i], stop: in.Starts[i] + in.Durations[i], title: in.Titles[i], tlen: in.Lens[i]}
	}
	first := uint32(0)
	for i, name := range in.Channels {
		if int(first)+int(in.Counts[i]) > n {
			return nil, nil, errSavedGuide
		}
		g.chans[name] = span{first, in.Counts[i]}
		first += in.Counts[i]
	}
	return g, in.Refs, nil
}

// ---------------------------------------------------------------- our own file

// ReadGuide indexes a guide written by Filter and WriteProgramme, which put
// every programme on a line of its own (the XML escaper turns new lines in
// texts into &#xA;). Lines are cut apart without a full XML decoder: on a
// receiver that is the difference between seconds and minutes. Only the
// programmes of wanted channels (nil: all) overlapping [from, to) are kept.
func ReadGuide(path string, from, to time.Time, wanted map[string]bool) (*Guide, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	b := NewGuideBuilder(from, to)
	r := bufio.NewReaderSize(f, 128<<10)
	var offset int64
	for {
		line, err := r.ReadSlice('\n')
		pos := offset
		offset += int64(len(line))
		if ch, title, start, stop, ok := parseLine(line); ok && b.inWindow(start, stop) {
			// Channel ids are written escaped ("A&amp;E"); callers know them plain.
			if bytes.IndexByte(ch, '&') >= 0 {
				ch = []byte(html.UnescapeString(string(ch)))
			}
			if wanted == nil || wanted[string(ch)] {
				if bytes.IndexByte(title, '&') >= 0 {
					title = []byte(html.UnescapeString(string(title)))
				}
				b.add(ch, title, start, stop, pos)
			}
		}
		// A huge description: the start tag and the title came first.
		for err == bufio.ErrBufferFull {
			var more []byte
			more, err = r.ReadSlice('\n')
			offset += int64(len(more))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	g := b.Build()
	g.file, g.size, g.mod = path, st.Size(), st.ModTime()
	return g, nil
}

// Describe reads the description of p back from the guide's file. It is
// empty when the guide does not come from a file, the file has been
// replaced since, or the programme has none.
func (g *Guide) Describe(p Programme) string {
	if g == nil || g.file == "" || p.pos == 0 {
		return ""
	}
	f, err := os.Open(g.file)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || st.Size() != g.size || !st.ModTime().Equal(g.mod) {
		return ""
	}
	line, err := bufio.NewReaderSize(io.NewSectionReader(f, int64(p.pos), 256<<10), 16<<10).ReadSlice('\n')
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return ""
	}
	if desc := element(line, "desc"); desc != nil {
		return unescape(desc)
	}
	return unescape(element(line, "sub-title"))
}

var programmeTag = []byte("<programme ")

// parseLine reads one <programme> line: its channel, first title and times.
func parseLine(line []byte) (ch, title []byte, start, stop int64, ok bool) {
	if !bytes.HasPrefix(line, programmeTag) {
		return nil, nil, 0, 0, false
	}
	end := bytes.IndexByte(line, '>')
	if end < 0 {
		return nil, nil, 0, 0, false
	}
	tag := line[:end]
	var okStart, okStop bool
	start, okStart = parseStamp(attr(tag, " start=\""))
	stop, okStop = parseStamp(attr(tag, " stop=\""))
	ch = attr(tag, " channel=\"")
	title = element(line[end:], "title")
	return ch, title, start, stop, okStart && okStop && len(ch) > 0 && len(title) > 0
}

func attr(tag []byte, name string) []byte {
	i := bytes.Index(tag, []byte(name))
	if i < 0 {
		return nil
	}
	v := tag[i+len(name):]
	if j := bytes.IndexByte(v, '"'); j >= 0 {
		return v[:j]
	}
	return nil
}

// element is the text of the first <name> or <name lang="…"> in b.
func element(b []byte, name string) []byte {
	open := []byte("<" + name)
	for {
		i := bytes.Index(b, open)
		if i < 0 {
			return nil
		}
		b = b[i+len(open):]
		if len(b) > 0 && (b[0] == '>' || b[0] == ' ') {
			break
		}
	}
	gt := bytes.IndexByte(b, '>')
	if gt < 0 {
		return nil
	}
	b = b[gt+1:]
	if end := bytes.Index(b, []byte("</"+name+">")); end >= 0 {
		return b[:end]
	}
	return b // cut off by a huge line: keep what we have
}

func unescape(b []byte) string {
	if bytes.IndexByte(b, '&') < 0 {
		return string(b)
	}
	return html.UnescapeString(string(b))
}

// parseStamp reads an XMLTV time ("20260927100000 +0300"; without a zone
// it is UTC) without time.Parse, which is slow on a receiver.
func parseStamp(b []byte) (int64, bool) {
	b = bytes.TrimSpace(b)
	if len(b) < 14 {
		return 0, false
	}
	var v [6]int64
	widths := [6]int{4, 2, 2, 2, 2, 2}
	p := 0
	for i, w := range widths {
		for k := 0; k < w; k++ {
			c := b[p]
			if c < '0' || c > '9' {
				return 0, false
			}
			v[i] = v[i]*10 + int64(c-'0')
			p++
		}
	}
	y, mo, d, h, mi, s := v[0], v[1], v[2], v[3], v[4], v[5]
	if mo < 1 || mo > 12 || d < 1 || d > 31 || h > 24 || mi > 59 || s > 60 {
		return 0, false
	}
	unix := daysFromCivil(y, mo, d)*86400 + h*3600 + mi*60 + s
	rest := bytes.TrimSpace(b[14:])
	if len(rest) >= 5 && (rest[0] == '+' || rest[0] == '-') {
		var z int64
		for _, c := range rest[1:5] {
			if c < '0' || c > '9' {
				return unix, true
			}
			z = z*10 + int64(c-'0')
		}
		off := (z/100*60 + z%100) * 60
		if rest[0] == '+' {
			unix -= off
		} else {
			unix += off
		}
	}
	return unix, true
}

// daysFromCivil counts days since 1970-01-01 (H. Hinnant's algorithm).
func daysFromCivil(y, m, d int64) int64 {
	if m <= 2 {
		y--
	}
	era := y / 400
	if y < 0 && y%400 != 0 {
		era--
	}
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}
