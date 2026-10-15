package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPlaylist = `#EXTM3U url-tvg="file://%DIR%/guide.xml"
#EXTINF:-1 tvg-id="one",Один
http://h/1.ts
#EXTINF:-1,Два HD
http://h/2.ts
`

const testGuide = `<tv>
<channel id="one"><display-name>One</display-name></channel>
<channel id="two"><display-name>Два</display-name></channel>
<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="one"><title>A</title></programme>
<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="two"><title>B</title></programme>
</tv>`

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "list.m3u"), []byte(strings.ReplaceAll(testPlaylist, "%DIR%", dir)), 0o644)
	os.WriteFile(filepath.Join(dir, "guide.xml"), []byte(testGuide), 0o644)
	os.MkdirAll(filepath.Join(dir, "epgimport"), 0o755)

	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{
		DataDir:      filepath.Join(dir, "data"),
		E2Dir:        filepath.Join(dir, "enigma2"),
		EPGImportDir: filepath.Join(dir, "epgimport"),
		PublicURL:    "http://127.0.0.1:8088",
	}, defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	src := &Source{ID: "s1", Name: "test", Type: "m3u", URL: "file://" + filepath.Join(dir, "list.m3u")}
	a.st.Sources = append(a.st.Sources, src)
	if err := a.RefreshSource(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	page := a.Channels("s1", "", "", 0)
	if page.Total != 2 {
		t.Fatalf("channels = %+v", page)
	}

	fav := a.st.Bouquets[0].ID
	var refs []ChannelRef
	for _, c := range page.Items {
		refs = append(refs, ChannelRef{Source: c.Source, ID: c.ID})
	}
	refs = append(refs, ChannelRef{Source: "s1", ID: "bogus"})
	if err := a.ModifyBouquet(fav, refs, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(a.st.Bouquets[0].Channels); n != 2 {
		t.Fatalf("unknown channels must be rejected, got %d refs", n)
	}
	if !a.View().Dirty {
		t.Fatal("state must be dirty after a change")
	}

	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	a.Wait()
	if a.View().Dirty {
		t.Fatal("apply must clear dirty")
	}
	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	st := a.View().EPG
	if st.Wanted != 2 || st.Matched != 2 || st.Programmes != 2 {
		t.Fatalf("epg status = %+v", st)
	}
	mapping, _ := os.ReadFile(filepath.Join(dir, "epgimport", "dreaminator.channels.xml"))
	if strings.Count(string(mapping), "<channel ") != 2 {
		t.Fatalf("channels.xml:\n%s", mapping)
	}

	// Reloading from disk keeps everything.
	b, err := New(a.cfg, defaults, a.log)
	if err != nil {
		t.Fatal(err)
	}
	if v := b.View(); len(v.Bouquets[0].Channels) != 2 || v.Bouquets[0].Channels[0].Missing {
		t.Fatalf("reloaded state = %+v", v.Bouquets)
	}
}

func TestUploadedPlaylist(t *testing.T) {
	dir := t.TempDir()
	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: dir, E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg")},
		defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.AddUploadedSource("", "", "junk.m3u", strings.NewReader("<html>nope</html>")); err == nil {
		t.Fatal("a file without channels must be rejected")
	}
	if n := len(a.View().Sources); n != 0 {
		t.Fatalf("rejected upload must not create a source, got %d", n)
	}

	src, err := a.AddUploadedSource("", "", "Мой провайдер.m3u", strings.NewReader("#EXTM3U\n#EXTINF:-1,Один\nhttp://h/1.ts\n"))
	if err != nil {
		t.Fatal(err)
	}
	if src.Name != "Мой провайдер" || src.ChannelCount != 1 || src.Type != "file" {
		t.Fatalf("source = %+v", src)
	}

	// A broken replacement keeps the working playlist.
	if _, err := a.ReplaceUpload(src.ID, strings.NewReader("garbage")); err == nil {
		t.Fatal("broken replacement must be rejected")
	}
	if page := a.Channels(src.ID, "", "", 0); page.Total != 1 {
		t.Fatalf("channels after failed replace = %d", page.Total)
	}

	updated, err := a.ReplaceUpload(src.ID, strings.NewReader("#EXTM3U\n#EXTINF:-1,Один\nhttp://h/1.ts\n#EXTINF:-1,Два\nhttp://h/2.ts\n"))
	if err != nil || updated.ChannelCount != 2 {
		t.Fatalf("replace: %v %+v", err, updated)
	}
	// The nightly refresh re-reads the stored file.
	if err := a.RefreshSource(context.Background(), src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpdateSource(src.ID, SourceInput{Type: "m3u", URL: "http://x/y.m3u"}); err == nil {
		t.Fatal("switching an uploaded playlist to a link must be refused")
	}

	if err := a.DeleteSource(src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.uploadPath(src.ID)); !os.IsNotExist(err) {
		t.Fatal("deleting the source must delete the uploaded file")
	}
}

func TestSeveralGuides(t *testing.T) {
	dir := t.TempDir()
	// The first guide knows only "one"; the backup knows both. "one" must
	// come from the first guide only, "two" from the backup.
	os.WriteFile(filepath.Join(dir, "g1.xml"), []byte(`<tv><channel id="one"><display-name>Один</display-name></channel>
<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="one"><title>from g1</title></programme></tv>`), 0o644)
	os.WriteFile(filepath.Join(dir, "g2.xml"), []byte(`<tv><channel id="one"><display-name>Один</display-name></channel>
<channel id="two"><display-name>Два</display-name></channel>
<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="one"><title>from g2</title></programme>
<programme start="20990101000000 +0000" stop="20990101010000 +0000" channel="two"><title>two from g2</title></programme></tv>`), 0o644)
	pl := fmt.Sprintf("#EXTM3U url-tvg=\"file://%s/g1.xml,file://%s/g2.xml\"\n#EXTINF:-1 tvg-id=\"one\",Один\nhttp://h/1.ts\n#EXTINF:-1,Два\nhttp://h/2.ts\n", dir, dir)
	os.MkdirAll(filepath.Join(dir, "epgimport"), 0o755)

	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: filepath.Join(dir, "data"), E2Dir: filepath.Join(dir, "e2"),
		EPGImportDir: filepath.Join(dir, "epgimport"), PublicURL: "http://127.0.0.1:8088"}, defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	src, err := a.AddUploadedSource("t", "", "t.m3u", strings.NewReader(pl))
	if err != nil {
		t.Fatal(err)
	}
	var refs []ChannelRef
	for _, c := range a.Channels(src.ID, "", "", 0).Items {
		refs = append(refs, ChannelRef{Source: c.Source, ID: c.ID})
	}
	a.ModifyBouquet(a.st.Bouquets[0].ID, refs, nil)
	if err := a.RebuildEPG(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	st := a.View().EPG
	if st.Wanted != 2 || st.Matched != 2 || st.Programmes != 2 {
		t.Fatalf("epg status = %+v", st)
	}
	out, _ := os.ReadFile(a.EPGPath())
	if strings.Contains(string(out), ">from g2<") {
		t.Errorf("channel covered by the first guide was taken from the backup too:\n%s", out)
	}
	if !strings.Contains(string(out), ">from g1<") || !strings.Contains(string(out), ">two from g2<") {
		t.Errorf("unexpected guide:\n%s", out)
	}
}
