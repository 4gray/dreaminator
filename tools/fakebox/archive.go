package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// A demo provider with an archive (catch-up), like the playlists it copies:
//
//	/archive/playlist.m3u   two channels: tvg-rec="3" (Edem) and
//	                        catchup="shift" catchup-days="1" (VipLime)
//	/archive/guide.xml      their guide, yesterday and today only (epg.one)
//	/archive/{ch}/index.m3u8?utc=&lutc=
//	                        edem: a sliding list without an end, like a
//	                        delayed live stream; vip: a recording from utc
//	                        to lutc with #EXT-X-ENDLIST
//
// The segments do not exist: the archive is for the service and the plugin
// tests, not for watching.
func archiveRoutes(mux *http.ServeMux, self string) {
	mux.HandleFunc("GET /archive/playlist.m3u", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		fmt.Fprintf(w, "#EXTM3U url-tvg=\"%[1]s/archive/guide.xml\"\n"+
			"#EXTINF:0 tvg-id=\"edem\" tvg-rec=\"3\",Архив Edem\n%[1]s/archive/edem/index.m3u8\n"+
			"#EXTINF:0 tvg-id=\"vip\" catchup=\"shift\" catchup-days=\"1\",Архив VipLime\n%[1]s/archive/vip/index.m3u8\n"+
			"#EXTINF:0 tvg-id=\"edem\" tvg-rec=\"0\",Без архива\n%[1]s/archive/none/index.m3u8\n", self)
	})
	mux.HandleFunc("GET /archive/guide.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
		fmt.Fprint(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tv>\n"+
			"<channel id=\"edem\"><display-name>Архив Edem</display-name></channel>\n"+
			"<channel id=\"vip\"><display-name>Архив VipLime</display-name></channel>\n")
		const layout = "20060102150405 -0700"
		for h := 0; h < 48; h++ {
			s := from.Add(time.Duration(h) * time.Hour)
			for _, ch := range []string{"edem", "vip"} {
				fmt.Fprintf(w, "<programme start=\"%s\" stop=\"%s\" channel=\"%s\"><title>%s: выпуск %s</title></programme>\n",
					s.Format(layout), s.Add(time.Hour).Format(layout), ch, ch, s.Format("02.01 15:04"))
			}
		}
		fmt.Fprint(w, "</tv>\n")
	})
	mux.HandleFunc("GET /archive/{ch}/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		ch := r.PathValue("ch")
		utc, _ := strconv.ParseInt(r.URL.Query().Get("utc"), 10, 64)
		lutc, _ := strconv.ParseInt(r.URL.Query().Get("lutc"), 10, 64)
		var b strings.Builder
		b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:10\n")
		switch {
		case utc == 0:
			live := time.Now().Unix() / 10
			fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", live)
			for i := int64(0); i < 6; i++ {
				fmt.Fprintf(&b, "#EXTINF:10.0,\n/archive/%s/live-%d.ts\n", ch, live+i)
			}
		case ch == "vip":
			if lutc <= utc {
				http.Error(w, "bad range", http.StatusBadRequest)
				return
			}
			b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MEDIA-SEQUENCE:0\n")
			for t := utc; t < lutc; t += 10 {
				fmt.Fprintf(&b, "#EXTINF:10.0,\n/archive/vip/rec-%d.ts\n", t)
			}
			b.WriteString("#EXT-X-ENDLIST\n")
		default:
			b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
			for i := int64(0); i < 6; i++ {
				fmt.Fprintf(&b, "#EXTINF:10.0,\n/archive/%s/arch-%d.ts\n", ch, utc+i*10)
			}
		}
		w.Write([]byte(b.String()))
	})
}
