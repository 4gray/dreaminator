package app

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dreaminator/internal/stalker/stalkertest"
)

func TestStalkerSource(t *testing.T) {
	portal := &stalkertest.Portal{
		Path:      "/stalker_portal/server/load.php",
		TokenUses: 5,
		Channels: []stalkertest.Channel{
			{ID: "101", Name: "Первый канал", Genre: "Федеральные", Stream: "http://cdn.example/1tv.ts"},
			{ID: "102", Name: "Мир 24", Genre: "Новости", Stream: "http://cdn.example/mir24.ts"},
		},
	}
	srv := httptest.NewServer(portal)
	defer srv.Close()

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "epgimport"), 0o755)
	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, err := New(Config{DataDir: filepath.Join(dir, "data"), E2Dir: filepath.Join(dir, "e2"),
		EPGImportDir: filepath.Join(dir, "epgimport"), PublicURL: "http://127.0.0.1:8088", Port: "8088"},
		defaults, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	in := SourceInput{Type: "stalker", Server: srv.URL + "/stalker_portal/c/", MAC: "00-1a-79-12-34-56"}
	if err := in.validate(); err != nil {
		t.Fatal(err)
	}
	if in.MAC != "00:1A:79:12:34:56" {
		t.Fatalf("MAC not normalised: %q", in.MAC)
	}
	a.mu.Lock()
	src := &Source{ID: "st1"}
	applyInput(src, in)
	a.st.Sources = append(a.st.Sources, src)
	a.mu.Unlock()

	if err := a.RefreshSource(ctx, "st1"); err != nil {
		t.Fatal(err)
	}
	page := a.Channels("st1", "", "", 0)
	if page.Total != 2 || page.Items[1].Group != "Новости" {
		t.Fatalf("channels = %+v", page)
	}
	a.mu.Lock()
	ch := *a.channel("st1", "102")
	a.mu.Unlock()
	if !strings.HasPrefix(ch.URL, "http://127.0.0.1:8088/stalker/st1/102#User-Agent=") || !ch.NeedsLink {
		t.Fatalf("channel = %+v", ch)
	}

	for i := 0; i < 3; i++ {
		u, err := a.ResolveStalker(ctx, "st1", "102")
		if err != nil || !strings.HasPrefix(u, "http://cdn.example/mir24.ts?play_token=") {
			t.Fatalf("resolve %d = %q, %v", i, u, err)
		}
	}

	refs := []ChannelRef{{Source: "st1", ID: "101"}, {Source: "st1", ID: "102"}}
	a.ModifyBouquet(a.st.Bouquets[0].ID, refs, nil)
	if err := a.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	a.Wait()
	bq, _ := os.ReadFile(filepath.Join(dir, "e2", "userbouquet.dreaminator_"+a.st.Bouquets[0].ID+".tv"))
	if !strings.Contains(string(bq), "http%3a//127.0.0.1%3a8088/stalker/st1/101#User-Agent=Mozilla/5.0 (QtEmbedded; U; Linux; C)") {
		t.Fatalf("bouquet:\n%s", bq)
	}

	if err := a.RebuildEPG(ctx, true); err != nil {
		t.Fatal(err)
	}
	if st := a.View().EPG; st.Matched != 2 || st.Programmes < 100 {
		t.Fatalf("portal guide: %+v", st)
	}

	// The portal session survives a restart.
	a.mu.Lock()
	sess := a.source("st1").Stalker
	a.mu.Unlock()
	if sess == nil || sess.Endpoint != srv.URL+"/stalker_portal/server/load.php" || sess.Token == "" {
		t.Fatalf("session not persisted: %+v", sess)
	}

	// Changing the MAC drops the old token.
	in.MAC = "00:1A:79:00:00:02"
	in.validate()
	a.mu.Lock()
	applyInput(a.source("st1"), in)
	dropped := a.source("st1").Stalker == nil
	a.mu.Unlock()
	if !dropped {
		t.Fatal("token kept across identity change")
	}
}

func TestAccountAndProbe(t *testing.T) {
	ts := []byte{}
	// PAT + PMT (H.264) + filler, enough for the probe.
	pat := []byte{0x47, 0x40, 0x00, 0x10, 0x00, 0x00, 0xb0, 0x0d, 0, 1, 0xc1, 0, 0, 0, 1, 0xe1, 0x00, 0, 0, 0, 0}
	pmt := []byte{0x47, 0x41, 0x00, 0x10, 0x00, 0x02, 0xb0, 0x12, 0, 1, 0xc1, 0, 0, 0xe1, 0x01, 0xf0, 0x00, 0x1b, 0xe1, 0x01, 0xf0, 0x00, 0, 0, 0, 0}
	for _, p := range [][]byte{pat, pmt} {
		pkt := make([]byte, 188)
		for i := range pkt {
			pkt[i] = 0xff
		}
		copy(pkt, p)
		ts = append(ts, pkt...)
	}
	for i := 0; i < 50; i++ {
		ts = append(ts, append([]byte{0x47, 0x01, 0x01, 0x10}, make([]byte, 184)...)...)
	}
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(ts) }))
	defer stream.Close()

	portal := httptest.NewServer(&stalkertest.Portal{
		Path:     "/stalker_portal/server/load.php",
		Channels: []stalkertest.Channel{{ID: "1", Name: "Мир 24", Genre: "Новости", Stream: stream.URL + "/mir24.ts"}},
	})
	defer portal.Close()

	dir := t.TempDir()
	defaults := DefaultSettings()
	defaults.WebifURL = ""
	a, _ := New(Config{DataDir: dir, E2Dir: filepath.Join(dir, "e2"), EPGImportDir: filepath.Join(dir, "epg"), Port: "8088"},
		defaults, log.New(io.Discard, "", 0))
	src, err := a.AddSource(SourceInput{Type: "stalker", Server: portal.URL + "/stalker_portal/c/", MAC: "00:1A:79:00:00:01"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.RefreshSource(ctx, src.ID); err != nil {
		t.Fatal(err)
	}
	// AddSource also refreshes in the background; wait for whichever run wins.
	var acc *Account
	for i := 0; i < 100 && (acc == nil || acc.Expires == nil); i++ {
		a.mu.Lock()
		acc = a.source(src.ID).Account
		a.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	if acc == nil || acc.Expires == nil || acc.Plan != "Базовый" {
		t.Fatalf("account after refresh = %+v", acc)
	}
	if s := a.Summary(); !strings.HasPrefix(s.Subscription, "до ") {
		t.Fatalf("summary subscription = %q", s.Subscription)
	}

	// Expiring soon shows up as a problem for the TV plugin.
	soon := time.Now().Add(3 * 24 * time.Hour)
	a.mu.Lock()
	a.source(src.ID).Account.Expires = &soon
	a.mu.Unlock()
	if p := a.Summary().Problems; len(p) != 1 || !strings.Contains(p[0], "заканчивается через") {
		t.Fatalf("problems = %q", p)
	}

	r, err := a.ProbeChannel(ctx, src.ID, "1")
	if err != nil || !r.OK || r.Video != "H.264" || r.Format != "MPEG-TS" {
		t.Fatalf("probe = %+v, %v", r, err)
	}
}
