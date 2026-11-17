package app

import (
	"io"
	"log"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPickAddresses(t *testing.T) {
	got := pickAddresses([]ifaceAddr{
		{"lo", net.ParseIP("127.0.0.1")},
		{"tailscale0", net.ParseIP("100.64.0.7")},
		{"wlan0", net.ParseIP("192.168.1.60")},
		{"docker0", net.ParseIP("172.17.0.1")},
		{"eth0", net.ParseIP("192.168.1.45")},
		{"eth0", net.ParseIP("169.254.10.2")},
		{"eth0", net.ParseIP("fe80::1")},
	})
	want := []string{"192.168.1.45", "192.168.1.60"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestHumanWhen(t *testing.T) {
	now := time.Date(2026, 9, 30, 14, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "только что"},
		{now.Add(-12 * time.Minute), "12 мин назад"},
		{time.Date(2026, 9, 30, 4, 0, 0, 0, time.Local), "сегодня в 04:00"},
		{time.Date(2026, 9, 29, 4, 5, 0, 0, time.Local), "вчера в 04:05"},
		{time.Date(2026, 9, 2, 4, 0, 0, 0, time.Local), "2 сентября в 04:00"},
	} {
		if got := humanWhen(c.t, now); got != c.want {
			t.Errorf("humanWhen(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestSummary(t *testing.T) {
	dir := t.TempDir()
	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: dir, E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg"), Port: "8088"},
		defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	s := a.Summary()
	if s.Sources != 0 || s.Channels != 0 || s.EPGText != "ещё не загружалась" || s.Problems == nil {
		t.Fatalf("empty summary = %+v", s)
	}
	src, err := a.AddUploadedSource("Мой", "", "x.m3u", strings.NewReader("#EXTM3U\n#EXTINF:-1,Один\nhttp://h/1.ts\n"))
	if err != nil {
		t.Fatal(err)
	}
	ch := a.Channels(src.ID, "", "", 0).Items[0]
	a.ModifyBouquet(a.st.Bouquets[0].ID, []ChannelRef{{Source: ch.Source, ID: ch.ID}}, nil)
	a.mu.Lock()
	a.source(src.ID).Error = "сервер ответил: 403"
	a.mu.Unlock()

	s = a.Summary()
	if s.Sources != 1 || s.Channels != 1 || !s.Dirty || s.LastRefreshText != "только что" {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.Problems) != 1 || !strings.Contains(s.Problems[0], "«Мой»: сервер ответил: 403") {
		t.Fatalf("problems = %q", s.Problems)
	}
	for _, u := range s.URLs {
		if !strings.HasSuffix(u, ":8088") {
			t.Errorf("tablet url %q lacks the port", u)
		}
	}
}

func TestLanguageSettings(t *testing.T) {
	cases := map[string]string{
		"pt-BR": "pt", "fr-FR": "fr", "he-IL": "he", "iw": "he", "ar-SA": "ar",
		"it-IT": "it", "de-DE": "de", "es-ES": "es", "pl-PL": "pl", "en-GB": "en",
		"ja-JP": "ja", "jp": "ja", "zh-CN": "zh", "th-TH": "th", "tr-TR": "tr",
		"cs-CZ": "cs", "cz": "cs", "ru-RU": "ru", "auto": "", "unknown": "",
	}
	for input, want := range cases {
		if got := NormalizeLanguage(input); got != want {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", input, got, want)
		}
	}

	dir := t.TempDir()
	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: dir, E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg"), Port: "8088"},
		defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetLanguage("fr-FR"); err != nil {
		t.Fatal(err)
	}
	if got := a.View().Settings.Language; got != "fr" {
		t.Fatalf("View().Settings.Language = %q, want fr", got)
	}
	if got := a.Summary().Language; got != "fr" {
		t.Fatalf("Summary().Language = %q, want fr", got)
	}
	// UpdateSettings without Language keeps previous Language.
	keep := a.View().Settings
	keep.Language = ""
	if err := a.UpdateSettings(keep); err != nil {
		t.Fatal(err)
	}
	if got := a.View().Settings.Language; got != "fr" {
		t.Fatalf("UpdateSettings cleared language unexpectedly: %q", got)
	}
	// UpdateSettings with "auto" resets Language to "".
	clearLang := a.View().Settings
	clearLang.Language = "auto"
	if err := a.UpdateSettings(clearLang); err != nil {
		t.Fatal(err)
	}
	if got := a.View().Settings.Language; got != "" {
		t.Fatalf("UpdateSettings(auto) left language = %q", got)
	}
}
