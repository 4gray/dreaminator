package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// tsSample builds MPEG-TS with a PAT, a PMT announcing the given streams
// (stream_type → descriptor bytes) and some filler packets.
func tsSample(streams [][2]any, filler int) []byte {
	packet := func(pid int, section []byte) []byte {
		p := make([]byte, 188)
		for i := range p {
			p[i] = 0xff
		}
		p[0], p[1], p[2], p[3] = 0x47, 0x40|byte(pid>>8), byte(pid), 0x10
		p[4] = 0 // pointer_field
		copy(p[5:], section)
		return p
	}
	sec := func(tableID byte, body []byte) []byte {
		length := len(body) + 4 // + CRC
		out := []byte{tableID, 0xb0 | byte(length>>8), byte(length)}
		return append(append(out, body...), 0, 0, 0, 0)
	}
	pat := sec(0x00, []byte{0, 1, 0xc1, 0, 0, 0, 1, 0xe1, 0x00}) // program 1 → PMT PID 0x100
	pmtBody := []byte{0, 1, 0xc1, 0, 0, 0xe1, 0x01, 0xf0, 0x00}
	for i, s := range streams {
		desc := s[1].([]byte)
		pid := 0x101 + i
		pmtBody = append(pmtBody, s[0].(byte), 0xe0|byte(pid>>8), byte(pid), 0xf0|byte(len(desc)>>8), byte(len(desc)))
		pmtBody = append(pmtBody, desc...)
	}
	out := append(packet(0, pat), packet(0x100, sec(0x02, pmtBody))...)
	for i := 0; i < filler; i++ {
		f := make([]byte, 188)
		f[0], f[1], f[2], f[3] = 0x47, 0x01, 0x01, 0x10
		out = append(out, f...)
	}
	return out
}

func TestProbeTS(t *testing.T) {
	ts := tsSample([][2]any{{byte(0x1b), []byte{}}, {byte(0x0f), []byte{}}, {byte(0x06), []byte{0x6a, 0x01, 0x00}}}, 3000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Write(ts)
	}))
	defer srv.Close()
	r := Probe(context.Background(), srv.URL+"/live/1.ts#User-Agent=X", Options{})
	if !r.OK || r.Format != "MPEG-TS" || r.Video != "H.264" || strings.Join(r.Audio, ",") != "AAC,AC-3" || len(r.Warnings) != 0 {
		t.Fatalf("result = %+v", r)
	}
}

func TestProbeHLSAndHEVC(t *testing.T) {
	ts := tsSample([][2]any{{byte(0x24), []byte{}}, {byte(0x03), []byte{}}}, 200)
	mux := http.NewServeMux()
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=12000000\nhi/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=4000000\nmid/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=900000\nlow/index.m3u8\n")
	})
	mux.HandleFunc("/mid/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg1.ts\n#EXTINF:6,\nseg2.ts\n")
	})
	mux.HandleFunc("/mid/seg2.ts", func(w http.ResponseWriter, r *http.Request) { w.Write(ts) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// A resolver-style redirect in front, like the Stalker resolver.
	front := httptest.NewServer(http.RedirectHandler(srv.URL+"/master.m3u8", http.StatusFound))
	defer front.Close()

	r := Probe(context.Background(), front.URL, Options{})
	if !r.OK || r.Format != "HLS (MPEG-TS)" || r.Video != "H.265 (HEVC)" || r.Audio[0] != "MPEG Audio (MP2)" {
		t.Fatalf("result = %+v", r)
	}
	if len(r.Warnings) == 0 || !strings.Contains(r.Warnings[0], "H.265") {
		t.Fatalf("no HEVC warning: %+v", r.Warnings)
	}
}

func TestProbeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gone.ts":
			http.Error(w, "nope", http.StatusForbidden)
		case "/x.mpd":
			w.Header().Set("Content-Type", "application/dash+xml")
			fmt.Fprint(w, "<MPD/>")
		}
	}))
	defer srv.Close()
	if r := Probe(context.Background(), srv.URL+"/gone.ts", Options{}); r.OK || r.Status != 403 || !strings.Contains(r.Error, "403") {
		t.Fatalf("403: %+v", r)
	}
	if r := Probe(context.Background(), srv.URL+"/x.mpd", Options{}); r.OK || !strings.Contains(r.Error, "DASH") {
		t.Fatalf("dash: %+v", r)
	}
	if r := Probe(context.Background(), "http://127.0.0.1:1/x.ts", Options{}); r.OK || !strings.Contains(r.Error, "нет связи") {
		t.Fatalf("refused: %+v", r)
	}
}
