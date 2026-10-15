package app

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
