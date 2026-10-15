package tmdb

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Matching is deliberately conservative: a wrong poster and description are
// worse than the provider's own. A candidate wins only with a near-exact title
// and a clear lead over the runner-up; otherwise the entry stays undecorated
// and the viewer can choose by hand in the WebUI.
const (
	MinScore = 80 // exact title plus a compatible (or unknown) year
	Margin   = 10 // lead over the next candidate, e.g. same title one year apart
)

// Query is what the provider says about one entry.
type Query struct {
	Type   string   // movie | tv
	Titles []string // normalized forms of every title we know, best first
	Search []string // cleaned texts to send to TMDB search, best first
	Year   int
}

// NewQuery cleans the provider's title and original title. year is the
// provider's year field; a "(2019)" in a title is used when it is empty.
func NewQuery(typ, name, original, year string) Query {
	q := Query{Type: typ, Year: parseYear(year)}
	series := typ == "tv"
	for _, s := range []string{name, original} {
		// "Матрица / The Matrix" carries both titles in one field.
		for _, part := range splitTitles(s) {
			clean, y := Clean(part, series)
			if q.Year == 0 {
				q.Year = y
			}
			// "Robots 2023": the release year glued to the title. Only the
			// provider's own year is dropped, so "Blade Runner 2049" stays.
			variants := []string{clean}
			if q.Year > 0 {
				if t := strings.TrimSpace(strings.TrimSuffix(clean, " "+strconv.Itoa(q.Year))); t != clean && t != "" {
					variants = []string{t, clean}
				}
			}
			for _, v := range variants {
				n := Normalize(v)
				if n == "" || contains(q.Titles, n) {
					continue
				}
				q.Titles = append(q.Titles, n)
				if len(q.Search) < 3 {
					q.Search = append(q.Search, v)
				}
			}
		}
	}
	return q
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Only spaced separators split: "AC/DC" stays one title.
var titleSepRe = regexp.MustCompile(`\s+[/|]\s+`)

func splitTitles(s string) []string {
	var out []string
	for _, p := range titleSepRe.Split(s, 4) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseYear(s string) int {
	s = strings.TrimSpace(s)
	if len(s) >= 4 {
		s = s[:4]
	}
	y, err := strconv.Atoi(s)
	if err != nil || y < 1870 || y > 2200 {
		return 0
	}
	return y
}

// Tags providers put into titles that say nothing about the work itself.
var qualityTags = map[string]bool{
	"hd": true, "fhd": true, "uhd": true, "sd": true, "hq": true, "4k": true, "8k": true, "2k": true,
	"1080p": true, "1080i": true, "720p": true, "2160p": true, "480p": true, "360p": true,
	"hdr": true, "hdr10": true, "dv": true, "hevc": true, "h264": true, "h265": true, "x264": true, "x265": true, "avc": true,
	"3d": true, "bdrip": true, "bdremux": true, "remux": true, "webrip": true, "web": true, "dl": true, "webdl": true,
	"hdrip": true, "dvdrip": true, "tvrip": true, "satrip": true, "camrip": true, "ts": true,
	"ac3": true, "dts": true, "aac": true, "rus": true, "eng": true, "ukr": true, "ua": true, "ru": true, "en": true,
	"dub": true, "mvo": true, "avo": true, "sub": true, "subs": true, "multi": true,
	"лицензия": true, "дубляж": true, "дублированный": true, "звук": true, "субтитры": true,
}

// Quality tags removed even without brackets, at the end of a title.
var bareTags = map[string]bool{
	"hd": true, "fhd": true, "uhd": true, "4k": true, "8k": true, "1080p": true, "1080i": true, "720p": true,
	"2160p": true, "480p": true, "hdr": true, "hevc": true, "h264": true, "h265": true, "x264": true, "x265": true,
}

var (
	bracketRe = regexp.MustCompile(`[(\[{]([^()\[\]{}]*)[)\]}]`)
	seasonRes = []*regexp.Regexp{
		// "American Hostage s01", "Show S1", "Show s01e02"
		regexp.MustCompile(`(?i)[ \-–—:,.]+s\d{1,2}(?:\s?e\d{1,3})?$`),
		// "Show сезон 1", "Show - Season 2", "Show (сезоны 1-3)"
		regexp.MustCompile(`(?i)[ \-–—:,.(\[]+(?:сезоны|сезон|seasons|season)\s*\d+(?:\s*[-–,]\s*\d+)*\s*[)\]]?$`),
		// "Show 1 сезон", "Show 2-й сезон", "Show (1-3 сезон)"
		regexp.MustCompile(`(?i)[ \-–—:,.(\[]+\d+(?:\s*[-–,]\s*\d+)*\s*(?:-?(?:й|ий|ый)\s*)?(?:сезоны|сезонов|сезона|сезон|seasons|season)\s*[)\]]?$`),
		// "Show (8 серий)", "Show (17 серий)", "Show 10 episodes"
		regexp.MustCompile(`(?i)[ \-–—:,.(\[]+\d+\s*(?:серий|серии|серия|эпизодов|эпизода|episodes|eps)\s*[)\]]?$`),
		// "Смешарики (сериал 2004 – 2012)", "(мини-сериал)"
		regexp.MustCompile(`(?i)\s*[(\[](?:мини-)?(?:сериал|serial|tv series|mini-series)[^)\]]*[)\]]$`),
	}
)

// Clean removes what providers add to a title: quality and format tags in
// brackets ("[HD]", "(1080p)"), a bracketed year (returned separately), bare
// trailing quality tags and, for series, season suffixes ("s01", "сезон 1",
// "1 сезон"). The result keeps its case and letters, for searching.
func Clean(s string, series bool) (string, int) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	year := 0
	s = bracketRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(m[1 : len(m)-1])
		if y := parseYear(inner); y > 0 && len(inner) == 4 {
			year = y
			return " "
		}
		if allTags(inner) {
			return " "
		}
		return m
	})
	s = strings.Join(strings.Fields(s), " ")
	for {
		f := strings.Fields(s)
		if len(f) < 2 || !bareTags[strings.ToLower(strings.Trim(f[len(f)-1], ".,-–—:"))] {
			break
		}
		s = strings.Join(f[:len(f)-1], " ")
	}
	if series {
		for changed := true; changed; {
			changed = false
			for _, re := range seasonRes {
				if t := strings.TrimSpace(re.ReplaceAllString(s, "")); t != s && t != "" {
					s, changed = t, true
				}
			}
		}
	}
	return strings.Trim(s, " -–—:,."), year
}

func allTags(s string) bool {
	tokens := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if !qualityTags[t] {
			return false
		}
	}
	return true
}

var fold = map[rune]string{
	'ё': "е", 'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae", 'ç': "c",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'œ': "oe", 'ù': "u", 'ú': "u",
	'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'ß': "ss", 'ł': "l", 'ś': "s", 'ź': "z", 'ż': "z",
	'ć': "c", 'ń': "n", 'ą': "a", 'ę': "e", 'š': "s", 'č': "c", 'ž': "z", 'ř': "r", 'ğ': "g", 'ı': "i", 'ş': "s",
}

// Normalize makes titles comparable: lower case, ё→е, Latin accents folded,
// apostrophes dropped, other punctuation turned into spaces, spaces collapsed.
func Normalize(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		switch r {
		case '\'', '’', '‘', '`', 'ʼ', '´':
			continue
		}
		if f, ok := fold[r]; ok {
			b.WriteString(f)
			space = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

var articles = []string{"the ", "a ", "an ", "le ", "la ", "les ", "der ", "die ", "das ", "el ", "il ", "lo "}

func stripArticle(s string) string {
	for _, a := range articles {
		if strings.HasPrefix(s, a) {
			return s[len(a):]
		}
	}
	return s
}

// Candidate is a TMDB record with its score against a Query.
type Candidate struct {
	Title
	Score int
}

// Rank scores every title against q, best first (TMDB's order breaks ties).
func Rank(q Query, titles []Title) []Candidate {
	out := make([]Candidate, 0, len(titles))
	for _, t := range titles {
		out = append(out, Candidate{Title: t, Score: Score(q, t)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// Score rates a TMDB record from about -50 to 100: title 60, year 30, type 10.
func Score(q Query, t Title) int {
	score := titlePoints(q, t) + yearPoints(q.Year, t.Year(), q.Type)
	if t.Type == q.Type {
		score += 10
	} else {
		score -= 20
	}
	return score
}

func titlePoints(q Query, t Title) int {
	best := 0
	for _, ct := range []string{Normalize(t.Title), Normalize(t.OriginalTitle)} {
		if ct == "" {
			continue
		}
		for _, qt := range q.Titles {
			p := 0
			switch {
			case qt == ct:
				p = 60
			case stripArticle(qt) == stripArticle(ct):
				p = 55
			default:
				// Partial overlap can never reach MinScore on its own.
				p = int(40*dice(qt, ct) + 0.5)
			}
			best = max(best, p)
		}
	}
	return best
}

func yearPoints(q, c int, typ string) int {
	if q == 0 {
		return 10
	}
	if c == 0 {
		return 0
	}
	d := q - c
	if d < 0 {
		d = -d
	}
	switch {
	case d == 0:
		return 30
	case d == 1:
		return 20
	case typ == "tv" && q > c:
		return 10 // providers often date a show by its latest season
	case d == 2:
		return 5
	}
	return -30
}

// dice is the overlap of two titles' word sets, 0..1.
func dice(a, b string) float64 {
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	common := 0
	for w := range wa {
		if wb[w] {
			common++
		}
	}
	return 2 * float64(common) / float64(len(wa)+len(wb))
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// Pick returns the winner of ranked candidates, or nil with the reason:
// "none" when nothing is convincing, "ambiguous" when two are.
func Pick(ranked []Candidate) (*Candidate, string) {
	if len(ranked) == 0 || ranked[0].Score < MinScore {
		return nil, "none"
	}
	for _, c := range ranked[1:] {
		if c.ID != ranked[0].ID && ranked[0].Score-c.Score < Margin {
			return nil, "ambiguous"
		}
	}
	return &ranked[0], "matched"
}
