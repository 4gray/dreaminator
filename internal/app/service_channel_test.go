package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dreaminator/internal/enigma"
)

func TestChannelForService(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "guide.xml"), []byte(dayGuideXML(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))), 0o644)
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte(strings.ReplaceAll(archivePlaylist, "%DIR%", dir)), 0o644)
	now := time.Date(2026, 10, 5, 12, 30, 0, 0, time.UTC)
	a := guideTestApp(t, dir, &now)
	ctx := context.Background()
	a.st.Sources = append(a.st.Sources, &Source{ID: "s1", Name: "Edem", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")})
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	chans := map[string]enigma.Service{}
	for _, ch := range a.channels["s1"] {
		chans[ch.Name] = enigma.Service{Key: "s1/" + ch.ID, URL: ch.URL, Name: ch.Name}
	}
	find := func(ref string) ServiceChannel {
		t.Helper()
		v, err := a.ChannelForService(ref)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		return v
	}

	// A line of our lists, as Enigma2 prints it back (lower case hex).
	ref := enigma.ServiceRef(4097, chans["Архив"])
	v := find(strings.ToLower(ref[:strings.Index(ref, "http")]) + ref[strings.Index(ref, "http"):])
	if v.Source.ID != "s1" || v.Source.Name != "Edem" || v.Item.Name != "Архив" || v.Item.Archive != 3 || v.Item.Kind != "channel" ||
		v.Item.ID != catalogID("s1", "itv", a.channels["s1"][0].ID) {
		t.Fatalf("our list: %+v", v)
	}
	// Its key opens the programme like a catalog row.
	day := -1
	if g, err := a.CatalogGuide(ctx, "s1", v.Item.Key, &day, time.UTC); err != nil || g.Archive != 3 {
		t.Fatalf("guide by the key: %+v %v", g, err)
	}
	// The playlist's address changed since the lists were sent (a new token).
	moved := chans["Сутки"]
	moved.URL = "http://h/b.m3u8?token=old"
	if v := find(enigma.ServiceRef(4097, moved)); v.Item.Name != "Сутки" || v.Item.Archive != 1 {
		t.Fatalf("by ids: %+v", v)
	}
	// Another plugin's list: found by the stream address alone.
	if v := find("4097:0:1:0:0:0:0:0:0:0:http%3a//h/c.m3u8:Канал"); v.Item.Name != "Без архива" || v.Item.Archive != 0 {
		t.Fatalf("by address: %+v", v)
	}
	// An address with escapes, as Enigma2 prints another list's line.
	a.channels["s1"][2].URL = "http://h/c%20d.m3u8"
	if v := find("4097:0:1:0:0:0:0:0:0:0:http%3a//h/c%2520d.m3u8:Канал"); v.Item.Name != "Без архива" {
		t.Fatalf("escaped address: %+v", v)
	}
	for _, ref := range []string{
		"1:0:19:283D:3FB:1:C00000:0:0:0:",                     // satellite
		"4097:0:1:0:0:0:0:0:0:0:http%3a//other/x.m3u8:Чужой", // not in any playlist
		enigma.ServiceRef(4097, enigma.Service{Key: "gone/1", URL: "http://other/y.ts", Name: "Нет"}),
	} {
		if _, err := a.ChannelForService(ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", ref, err)
		}
	}

	// A key that expired finds the row again by its persistent id.
	if k := a.CurrentKey("s1", "expired", v.Item.ID); k != v.Item.Key {
		t.Fatalf("current key %q, want %q", k, v.Item.Key)
	}
	if k := a.CurrentKey("s1", v.Item.Key, "s1/itv/nothing"); k != v.Item.Key {
		t.Fatalf("a live key stays: %q", k)
	}
}
