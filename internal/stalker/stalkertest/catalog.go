package stalkertest

import (
	"crypto/sha1"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// serveCatalog runs with the portal lock held and after session validation.
func (p *Portal) serveCatalog(w http.ResponseWriter, r *http.Request) bool {
	q := r.URL.Query()
	section := q.Get("type")
	posterBase := strings.TrimSuffix(p.Path, "/server/load.php") + "/misc/logos/"
	if section != "vod" && section != "series" {
		return false
	}
	switch q.Get("action") {
	case "get_categories":
		cats := []map[string]any{{"id": 1, "title": "Демо каталог"}}
		if section == "vod" && p.DemoMovies > 0 {
			cats = append(cats, map[string]any{"id": 2, "title": "Фильмы с обложками"})
		}
		reply(w, cats)
	case "get_ordered_list":
		rows := []map[string]any{}
		movie, season := q.Get("movie_id"), q.Get("season_id")
		if movie == "" {
			cat := q.Get("category")
			if cat == "" || cat == "*" || cat == "1" {
				if section == "series" {
					rows = append(rows, map[string]any{"id": "regular", "name": "Обычный сериал", "cover": posterBase + "regular.png"})
				} else {
					rows = append(rows, map[string]any{"id": "film", "name": "Демо фильм", "description": "Проверка воспроизведения фильма", "year": 2025, "duration": 90, "cmd": "/media/file_film.mpg", "cover": posterBase + "film.png"}, map[string]any{"id": "embedded", "name": "Сериал с эпизодами", "cmd": "/media/file_embedded.mpg", "series": []any{1, "2", 3}, "cover": posterBase + "embedded.png"}, map[string]any{"id": "lazy", "name": "Сериал Ministra", "is_series": "1", "cover": posterBase + "lazy.png"})
				}
			}
			if section == "vod" && (cat == "" || cat == "*" || cat == "2") {
				rows = append(rows, p.demoMovies(strings.TrimSuffix(p.Path, "/server/load.php")+"/misc/posters/")...)
			}
			if term := strings.ToLower(q.Get("search")); term != "" && !p.IgnoreSearch {
				found := rows[:0:0]
				for _, r := range rows {
					if strings.Contains(strings.ToLower(fmt.Sprint(r["name"])), term) || strings.Contains(strings.ToLower(fmt.Sprint(r["o_name"])), term) {
						found = append(found, r)
					}
				}
				rows = found
			}
		} else if movie == "regular" && section == "series" {
			for n := 1; n <= 2; n++ {
				rows = append(rows, map[string]any{"id": fmt.Sprintf("regular:%d", n), "name": fmt.Sprintf("Сезон %d", n), "cmd": fmt.Sprintf("ffrt4://regular/season/%d", n), "series": []any{1, "2"}})
			}
			reply(w, rows)
			return true // regular portals also return raw arrays
		} else if movie == "lazy" && section == "vod" {
			if season == "" {
				for n := 1; n <= 2; n++ {
					rows = append(rows, map[string]any{"id": fmt.Sprintf("lazy:%d", n), "video_id": "lazy", "name": fmt.Sprintf("Сезон %d", n), "is_season": n, "season_number": strconv.Itoa(n)})
				}
			} else if season == "lazy:1" || season == "lazy:2" {
				for n := 1; n <= 2; n++ {
					rows = append(rows, map[string]any{"id": fmt.Sprintf("lazy-%s-%d", strings.TrimPrefix(season, "lazy:"), n), "name": fmt.Sprintf("Эпизод %d", n), "is_episode": "1", "series_number": n})
				}
			}
		}
		n, _ := strconv.Atoi(q.Get("p"))
		n = max(1, n)
		from, to := (n-1)*p.PageSize, n*p.PageSize
		items := []map[string]any{}
		if from < len(rows) {
			items = rows[from:min(to, len(rows))]
		}
		reply(w, map[string]any{"data": items, "total_items": strconv.Itoa(len(rows)), "max_page_items": p.PageSize})
	case "create_link":
		cmd := q.Get("cmd")
		episode := q.Get("series")
		id := ""
		if cmd == "/media/file_film.mpg" && episode == "" {
			id = "film"
		}
		if cmd == "/media/file_embedded.mpg" && (episode == "1" || episode == "2" || episode == "3") {
			id = "embedded-" + episode
		}
		for season := 1; season <= 2; season++ {
			for ep := 1; ep <= 2; ep++ {
				if cmd == fmt.Sprintf("ffrt4://regular/season/%d", season) && episode == strconv.Itoa(ep) && section == "vod" {
					id = fmt.Sprintf("regular-%d-%d", season, ep)
				}
				lazy := fmt.Sprintf("lazy-%d-%d", season, ep)
				if cmd == "/media/file_"+lazy+".mpg" && episode == strconv.Itoa(ep) {
					id = lazy
				}
			}
		}
		if id == "" {
			reply(w, map[string]any{"error": "nothing_to_play"})
			return true
		}
		p.Links++
		stream := fmt.Sprintf("http://cdn.example/catalog/%s.mp4", id)
		if p.CatalogStream != "" {
			stream = p.CatalogStream
		}
		u, err := url.Parse(stream)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			reply(w, map[string]any{"error": "invalid_sample_stream"})
			return true
		}
		query := u.Query()
		query.Set("play_token", fmt.Sprintf("tmp%d", p.Links))
		u.RawQuery = query.Encode()
		reply(w, map[string]any{"cmd": "ffmpeg " + u.String()})
	default:
		return false
	}
	return true
}

var demoWords = [][]string{
	{"Тихая", "Северная", "Последняя", "Ночная", "Золотая", "Дальняя"},
	{"гавань", "звезда", "дорога", "река", "граница", "мечта", "станция", "весна"},
}
var demoGenres = []string{"Drama", "Comedy, Family", "Thriller", "Adventure, Fantasy", "Documentary", "Action, Crime"}

// demoMovies is a deterministic film shelf with the metadata real Ministra
// portals send, so the TV grid has something realistic to page through.
func (p *Portal) demoMovies(posterBase string) []map[string]any {
	rows := []map[string]any{}
	for i := 0; i < p.DemoMovies; i++ {
		a, b := demoWords[0][i%len(demoWords[0])], demoWords[1][(i/len(demoWords[0]))%len(demoWords[1])]
		name := a + " " + b
		if i >= len(demoWords[0])*len(demoWords[1]) {
			name += fmt.Sprintf(" %d", i/(len(demoWords[0])*len(demoWords[1]))+1)
		}
		id := fmt.Sprintf("demo%02d", i)
		rows = append(rows, map[string]any{
			"id": id, "name": name, "o_name": fmt.Sprintf("Demo Picture %d", i+1), "year": strconv.Itoa(1995 + i%31),
			"time": strconv.Itoa(85 + i*7%65), "genres_str": demoGenres[i%len(demoGenres)], "age": []string{"", "12+", "16+", "18+"}[i%4],
			"rating_imdb": fmt.Sprintf("%d.%d", 5+i%4, i*3%10), "rating_kinopoisk": fmt.Sprintf("%d.%03d", 5+(i+1)%4, i*137%1000),
			"director": "Режиссёр демо", "actors": "Актёр Один, Актриса Два",
			"description": "Демонстрационный фильм для проверки каталога на телевизоре: сетка, карточка и запуск.",
			"cmd":         "/media/file_film.mpg", "screenshot_uri": posterBase + id + ".jpg",
		})
	}
	return rows
}

func servePoster(w http.ResponseWriter, id string) {
	sum := sha1.Sum([]byte(id))
	top := color.RGBA{sum[0]/2 + 40, sum[1]/2 + 30, sum[2]/2 + 50, 255}
	img := image.NewRGBA(image.Rect(0, 0, 300, 450))
	for y := 0; y < 450; y++ {
		for x := 0; x < 300; x++ {
			k := uint8(y * 200 / 450)
			img.SetRGBA(x, y, color.RGBA{top.R - top.R*k/255, top.G - top.G*k/255, top.B - top.B*k/255, 255})
		}
	}
	w.Header().Set("Content-Type", "image/jpeg")
	jpeg.Encode(w, img, &jpeg.Options{Quality: 80})
}
