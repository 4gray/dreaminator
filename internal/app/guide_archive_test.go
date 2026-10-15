package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dayGuideXML is a guide like epg.one's: hourly programmes of a, b and c
// from 00:00 of the day before day to the end of day (UTC), nothing older.
func dayGuideXML(day time.Time) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv>
<channel id="a"><display-name>A</display-name></channel>
<channel id="b"><display-name>B</display-name></channel>
<channel id="c"><display-name>C</display-name></channel>
`)
	start := day.AddDate(0, 0, -1)
	for h := 0; h < 48; h++ {
		s := start.Add(time.Duration(h) * time.Hour)
		for _, ch := range []string{"a", "b", "c"} {
			fmt.Fprintf(&b, `<programme start="%s" stop="%s" channel="%s"><title>%s %s</title></programme>`+"\n",
				s.Format("20060102150405 -0700"), s.Add(time.Hour).Format("20060102150405 -0700"), ch, ch, s.Format("Jan 2 15:04"))
		}
	}
	b.WriteString("</tv>\n")
	return b.String()
}

const archivePlaylist = `#EXTM3U url-tvg="file://%DIR%/guide.xml"
#EXTINF:-1 tvg-id="a" tvg-rec="3",Архив
http://h/iptv/T/1/index.m3u8
#EXTINF:-1 tvg-id="b" catchup="shift" catchup-days="1",Сутки
http://h/b.m3u8?token=x
#EXTINF:-1 tvg-id="c" tvg-rec="0",Без архива
http://h/c.m3u8
`

func TestCatalogArchiveAcrossNights(t *testing.T) {
	dir := t.TempDir()
	guide := filepath.Join(dir, "guide.xml")
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte(strings.ReplaceAll(archivePlaylist, "%DIR%", dir)), 0o644)
	day4 := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	now := day4.AddDate(0, 0, 1).Add(4 * time.Hour) // the nightly pass of 5 October
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, ch := range a.channels["s1"] {
		ids[ch.Name] = ch.ID
	}
	// The channel with three days is in a list: its live guide is epg.xml.
	if err := a.ModifyBouquet(a.st.Bouquets[0].ID, []ChannelRef{{Source: "s1", ID: ids["Архив"]}}, nil); err != nil {
		t.Fatal(err)
	}
	night := func(at time.Time) {
		t.Helper()
		now = at
		os.WriteFile(guide, []byte(dayGuideXML(time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC))), 0o644)
		if err := a.RebuildEPG(ctx, true); err != nil {
			t.Fatal(err)
		}
	}
	night(now)
	night(day4.AddDate(0, 0, 2).Add(4 * time.Hour)) // 6 October: the guide no longer has the 4th

	now = day4.AddDate(0, 0, 2).Add(12*time.Hour + 30*time.Minute)
	cards := liveCards(t, a, "s1")
	if cards["Архив"].Archive != 3 || cards["Сутки"].Archive != 1 || cards["Без архива"].Archive != 0 {
		t.Fatalf("rows: %d %d %d", cards["Архив"].Archive, cards["Сутки"].Archive, cards["Без архива"].Archive)
	}
	dayOf := func(c CatalogCard, d int) CatalogGuide {
		t.Helper()
		g, err := a.CatalogGuide(ctx, "s1", c.Key, &d, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	g := dayOf(cards["Архив"], -2)
	if g.Archive != 3 || fmt.Sprint(g.Days) != "[-2 -1 0]" || len(g.Programmes) != 24 || g.Programmes[0].Title != "a Oct 4 00:00" || !g.Programmes[0].Rec {
		t.Fatalf("4 October kept from the first night: archive %d days %v, %d programmes %+v", g.Archive, g.Days, len(g.Programmes), g.Programmes)
	}
	g = dayOf(cards["Архив"], 0)
	if len(g.Programmes) != 24 || !g.Programmes[12].Rec || g.Programmes[13].Rec || g.Now == nil || !g.Now.Rec || g.Next.Rec {
		t.Fatalf("today: on air plays from its start, the future does not: %+v now=%+v", g.Programmes, g.Now)
	}
	// One day deep: the 4th was dropped on the second night.
	if g := dayOf(cards["Сутки"], -2); len(g.Programmes) != 0 || fmt.Sprint(g.Days) != "[-1 0]" {
		t.Fatalf("one-day archive: days %v, %d programmes", g.Days, len(g.Programmes))
	}
	// Nothing deeper than the archive is shown; the one begun just before it is grey.
	if g := dayOf(cards["Сутки"], -1); len(g.Programmes) != 12 || g.Programmes[0].Title != "b Oct 5 12:00" || g.Programmes[0].Rec || !g.Programmes[1].Rec {
		t.Fatalf("one day back from 12:30: %+v", g.Programmes)
	}
	if g := dayOf(cards["Без архива"], -1); g.Archive != 0 || len(g.Programmes) != 0 {
		t.Fatalf("no archive, no past: %+v", g)
	}

	play := func(c CatalogCard, start time.Time) (ArchivePlayback, error) {
		return a.CatalogPlayArchive(ctx, "s1", c.Key, c.ID, start.Unix())
	}
	at := day4.Add(10 * time.Hour)
	p, err := play(cards["Архив"], at)
	if err != nil || p.URL != fmt.Sprintf("http://h/iptv/T/1/index.m3u8?utc=%d&lutc=%d", at.Unix(), now.Unix()) ||
		p.Archive.Title != "a Oct 4 10:00" || p.Name != "Архив" {
		t.Fatalf("recording = %+v, %v", p, err)
	}
	onAir := day4.AddDate(0, 0, 2).Add(12 * time.Hour)
	if p, err := play(cards["Сутки"], onAir); err != nil || p.URL != fmt.Sprintf("http://h/b.m3u8?token=x&utc=%d&lutc=%d", onAir.Unix(), now.Unix()) {
		t.Fatalf("from the start of the one on air = %+v, %v", p, err)
	}
	for name, start := range map[string]time.Time{
		"future":        onAir.Add(time.Hour),
		"not a start":   at.Add(time.Minute),
		"too old":       day4.Add(10 * time.Hour),
		"no archive":    at.AddDate(0, 0, 1),
		"archive by id": at,
	} {
		c := cards["Сутки"]
		switch name {
		case "no archive":
			c = cards["Без архива"]
		case "archive by id":
			c = cards["Архив"]
			c.Key = "gone"
		}
		_, err := play(c, start)
		if name == "archive by id" {
			if err != nil {
				t.Errorf("by persistent id: %v", err)
			}
			continue
		}
		if !errors.Is(err, ErrCatalogArchive) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Recordings go to the recents like the channel.
	if rows, _ := a.CatalogHomeItems("recent", "itv", 1); len(rows.Items) == 0 || rows.Items[0].Name != "Архив" || rows.Items[0].Archive != 3 {
		t.Fatalf("recents: %+v", rows.Items)
	}

	// A restart reads the archive back; a night whose guide fails keeps it.
	b := guideTestApp(t, dir, &now)
	b.LoadGuides()
	if g, _ := b.CatalogGuide(ctx, "s1", cards["Архив"].Key, intp(-2), time.UTC); len(g.Programmes) != 24 {
		t.Fatalf("after restart: %d programmes", len(g.Programmes))
	}
	os.Remove(guide)
	now = day4.AddDate(0, 0, 3).Add(4 * time.Hour)
	a.RebuildEPG(ctx, true)
	// Three days back from 04:00 on the 7th: the 4th from 04:00.
	if g := dayOf(cards["Архив"], -3); len(g.Programmes) != 20 || g.Programmes[0].Title != "a Oct 4 04:00" || !g.Programmes[0].Rec {
		t.Fatalf("failed night: %d programmes %+v", len(g.Programmes), g.Programmes)
	}
	if g := dayOf(cards["Сутки"], -2); len(g.Programmes) != 0 {
		t.Fatalf("the failed night still trims to the depth: %+v", g.Programmes)
	}
	// The day since the last good pass comes from that pass's live indexes:
	// epg.xml for the listed channel, the playlists index for the other.
	if g := dayOf(cards["Сутки"], -1); len(g.Programmes) != 20 || g.Programmes[0].Title != "b Oct 6 04:00" || g.Programmes[19].Title != "b Oct 6 23:00" {
		t.Fatalf("failed night, playlist channel: %+v", g.Programmes)
	}
	if g := dayOf(cards["Архив"], -1); len(g.Programmes) != 24 {
		t.Fatalf("failed night, listed channel: %d programmes", len(g.Programmes))
	}
	if st := a.GuideStats(); st["archiveChannels"] != 2 {
		t.Fatalf("debug: %+v", st)
	}

	// A playlist that stops announcing the archive takes the channel's past with it.
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte(strings.ReplaceAll(strings.ReplaceAll(archivePlaylist, "%DIR%", dir), ` tvg-rec="3"`, "")), 0o644)
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if g := dayOf(cards["Архив"], -1); g.Archive != 0 || len(g.Programmes) != 0 {
		t.Fatalf("archive withdrawn: %+v", g)
	}
	if _, err := play(cards["Архив"], at.AddDate(0, 0, 1)); !errors.Is(err, ErrCatalogArchive) {
		t.Fatalf("play withdrawn archive: %v", err)
	}
}

func intp(n int) *int { return &n }

// A listed channel four hours behind its guide: at a 04:30 pass the guide's
// 05:00 programme aired at 01:00. It is too old for epg.xml (cut two hours
// before the pass) and was still "ahead" in the guide's own times.
func TestCatalogArchiveNegativeShift(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "guide.xml"), []byte(dayGuideXML(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))), 0o644)
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte("#EXTM3U url-tvg=\"file://"+dir+"/guide.xml\"\n"+
		"#EXTINF:-1 tvg-id=\"a\" tvg-shift=\"-4\" tvg-rec=\"3\",Минус четыре\nhttp://h/m4.m3u8\n"), 0o644)
	now := time.Date(2026, 10, 5, 4, 30, 0, 0, time.UTC)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	ch := a.channels["s1"][0]
	if err := a.ModifyBouquet(a.st.Bouquets[0].ID, []ChannelRef{{Source: "s1", ID: ch.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	c := liveCards(t, a, "s1")["Минус четыре"]
	g, err := a.CatalogGuide(ctx, "s1", c.Key, intp(0), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	aired := time.Date(2026, 10, 5, 1, 0, 0, 0, time.UTC).Unix()
	var found *GuideEntry
	for i, p := range g.Programmes {
		if p.Start == aired {
			found = &g.Programmes[i]
		}
	}
	if found == nil || found.Title != "a Oct 5 05:00" || !found.Rec {
		t.Fatalf("01:00 on the shifted channel: %+v in %+v", found, g.Programmes)
	}
	if _, err := a.CatalogPlayArchive(ctx, "s1", c.Key, c.ID, aired); err != nil {
		t.Fatalf("play: %v", err)
	}
}

// Off for two days: the programmes between the last pass and the oldest day
// of the guide are only in the live indexes kept on disk.
func TestCatalogArchiveAfterDaysOff(t *testing.T) {
	dir := t.TempDir()
	guide := filepath.Join(dir, "guide.xml")
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte("#EXTM3U url-tvg=\"file://"+dir+"/guide.xml\"\n"+
		"#EXTINF:-1 tvg-id=\"a\" tvg-rec=\"3\",В списке\nhttp://h/a.m3u8\n"+
		"#EXTINF:-1 tvg-id=\"b\" tvg-rec=\"3\",Не в списке\nhttp://h/b.m3u8\n"), 0o644)
	day5 := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	now := day5.Add(4 * time.Hour)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, ch := range a.channels["s1"] {
		ids[ch.Name] = ch.ID
	}
	if err := a.ModifyBouquet(a.st.Bouquets[0].ID, []ChannelRef{{Source: "s1", ID: ids["В списке"]}}, nil); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(guide, []byte(dayGuideXML(day5)), 0o644)
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	// The same playlist again: nothing for the TV to receive.
	a.st.Dirty = false
	if err := a.RefreshSource(ctx, "s1"); err != nil || a.st.Dirty {
		t.Fatalf("unchanged playlist with an archive marked the lists changed (%v)", err)
	}

	now = day5.AddDate(0, 0, 2).Add(4 * time.Hour) // back on the 7th; the guide has the 6th and the 7th
	b := guideTestApp(t, dir, &now)
	b.LoadGuides()
	os.WriteFile(guide, []byte(dayGuideXML(day5.AddDate(0, 0, 2))), 0o644)
	if err := b.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	cards := liveCards(t, b, "s1")
	for _, name := range []string{"В списке", "Не в списке"} {
		g, err := b.CatalogGuide(ctx, "s1", cards[name].Key, intp(-2), time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		// All of 5 October: three days back from 04:00 on the 7th.
		if len(g.Programmes) != 24 || !strings.HasSuffix(g.Programmes[23].Title, "Oct 5 23:00") {
			t.Errorf("%s, 5 October: %d programmes %+v", name, len(g.Programmes), g.Programmes)
		}
	}
}

// A guide cut off halfway: its first hours win, the rest of the day comes
// from the previous pass's live index instead of being lost.
func TestCatalogArchiveTruncatedGuide(t *testing.T) {
	dir := t.TempDir()
	guide := filepath.Join(dir, "guide.xml")
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte("#EXTM3U url-tvg=\"file://"+dir+"/guide.xml\"\n"+
		"#EXTINF:-1 tvg-id=\"b\" tvg-rec=\"3\",Не в списке\nhttp://h/b.m3u8\n"), 0o644)
	day5 := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	now := day5.Add(4 * time.Hour)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(guide, []byte(dayGuideXML(day5)), 0o644)
	a.RebuildEPG(ctx, true)
	now = day5.AddDate(0, 0, 1).Add(4 * time.Hour)
	full := dayGuideXML(day5.AddDate(0, 0, 1))
	os.WriteFile(guide, []byte(full[:len(full)/4]), 0o644) // 5 October up to about noon
	a.RebuildEPG(ctx, true)
	g, err := a.CatalogGuide(ctx, "s1", liveCards(t, a, "s1")["Не в списке"].Key, intp(-1), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Programmes) != 24 || g.Programmes[0].Title != "b Oct 5 00:00" || g.Programmes[23].Title != "b Oct 5 23:00" {
		t.Fatalf("5 October after a cut-off guide: %d programmes %+v", len(g.Programmes), g.Programmes)
	}
}

// A pass at 14:00 whose guide stops at 13:00: the archive keeps 13:00–14:00
// from the previous index, and the TV sees it after the live programmes.
func TestCatalogArchiveAfterShortLiveGuide(t *testing.T) {
	dir := t.TempDir()
	guide := filepath.Join(dir, "guide.xml")
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte("#EXTM3U url-tvg=\"file://"+dir+"/guide.xml\"\n"+
		"#EXTINF:-1 tvg-id=\"b\" tvg-rec=\"3\",Не в списке\nhttp://h/b.m3u8\n"), 0o644)
	day6 := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	now := day6.Add(4 * time.Hour)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	full := dayGuideXML(day6)
	os.WriteFile(guide, []byte(full), 0o644)
	a.RebuildEPG(ctx, true)
	now = day6.Add(14 * time.Hour)
	end := strings.Index(full, "b Oct 6 12:00</title></programme>\n") + len("b Oct 6 12:00</title></programme>\n")
	os.WriteFile(guide, []byte(full[:end]), 0o644)
	a.RebuildEPG(ctx, true)
	now = now.Add(30 * time.Minute)
	c := liveCards(t, a, "s1")["Не в списке"]
	g, err := a.CatalogGuide(ctx, "s1", c.Key, intp(0), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	last := g.Programmes[len(g.Programmes)-1]
	// 14:00 began after the pass: nothing knows it.
	if last.Title != "b Oct 6 13:00" || !last.Rec || len(g.Programmes) != 14 {
		t.Fatalf("today after a short live guide: %d programmes, last %+v", len(g.Programmes), last)
	}
	at := day6.Add(13 * time.Hour).Unix()
	if _, err := a.CatalogPlayArchive(ctx, "s1", c.Key, c.ID, at); err != nil {
		t.Fatalf("13:00: %v", err)
	}
}
