package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
	"dreaminator/internal/stalker/stalkertest"
)

func searchPortal(t *testing.T, p *stalkertest.Portal) (*App, func()) {
	t.Helper()
	srv := httptest.NewServer(p)
	a := catalogTestApp(t)
	a.st.Sources[0].Server = srv.URL + "/c/"
	return a, srv.Close
}

func names(cards CatalogCards) []string {
	out := []string{}
	for _, c := range cards.Items {
		out = append(out, c.Name)
	}
	return out
}

func TestCatalogSearchUsesPortalAndKeysWork(t *testing.T) {
	a, done := searchPortal(t, &stalkertest.Portal{Path: "/server/load.php", Catalog: true, DemoMovies: 30})
	defer done()
	ctx := context.Background()
	res, err := a.CatalogSearch(ctx, "portal", "vod", "  тихая   ", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scope != "server" || len(res.Items) != 5 || res.Query != "тихая" {
		t.Fatalf("server search: scope=%s %v", res.Scope, names(res.CatalogCards))
	}
	// Results are ordinary catalog entries: details and playback work.
	if _, err := a.CatalogDetails(ctx, "portal", res.Items[0].Key); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CatalogPlay(ctx, "portal", res.Items[0].Key); err != nil {
		t.Fatal(err)
	}
	empty, err := a.CatalogSearch(ctx, "portal", "vod", "ничего такого", 1)
	if err != nil || empty.Scope != "server" || len(empty.Items) != 0 {
		t.Fatalf("empty result must stay a server answer: %+v %v", empty, err)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "file_film") {
		t.Fatal("commands exposed in search results")
	}
}

func TestCatalogSearchFallsBackWhenPortalIgnoresIt(t *testing.T) {
	a, done := searchPortal(t, &stalkertest.Portal{Path: "/server/load.php", Catalog: true, DemoMovies: 30, IgnoreSearch: true, PageSize: 14})
	defer done()
	ctx := context.Background()
	// Nothing opened yet: the local scope is honest about searching nothing.
	res, err := a.CatalogSearch(ctx, "portal", "vod", "тихая", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scope != "loaded" {
		t.Fatalf("ignored search must not be shown as portal results: %s %v", res.Scope, names(res.CatalogCards))
	}
	for _, n := range names(res.CatalogCards) {
		if !strings.Contains(strings.ToLower(n), "тихая") {
			t.Fatalf("unfiltered entry %q", n)
		}
	}
	// After a page is loaded its entries become searchable; the count says how many.
	if _, err := a.CatalogItems(ctx, "portal", "vod", "2", "", 1, false); err != nil {
		t.Fatal(err)
	}
	res, _ = a.CatalogSearch(ctx, "portal", "vod", "тихая", 1)
	if res.Scope != "loaded" || res.Searched == 0 || len(res.Items) == 0 {
		t.Fatalf("local search: %+v", res)
	}
}

func TestCatalogSearchLiveCoversAllChannels(t *testing.T) {
	a := catalogTestApp(t)
	for n := 0; n < 300; n++ {
		a.channels["portal"] = append(a.channels["portal"], playlist.Channel{ID: fmt.Sprint(n), Name: fmt.Sprintf("Канал %d", n), Group: "Все"})
	}
	a.channels["portal"] = append(a.channels["portal"], playlist.Channel{ID: "x", Name: "Первый канал HD", Group: "Новости"})
	res, err := a.CatalogSearch(context.Background(), "portal", "itv", "ПЕРВЫЙ", 1)
	if err != nil || res.Scope != "channels" || len(res.Items) != 1 || res.Items[0].Name != "Первый канал HD" {
		t.Fatalf("live search: %+v %v", res, err)
	}
	if _, err := a.CatalogSearch(context.Background(), "portal", "itv", "x", 1); !errors.Is(err, ErrCatalogInput) {
		t.Fatal("one-letter query accepted")
	}
}

func TestCatalogSearchNetworkErrorIsNotEmptyResult(t *testing.T) {
	srv := httptest.NewServer(&stalkertest.Portal{Path: "/server/load.php", Catalog: true})
	a := catalogTestApp(t)
	a.st.Sources[0].Server = srv.URL + "/c/"
	srv.Close()
	_, err := a.CatalogSearch(context.Background(), "portal", "vod", "фильм", 1)
	if !errors.Is(err, stalker.ErrCatalogNetwork) {
		t.Fatalf("want network error, got %v", err)
	}
}

func TestSearchWordsFoldCaseYoAndPunctuation(t *testing.T) {
	if !matchesWords("Ёлки-палки: Новый год", searchWords("елки новый")) {
		t.Fatal("ё/case/punctuation not folded")
	}
	if matchesWords("Новогодние ёлки", searchWords("год")) {
		t.Fatal("words must match from their start")
	}
}
