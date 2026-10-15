package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"dreaminator/internal/enigma"
	"dreaminator/internal/picon"
)

func TestPicons(t *testing.T) {
	logo := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for i := range logo.Pix {
		logo.Pix[i] = 0xff
	}
	logo.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	png.Encode(&buf, logo)
	var hits atomic.Int32
	logos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/broken.png" {
			fmt.Fprint(w, "<html>not an image</html>")
			return
		}
		w.Write(buf.Bytes())
	}))
	defer logos.Close()

	dir := t.TempDir()
	piconDir := filepath.Join(dir, "picon")
	os.MkdirAll(piconDir, 0o755)
	foreign := filepath.Join(piconDir, "1_0_19_283D_3FB_1_C00000_0_0_0.png") // a satellite picon
	os.WriteFile(foreign, []byte("sat"), 0o644)

	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: filepath.Join(dir, "data"), E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg"),
		Port: "8088", PiconCandidates: []string{piconDir}}, defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	pl := fmt.Sprintf("#EXTM3U\n#EXTINF:-1 tvg-logo=\"%[1]s/one.png\",Один\nhttp://h/1.ts\n#EXTINF:-1 tvg-logo=\"%[1]s/broken.png\",Два\nhttp://h/2.ts\n#EXTINF:-1,Три\nhttp://h/3.ts\n", logos.URL)
	src, err := a.AddUploadedSource("t", "", "t.m3u", strings.NewReader(pl))
	if err != nil {
		t.Fatal(err)
	}
	var refs []ChannelRef
	for _, c := range a.Channels(src.ID, "", "", 0).Items {
		refs = append(refs, ChannelRef{Source: c.Source, ID: c.ID})
	}
	a.ModifyBouquet(a.st.Bouquets[0].ID, refs, nil)
	if err := a.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	a.Wait()

	st := a.View().Picons
	if st.Channels != 3 || st.Done != 1 || st.NoLogo != 1 || len(st.Failed) != 1 || st.Dir != piconDir {
		t.Fatalf("status = %+v", st)
	}
	one := filepath.Join(piconDir, picon.FileName(enigma.EPGRef(refs[0].Source+"/"+refs[0].ID)))
	if data, err := os.ReadFile(one); err != nil || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatalf("picon %s missing: %v", one, err)
	}

	// Unchanged logos are not downloaded again.
	before := hits.Load()
	a.SyncPicons(t.Context(), false)
	if got := hits.Load() - before; got != 1 { // only the broken one is retried
		t.Fatalf("re-downloaded %d logos", got)
	}

	// A channel leaving the bouquets takes its picon with it; others' stay.
	a.ModifyBouquet(a.st.Bouquets[0].ID, nil, refs[:1])
	a.SyncPicons(t.Context(), false)
	if _, err := os.Stat(one); !os.IsNotExist(err) {
		t.Fatal("stale picon kept")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("satellite picon touched")
	}

	// Turning picons off removes ours only.
	a.ModifyBouquet(a.st.Bouquets[0].ID, refs[:1], nil)
	a.SyncPicons(t.Context(), false)
	s := a.View().Settings
	s.Picons = false
	a.UpdateSettings(s)
	a.SyncPicons(t.Context(), true)
	if matches, _ := filepath.Glob(filepath.Join(piconDir, "*.png")); len(matches) != 1 || matches[0] != foreign {
		t.Fatalf("after turning off: %v", matches)
	}
}
