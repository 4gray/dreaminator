package epg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseStamp(t *testing.T) {
	for _, s := range []string{"20260927100000 +0300", "20260101000000 -0530", "20240229235959 +0000", "19991231120000", "20261005033000 +1400"} {
		want, ok := parseTime(s)
		got, gok := parseStamp([]byte(s))
		if !ok || !gok || got != want.Unix() {
			t.Errorf("%q: got %d (%v), want %d", s, got, gok, want.Unix())
		}
	}
	for _, bad := range []string{"", "2026", "2026x927100000 +0300", "20261327100000"} {
		if _, ok := parseStamp([]byte(bad)); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func writeGuide(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "epg.xml")
	if err := os.WriteFile(path, []byte(Header+body+Footer), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadGuideFromFilteredFile(t *testing.T) {
	wants := []Want{{Ref: "REF1", TvgID: "1tv"}, {Ref: "REF2", Names: []string{"Россия 1"}}}
	var out strings.Builder
	now := time.Date(2026, 9, 27, 7, 30, 0, 0, time.UTC)
	if _, err := Filter(strings.NewReader(guide), "e0.", wants, &out, now); err != nil {
		t.Fatal(err)
	}
	// A Stalker portal's guide goes into the same file.
	start := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	WriteProgramme(&out, "s1.42", start, start.Add(time.Hour), "Портал", "Строка 1\nСтрока 2 <b>")
	WriteProgramme(&out, "s1.42", start.Add(time.Hour), start.Add(2*time.Hour), "Дальше", "")
	path := writeGuide(t, out.String())

	g, err := ReadGuide(path, now.Add(-3*time.Hour), now.Add(36*time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Channels() != 3 || g.Programmes() != 4 {
		t.Fatalf("channels=%d programmes=%d", g.Channels(), g.Programmes())
	}
	p, ok := g.At("e0.1tv", time.Date(2026, 9, 27, 7, 15, 0, 0, time.UTC))
	if !ok || p.Title != "Новости & погода" || p.Stop.Sub(p.Start) != time.Hour {
		t.Fatalf("at = %+v %v", p, ok)
	}
	if d := g.Describe(p); d != "Описание" {
		t.Errorf("describe = %q", d)
	}
	if _, ok := g.At("e0.1tv", time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)); ok {
		t.Error("nothing is on air after the last programme")
	}
	p, ok = g.At("s1.42", start.Add(10*time.Minute))
	if !ok || g.Describe(p) != "Строка 1\nСтрока 2 <b>" {
		t.Errorf("portal programme = %+v, desc %q", p, g.Describe(p))
	}
	next, ok := g.After("s1.42", p.Stop)
	if !ok || next.Title != "Дальше" || g.Describe(next) != "" {
		t.Errorf("next = %+v", next)
	}
	if list := g.Between("s1.42", start.Add(30*time.Minute), start.Add(61*time.Minute)); len(list) != 2 {
		t.Errorf("between = %+v", list)
	}

	// Descriptions are read from the file the index was built from, never
	// from a newer one at other offsets.
	os.WriteFile(path, []byte(Header+Footer), 0o644)
	future := time.Now().Add(time.Hour)
	os.Chtimes(path, future, future)
	if d := g.Describe(p); d != "" {
		t.Errorf("describe after the file changed = %q", d)
	}
}

func TestReadGuideWindowWantedAndLongLines(t *testing.T) {
	base := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	var b strings.Builder
	for h := 0; h < 48; h++ {
		start := base.Add(time.Duration(h) * time.Hour)
		desc := ""
		if h == 10 {
			desc = strings.Repeat("длинное описание ", 20000) // longer than the read buffer
		}
		WriteProgramme(&b, "a", start, start.Add(time.Hour), "Передача", desc)
		WriteProgramme(&b, "b", start, start.Add(time.Hour), "Другая", "")
	}
	path := writeGuide(t, b.String())
	g, err := ReadGuide(path, base.Add(6*time.Hour), base.Add(30*time.Hour), map[string]bool{"a": true})
	if err != nil {
		t.Fatal(err)
	}
	if g.Has("b") || g.Programmes() != 24 {
		t.Fatalf("has b = %v, programmes = %d", g.Has("b"), g.Programmes())
	}
	p, ok := g.At("a", base.Add(10*time.Hour+time.Minute))
	if !ok || p.Title != "Передача" {
		t.Fatalf("long line lost: %+v", p)
	}
	if d := g.Describe(p); !strings.HasPrefix(d, "длинное описание") {
		t.Errorf("long description = %.40q", d)
	}
	if p, ok := g.At("a", base.Add(11*time.Hour+time.Minute)); !ok || !p.Start.Equal(base.Add(11*time.Hour)) {
		t.Errorf("line after the long one = %+v", p)
	}
	// Titles are stored once.
	if len(g.text) != len("Передача") {
		t.Errorf("titles not shared: %d bytes", len(g.text))
	}
}

func TestGuideBuilderTrimsOverlaps(t *testing.T) {
	b := NewGuideBuilder(time.Unix(0, 0), time.Unix(100000, 0))
	b.Add("c", 3600, 7200, "B", 0)
	b.Add("c", 0, 4000, "A", 0) // overruns B
	b.Add("c", 3600, 7200, "B again", 0)
	b.Add("c", 7200, 7200, "empty", 0)
	b.Add("c", 200000, 300000, "outside", 0)
	b.Add("c", 7200, 9000, strings.Repeat("я", 200), 0)
	g := b.Build()
	list := g.Between("c", time.Unix(0, 0), time.Unix(100000, 0))
	if len(list) != 3 || list[0].Title != "A" || !list[0].Stop.Equal(time.Unix(3600, 0)) || list[1].Title != "B" {
		t.Fatalf("list = %+v", list)
	}
	if n := len([]rune(list[2].Title)); n != maxTitle/2 {
		t.Errorf("long title kept %d runes", n)
	}
}

func TestFilterIndexesUnlistedChannels(t *testing.T) {
	now := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	idx := &Index{
		Wants: []Want{
			{Ref: "src/7", Names: []string{"Другой"}},
			{Ref: "src/8", TvgID: "1tv", Shift: 2 * time.Hour},
			{Ref: "src/9", Names: []string{"Нет такого"}},
		},
		Builder: NewGuideBuilder(now.Add(-2*time.Hour), now.Add(36*time.Hour)),
	}
	var out strings.Builder
	res, err := FilterIndex(strings.NewReader(guide), "e0.", []Want{{Ref: "REF1", TvgID: "1tv"}}, &out, now, idx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Programmes != 1 || strings.Contains(out.String(), "Лишнее") {
		t.Fatalf("written guide changed: %d\n%s", res.Programmes, out.String())
	}
	if ref := idx.Refs["src/7"]; ref.Channel != "e0.other" {
		t.Errorf("name match = %+v", ref)
	}
	if ref := idx.Refs["src/8"]; ref.Channel != "e0.1tv" || ref.Shift != 2*time.Hour {
		t.Errorf("tvg-id match = %+v", ref)
	}
	if _, ok := idx.Refs["src/9"]; ok {
		t.Error("unmatched channel indexed")
	}
	g := idx.Builder.Build()
	if p, ok := g.At("e0.other", time.Date(2026, 9, 27, 7, 30, 0, 0, time.UTC)); !ok || p.Title != "Лишнее" {
		t.Errorf("indexed programme = %+v", p)
	}
	if g.Programmes() != 2 || g.Describe(Programme{}) != "" {
		t.Errorf("programmes = %d (the old one is outside the window)", g.Programmes())
	}
}

func TestGuideEncodeDecode(t *testing.T) {
	b := NewGuideBuilder(time.Unix(0, 0), time.Unix(100000, 0))
	b.Add("e0.b", 7200, 9000, "Вторая", 0)
	b.Add("e0.a", 0, 3600, "Первая", 0)
	b.Add("e0.a", 3600, 7200, "Вторая", 0)
	g := b.Build()
	refs := map[string]GuideRef{"src/1": {Channel: "e0.a"}, "src/2": {Channel: "e0.b", Shift: time.Hour}}
	var buf strings.Builder
	if err := g.Encode(&buf, refs); err != nil {
		t.Fatal(err)
	}
	back, backRefs, err := DecodeGuide(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if back.Programmes() != 3 || backRefs["src/2"].Shift != time.Hour || !back.From.Equal(g.From) || !back.To.Equal(g.To) {
		t.Fatalf("decoded = %d programmes, refs %+v", back.Programmes(), backRefs)
	}
	for _, ch := range []string{"e0.a", "e0.b"} {
		want, got := g.Between(ch, g.From, g.To), back.Between(ch, back.From, back.To)
		if len(want) != len(got) {
			t.Fatalf("%s: %+v vs %+v", ch, want, got)
		}
		for i := range want {
			if want[i].Title != got[i].Title || !want[i].Start.Equal(got[i].Start) || !want[i].Stop.Equal(got[i].Stop) {
				t.Errorf("%s[%d]: %+v vs %+v", ch, i, want[i], got[i])
			}
		}
	}
	if _, _, err := DecodeGuide(strings.NewReader(buf.String()[:buf.Len()/2])); err == nil {
		t.Error("a cut file must not decode")
	}
}

func TestReadGuideUnescapesChannelIDs(t *testing.T) {
	const odd = `<tv><channel id="A&amp;E 'us'"><display-name>A&amp;E</display-name></channel>
<programme start="20261005100000 +0000" stop="20261005110000 +0000" channel="A&amp;E 'us'"><title>Show</title></programme></tv>`
	var out strings.Builder
	now := time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC)
	res, err := Filter(strings.NewReader(odd), "e0.", []Want{{Ref: "R", TvgID: "A&E 'us'"}}, &out, now)
	if err != nil || len(res.Map["e0.A&E 'us'"]) != 1 {
		t.Fatalf("filter: %v %v", res.Map, err)
	}
	path := writeGuide(t, out.String())
	g, err := ReadGuide(path, now.Add(-time.Hour), now.Add(time.Hour), map[string]bool{"e0.A&E 'us'": true})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := g.At("e0.A&E 'us'", now); !ok || p.Title != "Show" {
		t.Fatalf("escaped id lost: %+v, channels %d", p, g.Channels())
	}
}
