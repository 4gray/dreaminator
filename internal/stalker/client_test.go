package stalker

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dreaminator/internal/stalker/stalkertest"
)

func newPortal() *stalkertest.Portal {
	return &stalkertest.Portal{
		Path: "/stalker_portal/server/load.php",
		Channels: []stalkertest.Channel{
			{ID: "101", Name: "Первый канал", Genre: "Федеральные", Stream: "http://cdn.example/1tv/index.m3u8"},
			{ID: "102", Name: "Россия 1", Genre: "Федеральные", Stream: "http://cdn.example/r1.ts?a=1"},
			{ID: "103", Name: "Мир 24", Genre: "Новости", Stream: "http://cdn.example/mir24.ts", Static: true},
			{ID: "104", Name: "Ночной", Genre: "Взрослые", Stream: "http://cdn.example/x.ts", Censored: true},
		},
	}
}

func client(t *testing.T, portal *stalkertest.Portal, mac string, id Identity) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(portal)
	t.Cleanup(srv.Close)
	id.MAC = mac
	return New(srv.URL+"/stalker_portal/c/", id, Session{}, nil), srv
}

func TestCandidates(t *testing.T) {
	for in, want := range map[string]string{
		"http://h/stalker_portal/c/": "http://h/stalker_portal/portal.php http://h/stalker_portal/server/load.php",
		"h:8080":                     "http://h:8080/portal.php http://h:8080/server/load.php http://h:8080/stalker_portal/server/load.php",
		"http://h/stalker_portal/server/load.php": "http://h/stalker_portal/server/load.php http://h/stalker_portal/portal.php",
		"http://h/c/": "http://h/portal.php http://h/server/load.php http://h/stalker_portal/server/load.php",
	} {
		got, err := Candidates(in)
		if err != nil || strings.Join(got, " ") != want {
			t.Errorf("Candidates(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
}

func TestNormalizeMAC(t *testing.T) {
	for _, in := range []string{"00:1a:79:aa:bb:cc", "00-1A-79-AA-BB-CC", "001A79AABBCC", " 00.1a.79.aa.bb.cc "} {
		if got, err := NormalizeMAC(in); err != nil || got != "00:1A:79:AA:BB:CC" {
			t.Errorf("NormalizeMAC(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := NormalizeMAC("00:1A:79"); err == nil {
		t.Error("short MAC accepted")
	}
}

func TestChannelsAndLinks(t *testing.T) {
	portal := newPortal()
	c, srv := client(t, portal, "00:1A:79:12:34:56", Identity{})
	ctx := context.Background()

	genres, err := c.Genres(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(genres) != 3 {
		t.Fatalf("genres = %+v", genres)
	}
	if s := c.Session(); s.Endpoint != srv.URL+"/stalker_portal/server/load.php" || s.Simple || s.Token == "" {
		t.Fatalf("session = %+v", s)
	}

	chans, err := c.Channels(ctx, genres)
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 4 {
		t.Fatalf("channels = %+v (censored genre must be fetched separately)", chans)
	}
	if !chans[0].NeedsLink || chans[2].NeedsLink {
		t.Errorf("link semantics wrong: %+v", chans)
	}
	if chans[0].Logo != srv.URL+"/stalker_portal/misc/logos/101.png" {
		t.Errorf("logo = %q", chans[0].Logo)
	}

	u, err := c.Resolve(ctx, chans[1])
	if err != nil || u != "http://cdn.example/r1.ts?a=1&play_token=tmp1" {
		t.Fatalf("resolve = %q, %v", u, err)
	}
	// Static channel plays without create_link.
	if u, err := c.Resolve(ctx, chans[2]); err != nil || u != "http://cdn.example/mir24.ts" {
		t.Fatalf("static resolve = %q, %v", u, err)
	}
	if _, _, links, watchdogs := portal.Stats(); links != 1 || watchdogs != 1 {
		t.Errorf("links = %d, watchdogs = %d", links, watchdogs)
	}
}

func TestReauthenticatesOnExpiredToken(t *testing.T) {
	portal := newPortal()
	portal.TokenUses = 2
	c, _ := client(t, portal, "00:1A:79:12:34:56", Identity{})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := c.Genres(ctx); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if hs, _, _, _ := portal.Stats(); hs < 2 {
		t.Fatalf("expected re-handshakes, got %d", hs)
	}
}

func TestLoginAndBlockedMAC(t *testing.T) {
	ctx := context.Background()

	portal := newPortal()
	portal.Login, portal.Password = "dad", "secret"
	c, _ := client(t, portal, "00:1A:79:12:34:56", Identity{})
	if _, err := c.Genres(ctx); !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "логин") {
		t.Fatalf("missing login: %v", err)
	}
	c, _ = client(t, portal, "00:1A:79:12:34:56", Identity{Login: "dad", Password: "secret"})
	if _, err := c.Genres(ctx); err != nil {
		t.Fatalf("with login: %v", err)
	}

	c, _ = client(t, newPortal(), "AA:BB:CC:00:00:01", Identity{})
	if _, err := c.Genres(ctx); !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "не зарегистрирован") {
		t.Fatalf("blocked MAC: %v", err)
	}
}

func TestCrawlWhenNoAllChannels(t *testing.T) {
	portal := newPortal()
	portal.NoAllChannels = true
	portal.PageSize = 2
	for i := 0; i < 7; i++ {
		portal.Channels = append(portal.Channels, stalkertest.Channel{ID: "2" + string(rune('0'+i)), Name: "Канал", Genre: "Разное", Stream: "http://cdn.example/x.ts"})
	}
	c, _ := client(t, portal, "00:1A:79:12:34:56", Identity{})
	ctx := context.Background()
	genres, _ := c.Genres(ctx)
	chans, err := c.Channels(ctx, genres)
	if err != nil || len(chans) != 11 {
		t.Fatalf("crawl got %d channels, %v", len(chans), err)
	}
}

func TestGuide(t *testing.T) {
	c, _ := client(t, newPortal(), "00:1A:79:12:34:56", Identity{})
	guide, err := c.Guide(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	progs := guide["101"]
	if len(progs) != 4 || !progs[0].Stop.After(progs[0].Start) || !strings.Contains(progs[0].Title, "Первый канал") {
		t.Fatalf("guide = %+v", progs)
	}
	if d := time.Until(progs[1].Start); d > time.Hour || d < -time.Hour {
		t.Errorf("unexpected start %v", progs[1].Start)
	}
}

func TestStreamURLForms(t *testing.T) {
	c := New("http://portal.tv/stalker_portal/c/", Identity{MAC: "00:1A:79:00:00:01"}, Session{Endpoint: "http://portal.tv/stalker_portal/server/load.php"}, nil)
	for _, tc := range []struct{ cmd, orig, want string }{
		{"ffmpeg http://a/b.ts", "", "http://a/b.ts"},
		{"auto /media/123.mpg", "", "http://portal.tv/stalker_portal/media/123.mpg"},
		{"?token=x", "ffmpeg http://cdn/ch/1?old=1", "http://cdn/ch/1?token=x"},
		{"?token=x", "/ch/1", "http://portal.tv/stalker_portal/ch/1?token=x"},
	} {
		if got := c.streamURL(tc.cmd, tc.orig); got != tc.want {
			t.Errorf("streamURL(%q, %q) = %q, want %q", tc.cmd, tc.orig, got, tc.want)
		}
	}
	if !NeedsLink("ffrt3 http://localhost/ch/1_", false, false, true) || NeedsLink("ffmpeg http://cdn/x.ts", false, false, true) || !NeedsLink("ffmpeg http://cdn/x.ts", false, false, false) {
		t.Error("NeedsLink rule")
	}
	if got := encodeCmd("ffrt3 http://localhost/ch/1_&x"); got != "ffrt3%20http://localhost/ch/1_%26x" {
		t.Errorf("encodeCmd = %q", got)
	}
}

func TestAccount(t *testing.T) {
	c, _ := client(t, newPortal(), "00:1A:79:12:34:56", Identity{})
	acc, err := c.Account(context.Background())
	if err != nil || acc.Plan != "Базовый" || acc.Expires == nil || acc.Active == nil || !*acc.Active {
		t.Fatalf("account = %+v, %v", acc, err)
	}
	if d := time.Until(*acc.Expires); d < 50*24*time.Hour || d > 70*24*time.Hour {
		t.Errorf("expiry %v", acc.Expires)
	}
	for in, want := range map[string]string{
		"1767225600":               "2026-01-01",
		"1767225600000":            "2026-01-01",
		"12.03.2027":               "2027-03-12",
		"March 12, 2027, 12:00 am": "2027-03-12",
	} {
		got, unlimited, ok := parseExpiry(in)
		if !ok || unlimited || got.UTC().Add(12*time.Hour).Format("2006-01-02") != want {
			t.Errorf("parseExpiry(%q) = %v %v %v", in, got, unlimited, ok)
		}
	}
	if _, unlimited, ok := parseExpiry("-1"); !ok || !unlimited {
		t.Error("-1 must mean unlimited")
	}
}
