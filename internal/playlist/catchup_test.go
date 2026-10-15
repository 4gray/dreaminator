package playlist

import (
	"strings"
	"testing"
	"time"
)

func TestParseCatchupAttributes(t *testing.T) {
	src := `#EXTM3U catchup="append" catchup-source="?archive={utc}" catchup-days="2"
#EXTINF:0 tvg-rec="7",Edem
http://edem.example/iptv/TOKEN/106/index.m3u8
#EXTINF:0 tvg-rec="0",Edem без архива
http://edem.example/iptv/TOKEN/107/index.m3u8
#EXTINF:0 tvg-id="1" catchup="shift" catchup-days="3" timeshift="3",VipLime
http://vip.example/high/TOKEN/1.m3u8
#EXTINF:0 catchup="vod",Неизвестный режим
http://x.example/vod.m3u8
#EXTINF:0 catchup="flussonic-hls",Без глубины
http://fs.example/ch/index.m3u8?token=t
#EXTINF:0 catchup-days="90",Слишком глубоко
http://x.example/deep.m3u8
#EXTINF:0,Из заголовка
http://x.example/plain.ts
`
	res, err := ParseM3U(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*Catchup{}
	for _, ch := range res.Channels {
		got[ch.Name] = ch.Catchup
	}
	check := func(name, mode string, days int, source string) {
		t.Helper()
		c := got[name]
		if mode == "" {
			if c != nil {
				t.Errorf("%s: archive %+v, want none", name, c)
			}
			return
		}
		if c == nil || c.Mode != mode || c.Days != days || c.Source != source {
			t.Errorf("%s: %+v, want %s/%d/%q", name, c, mode, days, source)
		}
	}
	// The channel's own depth, the header's mode and template.
	check("Edem", "append", 7, "?archive={utc}")
	check("Edem без архива", "", 0, "")
	check("VipLime", "shift", 3, "?archive={utc}")
	check("Неизвестный режим", "", 0, "")
	check("Без глубины", "flussonic", 2, "?archive={utc}")
	check("Слишком глубоко", "append", MaxCatchupDays, "?archive={utc}")
	check("Из заголовка", "append", 2, "?archive={utc}")

	plain, _ := ParseM3U(strings.NewReader(strings.SplitN(src, "\n", 2)[1]))
	for _, ch := range plain.Channels {
		if ch.Name == "Edem" && (ch.Catchup == nil || ch.Catchup.Mode != "shift" || ch.Catchup.Days != 7) {
			t.Errorf("tvg-rec alone: %+v, want shift/7", ch.Catchup)
		}
		if ch.Name == "Без глубины" && (ch.Catchup == nil || ch.Catchup.Days != defaultCatchupDays) {
			t.Errorf("mode without depth: %+v", ch.Catchup)
		}
		if ch.Name == "Из заголовка" && ch.Catchup != nil {
			t.Errorf("no attributes, no header: %+v", ch.Catchup)
		}
	}
	// The archive is not part of the id: favourites survive a playlist that
	// starts announcing it.
	if plain.Channels[0].ID != res.Channels[0].ID {
		t.Error("archive attributes changed the channel id")
	}
}

func TestCatchupURL(t *testing.T) {
	start := time.Date(2026, 10, 7, 5, 0, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	now := start.Add(3 * time.Hour)
	s, e, n := "1791349200", "1791354600", "1791360000"
	cases := []struct {
		c      Catchup
		stream string
		want   string
	}{
		{Catchup{Mode: "shift"}, "http://h/iptv/T/106/index.m3u8", "http://h/iptv/T/106/index.m3u8?utc=" + s + "&lutc=" + n},
		{Catchup{Mode: "shift"}, "http://h/1.m3u8?token=x", "http://h/1.m3u8?token=x&utc=" + s + "&lutc=" + n},
		{Catchup{Mode: "default", Source: "http://a/{utc}/{utcend}/{lutc}/{duration}/{duration:60}/{offset:60}"}, "http://h/live",
			"http://a/" + s + "/" + e + "/" + n + "/5400/90/180"},
		{Catchup{Mode: "default", Source: "http://a/${start}-${end}-${timestamp}/{Y}{m}{d}{H}{M}{S}/{utc:Y-m-d H:M}"}, "http://h/live",
			"http://a/" + s + "-" + e + "-" + n + "/20261007050000/2026-10-07 05:00"},
		{Catchup{Mode: "default"}, "http://h/live", "http://h/live?utc=" + s + "&lutc=" + n},
		{Catchup{Mode: "append", Source: "&from={utc}&{unknown}"}, "http://h/live?a=1", "http://h/live?a=1&from=" + s + "&{unknown}"},
		// Parameters go before Enigma2's "#…" and Kodi's "|…" headers.
		{Catchup{Mode: "shift"}, "http://h/1.m3u8#User-Agent=X", "http://h/1.m3u8?utc=" + s + "&lutc=" + n + "#User-Agent=X"},
		{Catchup{Mode: "shift"}, "http://h/1.m3u8?t=1|User-Agent=X", "http://h/1.m3u8?t=1&utc=" + s + "&lutc=" + n + "|User-Agent=X"},
		{Catchup{Mode: "append", Source: "?from={utc}"}, "http://h/live#x", "http://h/live?from=" + s + "#x"},
		{Catchup{Mode: "flussonic"}, "http://fs/ch1/index.m3u8?t=1#User-Agent=X", "http://fs/ch1/index-" + s + "-5400.m3u8?t=1#User-Agent=X"},
		{Catchup{Mode: "xc"}, "http://p/live/u/pw/42.ts#User-Agent=X", "http://p/timeshift/u/pw/90/2026-10-07:05-00/42.ts#User-Agent=X"},
		{Catchup{Mode: "flussonic"}, "http://fs:8080/ch1/index.m3u8?token=t", "http://fs:8080/ch1/index-" + s + "-5400.m3u8?token=t"},
		{Catchup{Mode: "flussonic"}, "http://fs/ch1/video.m3u8", "http://fs/ch1/video-" + s + "-5400.m3u8"},
		{Catchup{Mode: "flussonic"}, "http://fs/ch1/mpegts?token=t", "http://fs/ch1/timeshift_abs-" + s + ".ts?token=t"},
		{Catchup{Mode: "xc"}, "http://p:8080/live/u/pw/42.ts", "http://p:8080/timeshift/u/pw/90/2026-10-07:05-00/42.ts"},
		{Catchup{Mode: "xc"}, "http://p/u/pw/42", "http://p/timeshift/u/pw/90/2026-10-07:05-00/42.ts"},
		{Catchup{Mode: "xc"}, "https://p/live/u/pw/42.ts?token=t", "https://p/timeshift/u/pw/90/2026-10-07:05-00/42.ts?token=t"},
	}
	for _, c := range cases {
		got, err := c.c.URL(c.stream, start, stop, now)
		if err != nil || got != c.want {
			t.Errorf("%s %s:\n got %s (%v)\nwant %s", c.c.Mode, c.stream, got, err, c.want)
		}
	}
	for _, c := range []struct {
		c      *Catchup
		stream string
	}{{nil, "http://h/x"}, {&Catchup{Mode: "flussonic"}, "http://h/plain.ts"}, {&Catchup{Mode: "xc"}, "http://h/a.m3u8"}, {&Catchup{Mode: "odd"}, "http://h/x"}} {
		if _, err := c.c.URL(c.stream, start, stop, now); err == nil {
			t.Errorf("%+v %s: want an error", c.c, c.stream)
		}
	}
}
