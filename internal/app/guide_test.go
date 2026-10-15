package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dreaminator/internal/epg"
	"dreaminator/internal/stalker/stalkertest"
)

func guideTestApp(t *testing.T, dir string, now *time.Time) *App {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, "epgimport"), 0o755)
	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: filepath.Join(dir, "data"), E2Dir: filepath.Join(dir, "e2"),
		EPGImportDir: filepath.Join(dir, "epgimport"), PublicURL: "http://127.0.0.1:8088", Port: "8088"},
		defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if now != nil {
		a.clock = func() time.Time { return *now }
	}
	return a
}

// guideXML has hourly programmes for one, two and three from 06:00 on
// 5 October to 18:00 on 6 October (UTC); only "one" has descriptions.
func guideXML() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv>
<channel id="one"><display-name>One</display-name></channel>
<channel id="two"><display-name>Два</display-name></channel>
<channel id="three"><display-name>Три</display-name></channel>
`)
	start := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	for h := 0; h < 36; h++ {
		s := start.Add(time.Duration(h) * time.Hour)
		for _, ch := range []string{"one", "two", "three"} {
			fmt.Fprintf(&b, `<programme start="%s" stop="%s" channel="%s"><title>%s %s</title>`,
				s.Format("20060102150405 -0700"), s.Add(time.Hour).Format("20060102150405 -0700"), ch, ch, s.Format("Jan 2 15:04"))
			if ch == "one" {
				fmt.Fprintf(&b, `<desc>Описание %s &amp; подробности</desc>`, s.Format("15:04"))
			}
			b.WriteString("</programme>\n")
		}
	}
	b.WriteString("</tv>\n")
	return b.String()
}

const guidePlaylist = `#EXTM3U url-tvg="file://%DIR%/guide.xml"
#EXTINF:-1 tvg-id="one",Один
http://h/1.ts
#EXTINF:-1,Два HD
http://h/2.ts
#EXTINF:-1 tvg-id="two" tvg-shift="2",Два +2
http://h/2p.ts
#EXTINF:-1,Без программы
http://h/none.ts
`

func liveCards(t *testing.T, a *App, id string) map[string]CatalogCard {
	t.Helper()
	page, err := a.CatalogItems(context.Background(), id, "itv", "", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]CatalogCard{}
	for _, c := range a.CatalogCards(id, page).Items {
		out[c.Name] = c
	}
	return out
}

func nowTitle(c CatalogCard) string {
	if c.Now == nil {
		return ""
	}
	return c.Now.Title
}

func TestCatalogGuideListsAndPlaylists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "guide.xml"), []byte(guideXML()), 0o644)
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte(strings.ReplaceAll(guidePlaylist, "%DIR%", dir)), 0o644)
	now := time.Date(2026, 10, 5, 12, 10, 0, 0, time.UTC)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	var one ChannelRef
	for _, ch := range a.channels["s1"] {
		if ch.Name == "Один" {
			one = ChannelRef{Source: "s1", ID: ch.ID}
		}
	}
	if err := a.ModifyBouquet(a.st.Bouquets[0].ID, []ChannelRef{one}, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	if st := a.View().EPG; st.Matched != 1 {
		t.Fatalf("epg = %+v", st)
	}

	cards := liveCards(t, a, "s1")
	if got := nowTitle(cards["Один"]); got != "one Oct 5 12:00" {
		t.Errorf("listed channel now = %q", got)
	}
	if c := cards["Один"]; c.Now == nil || c.Now.Start != time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Unix() || c.Now.Desc != "" {
		t.Errorf("row stays light: %+v", c.Now)
	}
	if got := nowTitle(cards["Два HD"]); got != "two Oct 5 12:00" {
		t.Errorf("playlist channel (by name) now = %q", got)
	}
	// "+2": the original's programme of 10:00 is on at 12:00.
	if c := cards["Два +2"]; nowTitle(c) != "two Oct 5 10:00" || c.Now.Start != time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Unix() {
		t.Errorf("timeshift channel now = %+v", c.Now)
	}
	if c := cards["Без программы"]; c.Now != nil {
		t.Errorf("unknown programme invented: %+v", c.Now)
	}

	g, err := a.CatalogGuide(ctx, "s1", cards["Один"].Key, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Origin != "lists" || g.Now == nil || g.Now.Desc != "Описание 12:00 & подробности" || g.Next == nil || g.Next.Title != "one Oct 5 13:00" ||
		g.Next.Desc != "Описание 13:00 & подробности" || g.Programmes != nil || fmt.Sprint(g.Days) != "[0 1]" {
		t.Fatalf("brief = %+v now=%+v next=%+v", g, g.Now, g.Next)
	}
	day := 1
	g, _ = a.CatalogGuide(ctx, "s1", cards["Один"].Key, &day, time.UTC)
	if g.Date != "2026-10-06" || len(g.Programmes) != 18 || g.Programmes[0].Title != "one Oct 6 00:00" || g.Programmes[0].Desc != "" {
		t.Fatalf("tomorrow = %s, %d programmes, first %+v", g.Date, len(g.Programmes), g.Programmes)
	}
	// The TV's zone decides where days begin (UTC+3: today ends at 21:00 UTC;
	// epg.xml starts two hours before the build).
	day = 0
	g, _ = a.CatalogGuide(ctx, "s1", cards["Один"].Key, &day, time.FixedZone("TV", 3*3600))
	if g.Date != "2026-10-05" || len(g.Programmes) != 11 || g.Programmes[10].Title != "one Oct 5 20:00" {
		t.Fatalf("today in UTC+3 = %s, %d programmes", g.Date, len(g.Programmes))
	}
	g, _ = a.CatalogGuide(ctx, "s1", cards["Два HD"].Key, nil, nil)
	if g.Origin != "playlist" || g.Now == nil || g.Now.Title != "two Oct 5 12:00" || g.Now.Desc != "" {
		t.Fatalf("playlist brief = %+v", g)
	}
	g, err = a.CatalogGuide(ctx, "s1", cards["Без программы"].Key, &day, nil)
	if err != nil || g.Now != nil || g.Next != nil || len(g.Days) != 0 || len(g.Programmes) != 0 {
		t.Fatalf("no programme = %+v, %v", g, err)
	}
	if _, err := a.CatalogGuide(ctx, "s1", "expired", nil, nil); err != ErrCatalogExpired {
		t.Fatalf("expired key: %v", err)
	}
	day = 99
	if _, err := a.CatalogGuide(ctx, "s1", cards["Один"].Key, &day, nil); err != ErrCatalogInput {
		t.Fatalf("day 99: %v", err)
	}

	rows, err := a.CatalogNow("s1", []string{cards["Один"].Key, cards["Без программы"].Key, "bogus"})
	if err != nil || len(rows) != 1 || rows[cards["Один"].Key].Title != "one Oct 5 12:00" {
		t.Fatalf("now = %+v, %v", rows, err)
	}

	// A restart keeps both indexes: epg.xml with its map, playlists on disk.
	b := guideTestApp(t, dir, &now)
	b.LoadGuides()
	cards = liveCards(t, b, "s1")
	if nowTitle(cards["Один"]) != "one Oct 5 12:00" || nowTitle(cards["Два HD"]) != "two Oct 5 12:00" {
		t.Fatalf("after restart: %q, %q", nowTitle(cards["Один"]), nowTitle(cards["Два HD"]))
	}
	// Builds made before the map existed: EPGImport's channel file answers.
	os.Remove(b.guideMapPath())
	c := guideTestApp(t, dir, &now)
	c.LoadGuides()
	if got := nowTitle(liveCards(t, c, "s1")["Один"]); got != "one Oct 5 12:00" {
		t.Fatalf("from channels.xml: %q", got)
	}

	// A day later without a new build the index is read again for the new window.
	now = now.Add(26 * time.Hour)
	liveCards(t, a, "s1")
	a.Wait()
	a.guide.mu.Lock()
	from := a.guide.lists.From
	a.guide.mu.Unlock()
	if !from.Equal(now.Add(-guideBehind)) {
		t.Fatalf("lists index not re-read: from %v", from)
	}
	if got := nowTitle(liveCards(t, a, "s1")["Один"]); got != "one Oct 6 14:00" {
		t.Errorf("next day now = %q", got)
	}
}

func TestCatalogGuideXtreamShortEPG(t *testing.T) {
	now := time.Date(2026, 10, 5, 18, 10, 0, 0, time.UTC)
	var hits atomic.Int32
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("action") {
		case "":
			fmt.Fprint(w, `{"user_info":{"auth":1,"status":"Active"}}`)
		case "get_live_categories":
			fmt.Fprint(w, `[{"category_id":"1","category_name":"Новости"}]`)
		case "get_live_streams":
			fmt.Fprint(w, `[{"stream_id":7,"name":"Семь","category_id":"1"},{"stream_id":8,"name":"Восемь","category_id":"1"},{"stream_id":9,"name":"Девять","category_id":"1"}]`)
		case "get_short_epg":
			hits.Add(1)
			start := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)
			switch q.Get("stream_id") {
			case "7":
				fmt.Fprintf(w, `{"epg_listings":[{"title":"%s","description":"%s","start_timestamp":"%d","stop_timestamp":"%d"},`+
					`{"title":"%s","description":"%s","start_timestamp":"%d","stop_timestamp":"%d"}]}`,
					b64("Новости"), b64("Главное за день"), start.Unix(), start.Add(time.Hour).Unix(),
					b64("Кино"), b64("Фильм вечера"), start.Add(time.Hour).Unix(), start.Add(3*time.Hour).Unix())
			case "8":
				fmt.Fprint(w, `{"epg_listings":[]}`)
			default:
				http.Error(w, "boom", http.StatusInternalServerError)
			}
		}
	}))
	defer panel.Close()
	a := guideTestApp(t, t.TempDir(), &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "x1", Type: "xtream", Server: panel.URL, Username: "u", Password: "p"})
	if err := a.RefreshSource(ctx, "x1"); err != nil {
		t.Fatal(err)
	}
	cards := liveCards(t, a, "x1")
	if cards["Семь"].Now != nil {
		t.Fatal("rows never wait for the provider")
	}
	g, err := a.CatalogGuide(ctx, "x1", cards["Семь"].Key, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g.Origin != "provider" || g.Now == nil || g.Now.Title != "Новости" || g.Now.Desc != "Главное за день" ||
		g.Next == nil || g.Next.Desc != "Фильм вечера" || hits.Load() != 1 {
		t.Fatalf("short guide = %+v now=%+v next=%+v hits=%d", g, g.Now, g.Next, hits.Load())
	}
	// Kept for 15 minutes: the day view and the rows reuse it.
	day := 0
	g, _ = a.CatalogGuide(ctx, "x1", cards["Семь"].Key, &day, time.UTC)
	if len(g.Programmes) != 2 || hits.Load() != 1 {
		t.Fatalf("day = %+v, hits %d", g.Programmes, hits.Load())
	}
	if c := liveCards(t, a, "x1")["Семь"]; nowTitle(c) != "Новости" || c.Now.Desc != "" {
		t.Errorf("row after the guide was asked = %+v", c.Now)
	}
	if rows, _ := a.CatalogNow("x1", []string{cards["Семь"].Key}); rows[cards["Семь"].Key].Desc != "" || g.Programmes[0].Desc != "" {
		t.Errorf("descriptions leak into rows: %+v, day %+v", rows, g.Programmes)
	}
	now = now.Add(16 * time.Minute)
	a.CatalogGuide(ctx, "x1", cards["Семь"].Key, nil, nil)
	if hits.Load() != 2 {
		t.Errorf("stale short guide reused: %d", hits.Load())
	}
	// No programme is an answer too, and it is not asked again.
	g, err = a.CatalogGuide(ctx, "x1", cards["Восемь"].Key, nil, nil)
	a.CatalogGuide(ctx, "x1", cards["Восемь"].Key, nil, nil)
	if err != nil || g.Now != nil || len(g.Days) != 0 || hits.Load() != 3 {
		t.Fatalf("empty guide = %+v, %v, hits %d", g, err, hits.Load())
	}
	if _, err := a.CatalogGuide(ctx, "x1", cards["Девять"].Key, nil, nil); err != errGuideProvider {
		t.Fatalf("provider failure: %v", err)
	}
}

func TestCatalogGuideStalkerShortEPG(t *testing.T) {
	portal := &stalkertest.Portal{Path: "/stalker_portal/server/load.php", Channels: []stalkertest.Channel{
		{ID: "101", Name: "Первый канал", Genre: "Федеральные", Stream: "http://cdn.example/1tv.ts"},
		{ID: "102", Name: "Без программы", Genre: "Федеральные", Stream: "http://cdn.example/x.ts", NoGuide: true},
	}}
	srv := httptest.NewServer(portal)
	defer srv.Close()
	a := guideTestApp(t, t.TempDir(), nil)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "st1", Type: "stalker", Server: srv.URL + "/stalker_portal/c/", MAC: "00:1A:79:12:34:56"})
	if err := a.RefreshSource(ctx, "st1"); err != nil {
		t.Fatal(err)
	}
	cards := liveCards(t, a, "st1")
	g, err := a.CatalogGuide(ctx, "st1", cards["Первый канал"].Key, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hour := time.Now().Truncate(time.Hour)
	if g.Now == nil || g.Now.Title != "Первый канал: передача в "+hour.Format("15:04") || !strings.HasPrefix(g.Now.Desc, "Описание передачи") ||
		g.Next == nil || g.Next.Start != hour.Add(time.Hour).Unix() || portal.ShortGuides != 1 {
		t.Fatalf("portal short guide = %+v now=%+v next=%+v (%d calls)", g, g.Now, g.Next, portal.ShortGuides)
	}
	a.CatalogGuide(ctx, "st1", cards["Первый канал"].Key, nil, nil)
	g, err = a.CatalogGuide(ctx, "st1", cards["Без программы"].Key, nil, nil)
	if err != nil || g.Now != nil || portal.ShortGuides != 2 {
		t.Fatalf("no guide = %+v, %v (%d calls)", g, err, portal.ShortGuides)
	}
}

func TestShortGuideBudget(t *testing.T) {
	a := guideTestApp(t, t.TempDir(), nil)
	big := strings.Repeat("я", 4000)
	for i := 0; i < 400; i++ {
		a.keepShortGuide(fmt.Sprintf("s/%d", i), []epg.Programme{{Title: "x", Desc: big}})
	}
	if a.guide.shortBytes > shortGuideBytes || len(a.guide.short) > 200 || a.guide.short["s/399"] == nil {
		t.Fatalf("short guides: %d bytes in %d entries", a.guide.shortBytes, len(a.guide.short))
	}
}

func TestIndexOnlyGuideFailureIsNotAnEPGError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte("#EXTM3U url-tvg=\"file://"+dir+"/missing.xml\"\n#EXTINF:-1,Один\nhttp://h/1.ts\n"), 0o644)
	a := guideTestApp(t, dir, nil)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatalf("index-only guide failure surfaced: %v", err)
	}
	if st := a.View().EPG; st.Error != "" {
		t.Fatalf("epg status = %+v", st)
	}
}

func TestGuideReadOfAReplacedFileIsDropped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "guide.xml"), []byte(guideXML()), 0o644)
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte(strings.ReplaceAll(guidePlaylist, "%DIR%", dir)), 0o644)
	now := time.Date(2026, 10, 5, 12, 10, 0, 0, time.UTC)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	if got := nowTitle(liveCards(t, a, "s1")["Два HD"]); got != "two Oct 5 12:00" {
		t.Fatalf("playlist channel = %q", got)
	}
	// A read that started before epg.xml was replaced never lands.
	gen, ok := a.startListsLoad(false)
	if !ok {
		t.Fatal("no read under way, one must start")
	}
	if _, again := a.startListsLoad(false); again {
		t.Fatal("two reads at once")
	}
	a.guideReplacing()
	a.guide.mu.Lock()
	before := a.guide.lists
	a.guide.mu.Unlock()
	a.loadListsGuide(gen)
	a.guide.mu.Lock()
	after, running := a.guide.lists, a.guide.listsRunning
	a.guide.mu.Unlock()
	if after != before || running != 0 {
		t.Fatalf("stale read landed (%v) or still counted (%d)", after != before, running)
	}

	// The playlist's guide is down for a night: its channels keep the
	// programmes of the last pass instead of going blank.
	os.Rename(filepath.Join(dir, "guide.xml"), filepath.Join(dir, "guide.off"))
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatalf("a guide read only for the catalog is not an EPG error: %v", err)
	}
	if got := nowTitle(liveCards(t, a, "s1")["Два HD"]); got != "two Oct 5 12:00" {
		t.Fatalf("after a failed night = %q", got)
	}
	os.Rename(filepath.Join(dir, "guide.off"), filepath.Join(dir, "guide.xml"))
	a.RebuildEPG(ctx, true)
	a.RebuildEPG(ctx, true)
	if ref := a.guide.playlistRefs[a.st.Sources[0].ID+"/"+cardID(t, a, "Два HD")]; strings.HasPrefix(ref.Channel, "kept.") {
		t.Fatalf("a working guide still serves kept programmes: %+v", ref)
	}
}

func cardID(t *testing.T, a *App, name string) string {
	t.Helper()
	for _, ch := range a.channels["s1"] {
		if ch.Name == name {
			return ch.ID
		}
	}
	t.Fatalf("no channel %q", name)
	return ""
}
