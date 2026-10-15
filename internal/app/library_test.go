package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
	"dreaminator/internal/stalker/stalkertest"
)

// libraryApp has a Stalker portal with the catalog fixture and an M3U
// playlist; t.Cleanup writes nothing behind the test's back.
func libraryApp(t *testing.T, dir string) (*App, func()) {
	t.Helper()
	portal := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Catalog: true})
	a, err := New(Config{DataDir: dir}, DefaultSettings(), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	if a.source("portal") == nil {
		a.st.Sources = append(a.st.Sources,
			&Source{ID: "portal", Name: "Portal", Type: "stalker", MAC: "00:1A:79:AA:BB:CC"},
			&Source{ID: "m3u", Name: "Cyrius", Type: "m3u", URL: "http://lists.example/tv.m3u"})
	}
	// A restarted test portal has a new address and knows no old session.
	a.source("portal").Server = portal.URL + "/c/"
	a.source("portal").Stalker = nil
	a.save()
	a.setChannels("m3u", []playlist.Channel{
		{ID: "one", Name: "Первый", Group: "Эфир", Logo: "http://logos.example/1.png", URL: "http://stream.example/1.ts"},
		{ID: "two", Name: "Второй", Group: "Эфир", URL: "http://stream.example/2.ts"},
	})
	a.mu.Unlock()
	return a, func() {
		// Write what is pending now, so no timer writes into a removed TempDir.
		a.SaveLibrary()
		portal.Close()
	}
}

func vodRoots(t *testing.T, a *App) map[string]stalker.CatalogItem {
	t.Helper()
	p, err := a.CatalogItems(context.Background(), "portal", "vod", "1", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]stalker.CatalogItem{}
	for _, item := range p.Items {
		out[item.ID] = item
	}
	return out
}

func homeItems(t *testing.T, a *App, list, category string) []CatalogCard {
	t.Helper()
	cards, err := a.CatalogHomeItems(list, category, 1)
	if err != nil {
		t.Fatal(err)
	}
	return cards.Items
}

func TestCatalogRowsCarryPersistentIDs(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	ctx := context.Background()
	roots := vodRoots(t, a)
	if got := roots["film"].CatalogID; got != "portal/vod/film" {
		t.Fatalf("film id %q", got)
	}
	seasons, err := a.CatalogItems(ctx, "portal", "vod", "1", roots["lazy"].Key, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := a.CatalogItems(ctx, "portal", "vod", "1", seasons.Items[1].Key, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := episodes.Items[1].CatalogID; got != "portal/vod/lazy/lazy:2/2" {
		t.Fatalf("episode id %q (series, season, number)", got)
	}
	live, err := a.CatalogItems(ctx, "m3u", "itv", "", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	cards := Cards(live)
	if cards.Items[0].ID != "m3u/itv/one" {
		t.Fatalf("channel card %+v", cards.Items[0])
	}
	if src, section, rest, ok := parseCatalogID(catalogID("s", "vod", "a/b")); !ok || src != "s" || section != "vod" || rest[0] != "a/b" {
		t.Fatalf("slashes in provider ids: %v %v %v", src, section, rest)
	}
	// Lean rows: the id adds a few dozen bytes per row and nothing else.
	b, _ := json.Marshal(cards.Items[0])
	if len(b) > 160 || strings.Contains(string(b), "stream.example") {
		t.Fatalf("row %s", b)
	}
}

func TestFavoriteChannelsAreTheFavoritesList(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	a.st.Dirty = false
	res, err := a.SetFavorite("m3u", "", "m3u/itv/two", true)
	if err != nil || !res.Fav || !res.Channel || res.List != "Избранное" || res.Created || res.Unsent {
		t.Fatalf("add %+v %v", res, err)
	}
	b := a.favoritesBouquetLocked()
	if len(b.Channels) != 1 || b.Channels[0] != (ChannelRef{Source: "m3u", ID: "two", Name: "Второй"}) || !a.st.Dirty {
		t.Fatalf("bouquet %+v dirty=%v", b.Channels, a.st.Dirty)
	}
	// Changes the tablet has not sent go to the TV with the next send.
	res, _ = a.SetFavorite("m3u", "", "m3u/itv/one", true)
	if !res.Unsent {
		t.Fatal("unsent tablet changes not reported")
	}
	live, _ := a.CatalogItems(context.Background(), "m3u", "itv", "", "", 1, false)
	cards := Cards(live)
	a.MarkLibrary(&cards)
	if !cards.Items[0].Fav || !cards.Items[1].Fav {
		t.Fatalf("stars %+v", cards.Items)
	}
	rows := homeItems(t, a, "favorites", "itv")
	if len(rows) != 2 || rows[0].Name != "Второй" || rows[0].Source != "m3u" || rows[0].Number != "1" || !rows[0].Fav || rows[0].Missing {
		t.Fatalf("home rows %+v", rows)
	}
	// Removing by key works like removing by id.
	if res, err := a.SetFavorite("m3u", cards.Items[0].Key, "", false); err != nil || res.Fav {
		t.Fatalf("remove %+v %v", res, err)
	}
	if len(b.Channels) != 1 || b.Channels[0].ID != "two" {
		t.Fatalf("after removal %+v", b.Channels)
	}
	// Renamed by the viewer: it is no longer "Избранное", a new one is made
	// (first, where the default list was).
	b.Name = "Мои каналы"
	res, err = a.SetFavorite("m3u", "", "m3u/itv/one", true)
	if err != nil || !res.Created || a.st.Bouquets[0].Name != "Избранное" || len(a.st.Bouquets[0].Channels) != 1 {
		t.Fatalf("renamed list: %+v %v %+v", res, err, a.st.Bouquets)
	}
	// Deleted by the viewer: made again.
	if err := a.DeleteBouquet(a.st.Bouquets[0].ID); err != nil {
		t.Fatal(err)
	}
	if res, err := a.SetFavorite("m3u", "", "m3u/itv/two", true); err != nil || !res.Created {
		t.Fatalf("deleted list: %+v %v", res, err)
	}
	if b := a.favoritesBouquetLocked(); len(b.Channels) != 1 || b.Channels[0].ID != "two" {
		t.Fatalf("recreated %+v", b)
	}
	// The list is written into the receiver's bouquets like any other.
	found := false
	for _, eb := range a.buildBouquets() {
		if eb.Name == "Избранное" && len(eb.Services) == 1 && eb.Services[0].URL == "http://stream.example/2.ts" {
			found = true
		}
	}
	if !found {
		t.Fatal("favorites list not built for Enigma2")
	}
	if _, err := a.SetFavorite("m3u", "", "m3u/itv/nope", true); err != ErrCatalogExpired {
		t.Fatalf("unknown channel: %v", err)
	}
}

func TestFilmFavoritesRecentsAndRestart(t *testing.T) {
	dir := t.TempDir()
	a, done := libraryApp(t, dir)
	ctx := context.Background()
	roots := vodRoots(t, a)
	for _, id := range []string{"film", "lazy"} {
		if res, err := a.SetFavorite("portal", roots[id].Key, "", true); err != nil || !res.Fav || res.Channel {
			t.Fatalf("favorite %s: %+v %v", id, res, err)
		}
	}
	if _, err := a.SetFavorite("portal", "", "portal/vod/lazy/lazy:1", true); err != ErrCatalogInput {
		t.Fatalf("a season as favorite: %v", err)
	}
	// Recents: a channel, the film, and for an episode its series.
	if _, err := a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/one"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CatalogPlay(ctx, "portal", roots["film"].Key); err != nil {
		t.Fatal(err)
	}
	seasons, _ := a.CatalogItems(ctx, "portal", "vod", "1", roots["embedded"].Key, 1, false)
	episodes, _ := a.CatalogItems(ctx, "portal", "vod", "1", seasons.Items[0].Key, 1, false)
	if _, err := a.CatalogPlay(ctx, "portal", episodes.Items[1].Key); err != nil {
		t.Fatal(err)
	}
	video := homeItems(t, a, "recent", "video")
	if len(video) != 2 || video[0].ID != "portal/vod/embedded" || video[0].Kind != "series" || video[1].ID != "portal/vod/film" {
		t.Fatalf("recent films %+v", video)
	}
	if a.lib.Recent[0].Episode != "portal/vod/embedded/embedded/2" {
		t.Fatalf("episode of the recent series: %q", a.lib.Recent[0].Episode)
	}
	if ch := homeItems(t, a, "recent", "itv"); len(ch) != 1 || ch[0].Name != "Первый" || !ch[0].HasPoster {
		t.Fatalf("recent channels %+v", ch)
	}
	if err := a.SaveLibrary(); err != nil {
		t.Fatal(err)
	}
	done()

	// After a restart (and so without any list open) the library still plays
	// and opens: the film by id, the series' seasons by its library key.
	b, done := libraryApp(t, dir)
	defer done()
	fav := homeItems(t, b, "favorites", "video")
	if len(fav) != 2 || fav[0].ID != "portal/vod/lazy" || fav[1].Name != "Демо фильм" || !fav[1].HasPoster || fav[1].Section != "vod" || !fav[1].Fav {
		t.Fatalf("favorites after restart %+v", fav)
	}
	play, err := b.CatalogPlayEntry(ctx, "portal", "", "portal/vod/film")
	if err != nil || !strings.Contains(play.URL, "catalog/film.mp4") {
		t.Fatalf("film by id after restart: %+v %v", play, err)
	}
	b.mu.Lock()
	delete(b.catalogs, "portal") // the 10-minute budget, a refresh…
	b.mu.Unlock()
	seasons, err = b.CatalogItems(ctx, "portal", "vod", "", fav[0].Key, 1, false)
	if err != nil || len(seasons.Items) != 2 {
		t.Fatalf("seasons of a favorite series without its list: %+v %v", seasons, err)
	}
	episodes, err = b.CatalogItems(ctx, "portal", "vod", "", seasons.Items[0].Key, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if play, err := b.CatalogPlay(ctx, "portal", episodes.Items[0].Key); err != nil || !strings.Contains(play.URL, "lazy-1-1") {
		t.Fatalf("episode of a favorite series: %+v %v", play, err)
	}
	if d, err := b.CatalogDetails(ctx, "portal", fav[1].Key); err != nil || d.Description != "Проверка воспроизведения фильма" {
		t.Fatalf("card of a favorite film: %+v %v", d, err)
	}
	if _, err := b.SetFavorite("portal", "", "portal/vod/film", false); err != nil || len(b.lib.Favorites) != 1 {
		t.Fatalf("remove film: %v %+v", err, b.lib.Favorites)
	}
}

func TestRecentsKeepThirtyNewestFirst(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	chans := []playlist.Channel{}
	for n := 0; n < 40; n++ {
		chans = append(chans, playlist.Channel{ID: fmt.Sprint(n), Name: fmt.Sprintf("Канал %d", n), URL: "http://stream.example/x.ts"})
	}
	a.setChannels("m3u", chans)
	ctx := context.Background()
	for n := 0; n < 40; n++ {
		if _, err := a.CatalogPlayEntry(ctx, "m3u", "", fmt.Sprintf("m3u/itv/%d", n)); err != nil {
			t.Fatal(err)
		}
	}
	// Watching one again moves it to the top instead of adding a copy.
	a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/20")
	rows := homeItems(t, a, "recent", "itv")
	if len(rows) != recentMax || rows[0].Name != "Канал 20" || rows[1].Name != "Канал 39" || rows[recentMax-1].Name != "Канал 10" {
		t.Fatalf("%d recents: first %q, second %q, last %q", len(rows), rows[0].Name, rows[1].Name, rows[len(rows)-1].Name)
	}
	home := a.CatalogHome()
	if home.Recent[0] != (HomeCategory{ID: "itv", Name: "Каналы", Count: recentMax}) || home.Favorites[0].Count != 0 || home.List != "Избранное" {
		t.Fatalf("home %+v", home)
	}
}

func TestDeletingASourceForgetsItsLibraryRows(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	ctx := context.Background()
	roots := vodRoots(t, a)
	a.SetFavorite("portal", roots["film"].Key, "", true)
	a.SetFavorite("m3u", "", "m3u/itv/one", true)
	a.CatalogPlay(ctx, "portal", roots["film"].Key)
	a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/two")
	if err := a.DeleteSource("portal"); err != nil {
		t.Fatal(err)
	}
	if len(a.lib.Favorites) != 0 || len(a.lib.Recent) != 1 || a.lib.Recent[0].Source != "m3u" {
		t.Fatalf("library after delete %+v %+v", a.lib.Favorites, a.lib.Recent)
	}
	if err := a.DeleteSource("m3u"); err != nil {
		t.Fatal(err)
	}
	if len(a.lib.Recent) != 0 || len(a.favoritesBouquetLocked().Channels) != 0 {
		t.Fatalf("playlist rows left: %+v %+v", a.lib.Recent, a.favoritesBouquetLocked())
	}
}

func TestChannelGoneFromPlaylistIsMissing(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	ctx := context.Background()
	a.SetFavorite("m3u", "", "m3u/itv/one", true)
	a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/one")
	old, _ := a.CatalogItems(ctx, "m3u", "itv", "", "", 1, false)
	// The provider dropped the channel, then brought the other one back under a new address.
	a.setChannels("m3u", []playlist.Channel{{ID: "two", Name: "Второй", URL: "http://stream.example/2-new.ts"}})
	for _, list := range []string{"favorites", "recent"} {
		rows := homeItems(t, a, list, "itv")
		if len(rows) != 1 || !rows[0].Missing || rows[0].Name != "Первый" || rows[0].HasPoster {
			t.Fatalf("%s: %+v", list, rows)
		}
	}
	if _, err := a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/one"); err != ErrCatalogExpired {
		t.Fatalf("missing channel plays: %v", err)
	}
	// A key from before the refresh expires; the id plays the new address.
	if _, err := a.CatalogPlay(ctx, "m3u", old.Items[1].Key); err != ErrCatalogExpired {
		t.Fatalf("old key: %v", err)
	}
	if p, err := a.CatalogPlayEntry(ctx, "m3u", old.Items[1].Key, old.Items[1].CatalogID); err != nil || p.URL != "http://stream.example/2-new.ts" {
		t.Fatalf("by id after refresh: %+v %v", p, err)
	}
	// The key wins over the id: it names exactly one row.
	fresh, _ := a.CatalogItems(ctx, "m3u", "itv", "", "", 1, false)
	if p, err := a.CatalogPlayEntry(ctx, "m3u", fresh.Items[0].Key, "m3u/itv/unknown"); err != nil || p.URL != "http://stream.example/2-new.ts" {
		t.Fatalf("key with an unknown id: %+v %v", p, err)
	}
	// Library rows never replace the fuller entry of a listed row.
	a.SetFavorite("m3u", "", "m3u/itv/two", true)
	homeItems(t, a, "favorites", "itv")
	if rec, _ := a.catalogRecordLocked("m3u", a.catalogs["m3u"], fresh.Items[0].Key); rec.item.Number != "1" {
		t.Fatalf("listed row replaced by its library copy: %+v", rec.item)
	}
	// It can still be removed from the favorites.
	if _, err := a.SetFavorite("m3u", "", "m3u/itv/one", false); err != nil || len(a.favoritesBouquetLocked().Channels) != 1 {
		t.Fatalf("remove missing: %v", err)
	}
}

func TestLibraryWritesAtMostOncePerMinuteAndSurvivesACut(t *testing.T) {
	dir := t.TempDir()
	a, done := libraryApp(t, dir)
	defer done()
	path := filepath.Join(dir, "library.json")
	ctx := context.Background()
	scheduled := func() time.Duration {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.lib.timer == nil || !a.lib.dirty {
			return -1
		}
		return time.Until(a.lib.due)
	}
	// After a quiet minute a change is written within seconds (a burst of
	// changes becomes one write).
	a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/one")
	if wait := scheduled(); wait < 0 || wait > libraryDebounce {
		t.Fatalf("first write in %v", wait)
	}
	if err := a.SaveLibrary(); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(first), "m3u/itv/one") {
		t.Fatalf("first write %s %v", first, err)
	}
	// The next change waits for the minute since that write.
	a.CatalogPlayEntry(ctx, "m3u", "", "m3u/itv/two")
	if wait := scheduled(); wait < libraryWriteEvery-5*time.Second {
		t.Fatalf("second write in %v, within the minute", wait)
	}
	if now, _ := os.ReadFile(path); string(now) != string(first) {
		t.Fatal("written before its time")
	}
	// Closing the catalog writes now; the previous version stays as .bak.
	if err := a.SaveLibrary(); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(path); !strings.Contains(string(now), "m3u/itv/two") {
		t.Fatalf("close did not write: %s", now)
	}
	if bak, _ := os.ReadFile(path + ".bak"); string(bak) != string(first) {
		t.Fatal("previous version not kept")
	}
	if scheduled() >= 0 {
		t.Fatal("a write is still scheduled with nothing to write")
	}
	// A power cut left the file cut off: the previous version is used.
	os.WriteFile(path, []byte(`{"version":1,"recent":[{"id":"m3u/it`), 0o644)
	b, doneB := libraryApp(t, dir)
	defer doneB()
	if len(b.lib.Recent) != 1 || b.lib.Recent[0].ID != "m3u/itv/one" {
		t.Fatalf("fallback library %+v", b.lib.Recent)
	}
	if matches, _ := filepath.Glob(path + ".broken-*"); len(matches) != 1 {
		t.Fatal("unreadable library not kept aside")
	}
}

func TestListedRowsRefreshTheLibraryCopy(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	roots := vodRoots(t, a)
	a.SetFavorite("portal", roots["film"].Key, "", true)
	a.lib.Favorites[0].Item.Cmd = "/media/old.mpg"
	a.lib.Favorites[0].Name = "Старое название"
	a.mu.Lock()
	delete(a.catalogs, "portal")
	a.mu.Unlock()
	vodRoots(t, a)
	if e := a.lib.Favorites[0]; e.Item.Cmd != "/media/file_film.mpg" || e.Name != "Демо фильм" {
		t.Fatalf("library copy not refreshed: %+v", e)
	}
}

func TestLibraryRowsKeepTheCacheBudget(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	chans := []playlist.Channel{}
	for n := 0; n < 3000; n++ {
		chans = append(chans, playlist.Channel{ID: fmt.Sprint(n), Name: strings.Repeat("к", 200), URL: "http://stream.example/x.ts"})
	}
	a.setChannels("m3u", chans)
	b := a.favoritesBouquetLocked() // the default list made by New
	for _, ch := range chans {
		b.Channels = append(b.Channels, ChannelRef{Source: "m3u", ID: ch.ID, Name: ch.Name})
	}
	for page := 1; page <= 60; page++ {
		cards, err := a.CatalogHomeItems("favorites", "itv", page)
		if err != nil {
			t.Fatal(err)
		}
		c := a.catalogs["m3u"]
		if len(c.library) > libraryCacheItems || c.libraryBytes > libraryCacheBytes || len(c.items) > 0 {
			t.Fatalf("page %d: %d library rows, %d bytes, %d listed", page, len(c.library), c.libraryBytes, len(c.items))
		}
		// Whatever was dropped comes back from the library on demand.
		if _, err := a.CatalogPlay(context.Background(), "m3u", cards.Items[0].Key); err != nil && page > 1 {
			t.Fatalf("page %d: %v", page, err)
		}
	}
}

func TestKeyWinsOverASharedEpisodeID(t *testing.T) {
	a, done := libraryApp(t, t.TempDir())
	defer done()
	ctx := context.Background()
	roots := vodRoots(t, a)
	seasons, _ := a.CatalogItems(ctx, "portal", "vod", "1", roots["lazy"].Key, 1, false)
	episodes, _ := a.CatalogItems(ctx, "portal", "vod", "1", seasons.Items[0].Key, 1, false)
	// A portal that numbers two episodes alike gives them one persistent id.
	first, second := episodes.Items[0], episodes.Items[1]
	a.mu.Lock()
	rec := a.catalogs["portal"].items[second.Key]
	rec.item.CatalogID = first.CatalogID
	a.catalogs["portal"].items[second.Key] = rec
	a.mu.Unlock()
	for i := 0; i < 10; i++ {
		if p, err := a.CatalogPlayEntry(ctx, "portal", second.Key, first.CatalogID); err != nil || !strings.Contains(p.URL, "lazy-1-2") {
			t.Fatalf("played %+v %v instead of the second episode", p, err)
		}
	}
}
