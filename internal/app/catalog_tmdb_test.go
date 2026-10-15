package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"testing"

	"dreaminator/internal/stalker"
	"dreaminator/internal/tmdb/tmdbtest"
)

func jpegBackdrop(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 780, 439)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCatalogCardUsesTMDBWithoutBlockingProviderData(t *testing.T) {
	s := fakeTMDB(t)
	s.AddMovie(tmdbtest.Record{ID: 7001, Title: "Дорога к дому", Original: "Road Home", Date: "2022-02-02", Overview: "Из TMDB", Backdrop: "/road.jpg", Vote: 7.4, Votes: 100})
	s.AddImage("/road.jpg", jpegBackdrop(t))
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	film := vodMovie("r1", "Дорога к дому", "", "2022")
	film.Key = "film-key"
	browse(t, a, "", film)

	details, err := a.CatalogDetails(ctx, "portal", "film-key")
	if err != nil || details.Name != "Дорога к дому" || details.HasBackdrop {
		t.Fatalf("provider details must not wait for TMDB: %+v %v", details, err)
	}
	info, err := a.CatalogTMDB(ctx, "portal", "film-key")
	if err != nil || info == nil || info.ID != 7001 || !info.HasBackdrop || info.Rating != "7.4" {
		t.Fatalf("tmdb: %+v %v", info, err)
	}
	art, err := a.CatalogImage(ctx, "portal", "film-key", "backdrop", 0)
	if err != nil || art.ContentType != "image/jpeg" {
		t.Fatalf("backdrop: %v", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(art.Data))
	if err != nil || cfg.Width != 780 {
		t.Fatalf("shaded backdrop keeps TMDB size: %+v %v", cfg, err)
	}
	s.ResetHits()
	if _, err := a.CatalogImage(ctx, "portal", "film-key", "backdrop", 0); err != nil || s.Hits("image") != 0 {
		t.Fatalf("backdrop must come from the cache: %v hits=%d", err, s.Hits("image"))
	}
}

func TestCatalogCardWithoutTMDBKeyStillWorks(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, false)
	ctx := context.Background()
	film := vodMovie("m1", "The Matrix", "", "1999")
	film.Key = "k"
	browse(t, a, "", film)
	if info, err := a.CatalogTMDB(ctx, "portal", "k"); err != nil || info != nil {
		t.Fatalf("no key: %+v %v", info, err)
	}
	if _, err := a.CatalogImage(ctx, "portal", "k", "backdrop", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no backdrop without TMDB: %v", err)
	}
	if s.Hits("") != 0 {
		t.Fatal("TMDB contacted without a key")
	}
	if _, err := a.CatalogTMDB(ctx, "portal", "gone"); !errors.Is(err, ErrCatalogExpired) {
		t.Fatalf("stale key: %v", err)
	}
}

func TestCatalogEpisodesUsePortalSeasonNumbers(t *testing.T) {
	s := fakeTMDB(t)
	a, _ := tmdbTestApp(t, s, true)
	series := stalker.CatalogItem{Key: "series", Type: "vod", Kind: "series", ID: "h1", Name: "American Hostage s01", Year: "2024"}
	season := stalker.CatalogItem{Key: "season", Type: "vod", Kind: "season", ID: "h1", MovieID: "h1", SeasonID: "h1", Name: "Эпизоды"}
	browse(t, a, "", series)
	browse(t, a, "series", season)
	eps, err := a.CatalogEpisodes(context.Background(), "portal", "season")
	if err != nil || len(eps) != 3 || eps[0].Number != 1 || eps[1].Name != "Вторая" {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	none, err := a.CatalogEpisodes(context.Background(), "portal", "series")
	if err != nil || len(none) != 0 {
		t.Fatalf("a series root has no episode names: %+v", none)
	}
}

func TestSeasonNumberFromPortalNames(t *testing.T) {
	for _, tc := range []struct {
		season, series string
		number         string
		want           int
	}{
		{"Сезон 2", "Фарго", "", 2},
		{"Эпизоды", "Беспринципные (3 сезон)", "", 3},
		{"Эпизоды", "Russian Doll (2 season)", "", 2},
		{"Эпизоды", "American Hostage s01", "", 1},
		{"Season 4", "Show", "", 4},
		{"Эпизоды", "10 друзей Кролика", "", 1},
		{"Любое", "Любой", "5", 5},
	} {
		got := seasonNumber(stalker.CatalogItem{Name: tc.season, Number: tc.number}, stalker.CatalogItem{Name: tc.series})
		if got != tc.want {
			t.Errorf("%q / %q: got %d, want %d", tc.season, tc.series, got, tc.want)
		}
	}
}

func TestCatalogCastPhotosAndEpisodeStills(t *testing.T) {
	s := fakeTMDB(t)
	s.AddMovie(tmdbtest.Record{ID: 7100, Title: "Касса", Original: "Cash", Date: "2010-01-01", Crew: []string{"Режиссёр Один"},
		Cast: []tmdbtest.Person{{Name: "Актёр А", Character: "Герой", Photo: "/a.jpg"}, {Name: "Актёр Б"}}})
	s.AddImage("/a.jpg", picture(t, "jpeg"))
	s.AddShow(tmdbtest.Record{ID: 7200, Title: "Шоу", Original: "Show", Date: "2019-01-01", Crew: []string{"Автор", "Автор"}})
	s.AddSeason(7200, 2, tmdbtest.Episode{Number: 1, Name: "Первая", Still: "/st1.jpg"}, tmdbtest.Episode{Number: 2, Name: "Вторая"})
	s.AddImage("/st1.jpg", picture(t, "jpeg"))
	a, _ := tmdbTestApp(t, s, true)
	ctx := context.Background()
	film := vodMovie("c1", "Касса", "", "2010")
	film.Key = "film"
	series := stalker.CatalogItem{Key: "series", Type: "vod", Kind: "series", ID: "w", Name: "Шоу (2 сезон)", Year: "2019"}
	season := stalker.CatalogItem{Key: "season", Type: "vod", Kind: "season", ID: "w", MovieID: "w", Name: "Эпизоды"}
	ep1 := stalker.CatalogItem{Key: "ep1", Type: "vod", Kind: "episode", ID: "w:1", MovieID: "w", Episode: "1", Number: "1"}
	ep2 := stalker.CatalogItem{Key: "ep2", Type: "vod", Kind: "episode", ID: "w:2", MovieID: "w", Episode: "2", Number: "2"}
	browse(t, a, "", film, series)
	browse(t, a, "series", season)
	browse(t, a, "season", ep1, ep2)

	info, err := a.CatalogTMDB(ctx, "portal", "film")
	if err != nil || info == nil || len(info.Cast) != 2 || !info.Cast[0].HasPhoto || info.Cast[1].HasPhoto ||
		len(info.Directors) != 1 || info.Directors[0] != "Режиссёр Один" {
		t.Fatalf("credits: %+v %v", info, err)
	}
	if _, err := a.CatalogImage(ctx, "portal", "film", "cast", 0); err != nil {
		t.Fatalf("portrait: %v", err)
	}
	if _, err := a.CatalogImage(ctx, "portal", "film", "cast", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an actor without a photo: %v", err)
	}
	if _, err := a.CatalogImage(ctx, "portal", "film", "cast", 9); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of range: %v", err)
	}
	tv, _ := a.CatalogTMDB(ctx, "portal", "series")
	if tv == nil || len(tv.Directors) != 1 || tv.Directors[0] != "Автор" {
		t.Fatalf("series creators: %+v", tv)
	}
	eps, _ := a.CatalogEpisodes(ctx, "portal", "season")
	if len(eps) != 2 || !eps[0].HasStill || eps[1].HasStill {
		t.Fatalf("season 2 from the series name: %+v", eps)
	}
	if _, err := a.CatalogImage(ctx, "portal", "ep1", "still", 0); err != nil {
		t.Fatalf("still: %v", err)
	}
	if _, err := a.CatalogImage(ctx, "portal", "ep2", "still", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no still: %v", err)
	}
}
