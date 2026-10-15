package tmdb

import (
	"reflect"
	"testing"
)

func TestClean(t *testing.T) {
	for _, tc := range []struct {
		in     string
		series bool
		want   string
		year   int
	}{
		{"The Matrix", false, "The Matrix", 0},
		{"The Matrix (1999) [HD]", false, "The Matrix", 1999},
		{"Матрица [FHD]", false, "Матрица", 0},
		{"Дюна (2021) (4K, HDR)", false, "Дюна", 2021},
		{"Film (WEB-DL 1080p)", false, "Film", 0},
		{"Film 1080p", false, "Film", 0},
		{"Un prince (presque) charmant", false, "Un prince (presque) charmant", 0},
		{"American Hostage s01", true, "American Hostage", 0},
		{"American Hostage s01", false, "American Hostage s01", 0},
		{"Show S1", true, "Show", 0},
		{"Show s02e05", true, "Show", 0},
		{"Остаться в живых сезон 2", true, "Остаться в живых", 0},
		{"Остаться в живых 2 сезон", true, "Остаться в живых", 0},
		{"Остаться в живых - 3-й сезон", true, "Остаться в живых", 0},
		{"Lost (1-3 сезон)", true, "Lost", 0},
		{"Lost: Season 4 [HD]", true, "Lost", 0},
		{"Fargo (2014) s01", true, "Fargo", 2014},
		{"Сезон 1", true, "Сезон 1", 0}, // never clean a title away
		{"AC/DC: Let There Be Rock", false, "AC/DC: Let There Be Rock", 0},
	} {
		got, year := Clean(tc.in, tc.series)
		if got != tc.want || year != tc.year {
			t.Errorf("Clean(%q, %v) = %q, %d; want %q, %d", tc.in, tc.series, got, year, tc.want, tc.year)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"The Matrix":                   "the matrix",
		"  Ёжик   в тумане!":           "ежик в тумане",
		"Ocean's Eleven":               "oceans eleven",
		"Amélie":                       "amelie",
		"Un prince (presque) charmant": "un prince presque charmant",
		"WALL·E":                       "wall e",
		"Мстители: Финал":              "мстители финал",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewQuery(t *testing.T) {
	q := NewQuery("movie", "Матрица / The Matrix [HD]", "The Matrix", "")
	if !reflect.DeepEqual(q.Titles, []string{"матрица", "the matrix"}) || q.Year != 0 {
		t.Fatalf("split titles: %+v", q)
	}
	q = NewQuery("movie", "Дюна (2021)", "", "")
	if q.Year != 2021 || q.Search[0] != "Дюна" {
		t.Fatalf("title year: %+v", q)
	}
	q = NewQuery("movie", "Дюна (2021)", "", "2020-11-01")
	if q.Year != 2020 {
		t.Fatalf("the provider's year wins: %+v", q)
	}
	q = NewQuery("tv", "American Hostage s01", "", "2024")
	if q.Search[0] != "American Hostage" || q.Titles[0] != "american hostage" {
		t.Fatalf("season suffix: %+v", q)
	}
}

func movie(id int, title, original, date string) Title {
	return Title{ID: id, Type: "movie", Title: title, OriginalTitle: original, Date: date}
}

func TestPick(t *testing.T) {
	matrix := movie(603, "Матрица", "The Matrix", "1999-03-30")
	reloaded := movie(604, "Матрица: Перезагрузка", "The Matrix Reloaded", "2003-05-15")
	making := movie(684, "Создание «Матрицы»", "Making The Matrix", "1999-01-01")

	pick := func(q Query, titles ...Title) (int, string) {
		best, reason := Pick(Rank(q, titles))
		if best == nil {
			return 0, reason
		}
		return best.ID, reason
	}
	if id, why := pick(NewQuery("movie", "The Matrix", "", "1999"), reloaded, making, matrix); id != 603 {
		t.Fatalf("exact title and year: %d %s", id, why)
	}
	if id, _ := pick(NewQuery("movie", "Матрица", "", "2000"), matrix, reloaded); id != 603 {
		t.Fatal("a year off by one must still match")
	}
	if id, _ := pick(NewQuery("movie", "The Matrix", "", "2005"), matrix); id != 0 {
		t.Fatal("a title six years away is another film")
	}
	if id, why := pick(NewQuery("movie", "Двойник", "", "2013"), movie(1, "Двойник", "The Double", "2013-01-01"), movie(2, "Двойник", "Double", "2013-06-01")); id != 0 || why != "ambiguous" {
		t.Fatalf("same title and year twice must be ambiguous: %d %s", id, why)
	}
	if id, why := pick(NewQuery("movie", "Making", "", "1999"), making); id != 0 || why != "none" {
		t.Fatalf("a partial title must never win: %d %s", id, why)
	}
	if id, _ := pick(NewQuery("movie", "Матрица ремейк", "The Matrix", "1999"), matrix); id != 603 {
		t.Fatal("the original title must match too")
	}
	show := Title{ID: 60622, Type: "tv", Title: "Фарго", OriginalTitle: "Fargo", Date: "2014-04-15"}
	film := movie(275, "Фарго", "Fargo", "2014-01-01")
	if id, _ := pick(NewQuery("tv", "Fargo s01", "", "2014"), film, show); id != 60622 {
		t.Fatal("the work type must decide between a film and a show")
	}
	if id, _ := pick(NewQuery("tv", "Friends", "", "2003"), Title{ID: 1668, Type: "tv", Title: "Друзья", OriginalTitle: "Friends", Date: "1994-09-22"}); id != 1668 {
		t.Fatal("a show dated by its latest season must still match")
	}
	if id, why := pick(NewQuery("movie", "Нет такого", "", "")); id != 0 || why != "none" {
		t.Fatal("no candidates")
	}
}

func TestQueryDropsOnlyTheProvidersYear(t *testing.T) {
	q := NewQuery("movie", "Robots 2023", "", "2023")
	if len(q.Search) < 2 || q.Search[0] != "Robots" || q.Year != 2023 {
		t.Fatalf("glued year: %+v", q)
	}
	q = NewQuery("movie", "Blade Runner 2049", "", "2017")
	if len(q.Search) != 1 || q.Search[0] != "Blade Runner 2049" {
		t.Fatalf("a year that is part of the title must stay: %+v", q)
	}
}

func TestCleanDropsEpisodeCountsAndSeriesNotes(t *testing.T) {
	for in, want := range map[string]string{
		"Дубровский. Русский Зорро (8 серий)": "Дубровский. Русский Зорро",
		"Костя - Вера (17 серий)":             "Костя - Вера",
		"Смешарики (сериал 2004 – 2012)":      "Смешарики",
		"Барбоскины (15 сезонов)":             "Барбоскины",
	} {
		if got, _ := Clean(in, true); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	if got, _ := Clean("Семнадцать мгновений весны (12 серий)", false); got == "Семнадцать мгновений весны" {
		t.Error("films keep their titles as they are")
	}
}
