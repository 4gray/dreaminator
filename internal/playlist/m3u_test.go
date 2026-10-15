package playlist

import (
	"strings"
	"testing"
)

const sample = "\ufeff#EXTM3U url-tvg=\"http://epg.example/guide.xml.gz,http://other\"\n" +
	`#EXTINF:-1 tvg-id="perviy.ru" tvg-logo="http://logo/1.png" group-title="Общие, федеральные",Первый канал HD
http://host:8080/live/u/p/1.ts
#EXTINF:0,Без атрибутов
#EXTGRP:Новости
http://host/news.m3u8
#EXTINF:-1 group-title="Кино",Фильм
http://host/movie/u/p/55.mp4
#EXTINF:-1 tvg-name="Имя из атрибута",
http://host/3.ts
#EXTINF:-1,Дубль
http://host/a.ts
#EXTINF:-1,Дубль
http://host/b.ts
`

func TestParseM3U(t *testing.T) {
	res, err := ParseM3U(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.EPGURLs) != 2 || res.EPGURLs[0] != "http://epg.example/guide.xml.gz" || res.EPGURLs[1] != "http://other" {
		t.Errorf("epg urls = %q", res.EPGURLs)
	}
	if len(res.Channels) != 5 {
		t.Fatalf("got %d channels: %+v", len(res.Channels), res.Channels)
	}
	c := res.Channels[0]
	if c.Name != "Первый канал HD" || c.TvgID != "perviy.ru" || c.Group != "Общие, федеральные" || c.Logo != "http://logo/1.png" {
		t.Errorf("first channel = %+v", c)
	}
	if res.Channels[1].Group != "Новости" || res.Channels[1].Name != "Без атрибутов" {
		t.Errorf("EXTGRP channel = %+v", res.Channels[1])
	}
	if res.Channels[2].Name != "Имя из атрибута" {
		t.Errorf("tvg-name fallback = %+v", res.Channels[2])
	}
	if res.Channels[3].ID == res.Channels[4].ID {
		t.Errorf("duplicate names must get distinct ids")
	}

	again, _ := ParseM3U(strings.NewReader(strings.ReplaceAll(sample, "/1.ts", "/1.ts?token=new")))
	if again.Channels[0].ID != c.ID {
		t.Errorf("id must survive url changes")
	}
}

func TestParseM3URejectsHTML(t *testing.T) {
	if _, err := ParseM3U(strings.NewReader("<html><body>403</body></html>")); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeServer(t *testing.T) {
	for in, want := range map[string]string{
		"example.com:8080":                               "http://example.com:8080",
		"http://example.com:8080/":                       "http://example.com:8080",
		"http://example.com/get.php?username=a&password": "http://example.com",
		"https://x.tv/player_api.php":                    "https://x.tv",
	} {
		if got := NormalizeServer(in); got != want {
			t.Errorf("NormalizeServer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseM3UGuideHints(t *testing.T) {
	const pl = `#EXTM3U url-tvg="http://a/epg.xml.gz, http://b/epg.xml" x-tvg-url="http://c/epg.gz http://a/epg.xml.gz" tvg-shift="1"
#EXTINF:-1 tvg-name="Pervyj_kanal",Первый канал
http://h/1.ts
#EXTINF:-1 tvg-id="1tv" tvg-shift="+2",Первый +2
http://h/2.ts
#EXTINF:-1 tvg-name="НТВ" tvg-shift="-1,5",НТВ
http://h/3.ts
`
	res, err := ParseM3U(strings.NewReader(pl))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.EPGURLs, " ") != "http://a/epg.xml.gz http://b/epg.xml http://c/epg.gz" {
		t.Errorf("epg urls = %q", res.EPGURLs)
	}
	c := res.Channels
	if c[0].TvgName != "Pervyj_kanal" || c[0].TvgShift != 1 {
		t.Errorf("header shift / tvg-name: %+v", c[0])
	}
	if c[1].TvgShift != 2 {
		t.Errorf("channel shift overrides header: %+v", c[1])
	}
	if c[2].TvgName != "" || c[2].TvgShift != -1.5 {
		t.Errorf("tvg-name equal to name is dropped, comma decimal parsed: %+v", c[2])
	}
}
