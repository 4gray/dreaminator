//go:debug x509usefallbackroots=1

// fakebox is a virtual Enigma2 receiver for testing Dreaminator without a
// Dreambox. It imitates the parts Dreaminator touches:
//
//   - OpenWebif API: statusinfo, servicelistreload, zap, powerstate
//   - Enigma2 bouquet parsing (strict: format problems are reported)
//   - EPGImport: reads *.sources.xml / channels.xml, downloads the XMLTV and
//     maps programmes onto services by sid:tsid:onid like Enigma2's EPG cache
//
// and shows a "TV" page with the channel list, infobar and a browser player.
package main

import (
	"bufio"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/crypto/x509roots/fallback"
)

//go:embed tv.html
var tvHTML []byte

type Event struct {
	Start time.Time `json:"start"`
	Stop  time.Time `json:"stop"`
	Title string    `json:"title"`
}

type Service struct {
	Ref  string `json:"ref"`
	Type int    `json:"type"`
	Key  string `json:"key"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name"`
	Now  *Event `json:"now,omitempty"`
	Next *Event `json:"next,omitempty"`
}

type Bouquet struct {
	File     string     `json:"file"`
	Name     string     `json:"name"`
	Ours     bool       `json:"ours"`
	Services []*Service `json:"services"`
}

type EPGInfo struct {
	RanAt         *time.Time `json:"ranAt,omitempty"`
	SourceFound   bool       `json:"sourceFound"`
	SourceEnabled bool       `json:"sourceEnabled"`
	URL           string     `json:"url,omitempty"`
	Events        int        `json:"events"`
	Channels      int        `json:"channels"`
	Error         string     `json:"error,omitempty"`
	Running       bool       `json:"running"`
}

type Box struct {
	e2dir, epgdir string

	mu         sync.Mutex
	standby    bool
	current    *Service
	bouquets   []*Bouquet
	problems   []string
	reloadedAt *time.Time
	epg        map[string][]Event
	epgInfo    EPGInfo
	journal    []string
}

func (b *Box) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05 ") + fmt.Sprintf(format, args...)
	log.Print(line)
	b.journal = append(b.journal, line)
	if len(b.journal) > 60 {
		b.journal = b.journal[len(b.journal)-60:]
	}
}

// ---------------------------------------------------------------- parsing

func hexKey(s string) string {
	s = strings.ToUpper(strings.TrimLeft(s, "0"))
	if s == "" {
		return "0"
	}
	return s
}

// parseRef parses a service reference the way Enigma2 does:
// type:flags:data0..data7:path:name (type/flags decimal, data hex).
func parseRef(s string) (*Service, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 11 {
		return nil, fmt.Errorf("в ссылке %d полей вместо 11+", len(parts))
	}
	typ, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("тип %q не число", parts[0])
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return nil, fmt.Errorf("флаги %q не число", parts[1])
	}
	for i := 2; i < 10; i++ {
		if _, err := strconv.ParseUint(parts[i], 16, 32); err != nil {
			return nil, fmt.Errorf("поле %d (%q) не hex", i, parts[i])
		}
	}
	path := parts[10]
	svc := &Service{
		Ref:  s,
		Type: typ,
		Key:  hexKey(parts[3]) + ":" + hexKey(parts[4]) + ":" + hexKey(parts[5]),
		URL:  strings.NewReplacer("%3a", ":", "%3A", ":").Replace(path),
		Name: strings.Join(parts[11:], ":"),
	}
	switch typ {
	case 1:
	case 4097, 5001, 5002:
		if !strings.HasPrefix(svc.URL, "http://") && !strings.HasPrefix(svc.URL, "https://") {
			return nil, fmt.Errorf("у IPTV-сервиса нет ссылки на поток")
		}
		if strings.Contains(path, "://") {
			return nil, errors.New("двоеточие в ссылке не экранировано как %3a")
		}
	default:
		return nil, fmt.Errorf("неизвестный тип сервиса %d", typ)
	}
	return svc, nil
}

var bouquetLine = regexp.MustCompile(`^#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "([^"/]+)" ORDER BY bouquet$`)

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), nil
}

func (b *Box) parseBouquet(file string) (*Bouquet, []string) {
	var problems []string
	bad := func(line int, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf(format, args...)))
	}
	lines, err := readLines(filepath.Join(b.e2dir, file))
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: файл не найден", file)}
	}
	bq := &Bouquet{File: file, Ours: strings.HasPrefix(file, "userbouquet.dreaminator_")}
	var last *Service
	for i, line := range lines {
		n := i + 1
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "#NAME "):
			if bq.Name != "" || len(bq.Services) > 0 {
				bad(n, "#NAME должен быть один и в начале")
			}
			bq.Name = strings.TrimPrefix(line, "#NAME ")
		case strings.HasPrefix(line, "#SERVICE "):
			svc, err := parseRef(strings.TrimPrefix(line, "#SERVICE "))
			if err != nil {
				bad(n, "%v", err)
				last = nil
				continue
			}
			bq.Services = append(bq.Services, svc)
			last = svc
		case strings.HasPrefix(line, "#DESCRIPTION "):
			if last == nil {
				bad(n, "#DESCRIPTION без #SERVICE перед ним")
				continue
			}
			last.Name = strings.TrimPrefix(line, "#DESCRIPTION ")
		default:
			bad(n, "непонятная строка %q", line)
		}
	}
	if bq.Name == "" {
		problems = append(problems, file+": нет строки #NAME")
	}
	return bq, problems
}

// Reload imitates /api/servicelistreload?mode=2.
func (b *Box) Reload() error {
	var problems []string
	var bouquets []*Bouquet
	lines, err := readLines(filepath.Join(b.e2dir, "bouquets.tv"))
	if err != nil {
		problems = append(problems, "bouquets.tv не найден")
	}
	seen := map[string]bool{}
	for i, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case strings.HasPrefix(t, "#NAME"):
			if i != 0 {
				problems = append(problems, "bouquets.tv: #NAME не в первой строке")
			}
		case strings.HasPrefix(t, "#SERVICE"):
			m := bouquetLine.FindStringSubmatch(t)
			if m == nil {
				problems = append(problems, fmt.Sprintf("bouquets.tv:%d: непонятная строка %q", i+1, t))
				continue
			}
			if seen[m[1]] {
				problems = append(problems, fmt.Sprintf("bouquets.tv: %s подключён дважды", m[1]))
				continue
			}
			seen[m[1]] = true
			bq, p := b.parseBouquet(m[1])
			problems = append(problems, p...)
			if bq != nil {
				bouquets = append(bouquets, bq)
			}
		default:
			problems = append(problems, fmt.Sprintf("bouquets.tv:%d: непонятная строка %q", i+1, t))
		}
	}
	orphans, _ := filepath.Glob(filepath.Join(b.e2dir, "userbouquet.dreaminator_*.tv"))
	for _, o := range orphans {
		if !seen[filepath.Base(o)] {
			problems = append(problems, filepath.Base(o)+": файл лежит, но не подключён в bouquets.tv")
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.bouquets, b.problems, b.reloadedAt = bouquets, problems, &now
	total := 0
	for _, bq := range bouquets {
		total += len(bq.Services)
	}
	if len(problems) > 0 {
		b.logf("перечитал списки: %d бакетов, %d сервисов, ОШИБОК: %d", len(bouquets), total, len(problems))
		return errors.New(problems[0])
	}
	b.logf("перечитал списки: %d бакетов, %d сервисов, ошибок нет", len(bouquets), total)
	return nil
}

// ---------------------------------------------------------------- EPGImport

type xSources struct {
	Cats []struct {
		Sources []struct {
			Channels string   `xml:"channels,attr"`
			Desc     string   `xml:"description"`
			URLs     []string `xml:"url"`
		} `xml:"source"`
	} `xml:"sourcecat"`
}

type xChannels struct {
	Channels []struct {
		ID  string `xml:"id,attr"`
		Ref string `xml:",chardata"`
	} `xml:"channel"`
}

func parseXMLTVTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse("20060102150405 -0700", s); err == nil {
		return t, nil
	}
	return time.Parse("20060102150405", s)
}

// RunEPGImport imitates the EPG-Importer plugin for the "Dreaminator" source.
func (b *Box) RunEPGImport(ctx context.Context) {
	b.mu.Lock()
	if b.epgInfo.Running {
		b.mu.Unlock()
		return
	}
	b.epgInfo.Running = true
	b.logf("EPGImport: запуск")
	b.mu.Unlock()

	info, epg := b.epgImport(ctx)
	now := time.Now()
	info.RanAt = &now

	b.mu.Lock()
	defer b.mu.Unlock()
	b.epgInfo = info
	if info.Error == "" {
		b.epg = epg
		b.logf("EPGImport: %d передач для %d каналов", info.Events, info.Channels)
	} else {
		b.logf("EPGImport: ошибка: %s", info.Error)
	}
}

func (b *Box) epgImport(ctx context.Context) (EPGInfo, map[string][]Event) {
	var info EPGInfo
	files, _ := filepath.Glob(filepath.Join(b.epgdir, "*.sources.xml"))
	var channelsFile string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var src xSources
		if err := xml.Unmarshal(data, &src); err != nil {
			info.Error = fmt.Sprintf("%s: %v", filepath.Base(f), err)
			return info, nil
		}
		for _, c := range src.Cats {
			for _, s := range c.Sources {
				if s.Desc == "Dreaminator" && len(s.URLs) > 0 {
					info.SourceFound = true
					info.URL = strings.TrimSpace(s.URLs[0])
					channelsFile = s.Channels
				}
			}
		}
	}
	if !info.SourceFound {
		info.Error = "источник Dreaminator не найден в /etc/epgimport"
		return info, nil
	}
	conf, _ := os.ReadFile(filepath.Join(b.epgdir, "epgimport.conf"))
	info.SourceEnabled = strings.Contains(string(conf), "Dreaminator")
	if !info.SourceEnabled {
		info.Error = "источник Dreaminator не отмечен галочкой (epgimport.conf)"
		return info, nil
	}
	if !filepath.IsAbs(channelsFile) {
		channelsFile = filepath.Join(b.epgdir, channelsFile)
	}
	data, err := os.ReadFile(channelsFile)
	if err != nil {
		info.Error = "нет файла каналов " + channelsFile
		return info, nil
	}
	var chans xChannels
	if err := xml.Unmarshal(data, &chans); err != nil {
		info.Error = "channels.xml: " + err.Error()
		return info, nil
	}
	keys := map[string][]string{}
	for _, c := range chans.Channels {
		parts := strings.Split(strings.TrimSpace(c.Ref), ":")
		if len(parts) < 10 {
			info.Error = fmt.Sprintf("channels.xml: плохая ссылка %q", c.Ref)
			return info, nil
		}
		keys[c.ID] = append(keys[c.ID], hexKey(parts[3])+":"+hexKey(parts[4])+":"+hexKey(parts[5]))
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		info.Error = "не скачал программу: " + err.Error()
		return info, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		info.Error = "программа: " + resp.Status
		return info, nil
	}
	var r io.Reader = bufio.NewReader(resp.Body)
	if strings.HasSuffix(info.URL, ".gz") {
		if r, err = gzip.NewReader(r); err != nil {
			info.Error = err.Error()
			return info, nil
		}
	}

	epg := map[string][]Event{}
	d := xml.NewDecoder(r)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			info.Error = "XMLTV: " + err.Error()
			return info, nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "programme" {
			continue
		}
		var p struct {
			Start   string `xml:"start,attr"`
			Stop    string `xml:"stop,attr"`
			Channel string `xml:"channel,attr"`
			Title   string `xml:"title"`
		}
		if err := d.DecodeElement(&p, &se); err != nil {
			info.Error = "XMLTV: " + err.Error()
			return info, nil
		}
		start, err1 := parseXMLTVTime(p.Start)
		stop, err2 := parseXMLTVTime(p.Stop)
		if err1 != nil || err2 != nil {
			continue
		}
		for _, k := range keys[p.Channel] {
			epg[k] = append(epg[k], Event{Start: start, Stop: stop, Title: p.Title})
			info.Events++
		}
	}
	for k := range epg {
		sort.Slice(epg[k], func(i, j int) bool { return epg[k][i].Start.Before(epg[k][j].Start) })
	}
	info.Channels = len(epg)
	return info, epg
}

func (b *Box) nowNext(key string) (*Event, *Event) {
	now := time.Now()
	events := b.epg[key]
	for i, e := range events {
		if !now.Before(e.Start) && now.Before(e.Stop) {
			cur := e
			if i+1 < len(events) {
				next := events[i+1]
				return &cur, &next
			}
			return &cur, nil
		}
		if e.Start.After(now) {
			next := e
			return nil, &next
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------- OpenWebif

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func (b *Box) statusinfo(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := map[string]any{"inStandby": strconv.FormatBool(b.standby), "currservice_name": "", "currservice_serviceref": ""}
	if b.current != nil {
		res["currservice_name"] = b.current.Name
		res["currservice_serviceref"] = b.current.Ref
	}
	writeJSON(w, res)
}

func (b *Box) servicelistreload(w http.ResponseWriter, r *http.Request) {
	if mode := r.URL.Query().Get("mode"); mode != "2" && mode != "0" {
		writeJSON(w, map[string]any{"result": false, "message": "mode=" + mode + " не перечитывает бакеты"})
		return
	}
	if err := b.Reload(); err != nil {
		writeJSON(w, map[string]any{"result": false, "message": "ошибка формата: " + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"result": true, "message": "reloaded bouquets"})
}

func (b *Box) zap(ref string) error {
	svc, err := parseRef(ref)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.current = svc
	if b.standby {
		b.logf("переключение на «%s» в режиме ожидания — экран выключен", svc.Name)
	} else {
		b.logf("включён канал «%s»", svc.Name)
	}
	return nil
}

func (b *Box) apiZap(w http.ResponseWriter, r *http.Request) {
	if err := b.zap(r.URL.Query().Get("sRef")); err != nil {
		writeJSON(w, map[string]any{"result": false, "message": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"result": true, "message": "Active service is now '" + b.current.Name + "'"})
}

func (b *Box) powerstate(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch r.URL.Query().Get("newstate") {
	case "0":
		b.standby = !b.standby
	case "4":
		b.standby = false
	case "5":
		b.standby = true
	}
	b.logf("питание: %s", map[bool]string{true: "режим ожидания", false: "включён"}[b.standby])
	writeJSON(w, map[string]any{"result": true, "instandby": b.standby})
}

// ---------------------------------------------------------------- TV page

func (b *Box) tvState(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fill := func(s *Service) *Service {
		if s == nil {
			return nil
		}
		cp := *s
		cp.Now, cp.Next = b.nowNext(s.Key)
		return &cp
	}
	var bouquets []Bouquet
	for _, bq := range b.bouquets {
		out := Bouquet{File: bq.File, Name: bq.Name, Ours: bq.Ours}
		for _, s := range bq.Services {
			out.Services = append(out.Services, fill(s))
		}
		bouquets = append(bouquets, out)
	}
	writeJSON(w, map[string]any{
		"standby":    b.standby,
		"current":    fill(b.current),
		"bouquets":   bouquets,
		"problems":   b.problems,
		"reloadedAt": b.reloadedAt,
		"epg":        b.epgInfo,
		"journal":    b.journal,
	})
}

var uriAttr = regexp.MustCompile(`URI="([^"]+)"`)

// stream proxies streams for the browser player (providers rarely send CORS
// headers) and rewrites HLS playlists so segments go through the proxy too.
func stream(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	req.Header.Set("User-Agent", "VLC/3.0.20 LibVLC/3.0.20")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	base := resp.Request.URL
	proxied := func(ref string) string {
		u, err := base.Parse(strings.TrimSpace(ref))
		if err != nil {
			return ref
		}
		return "/tv/stream?url=" + url.QueryEscape(u.String())
	}
	if strings.Contains(strings.ToLower(ct), "mpegurl") || strings.HasSuffix(base.Path, ".m3u8") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		var out strings.Builder
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimRight(line, "\r")
			switch {
			case line == "":
			case strings.HasPrefix(line, "#"):
				line = uriAttr.ReplaceAllStringFunc(line, func(m string) string {
					return `URI="` + proxied(uriAttr.FindStringSubmatch(m)[1]) + `"`
				})
			default:
				line = proxied(line)
			}
			out.WriteString(line + "\n")
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		io.WriteString(w, out.String())
		return
	}
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func main() {
	webif := flag.String("webif", ":80", "адрес фальшивого OpenWebif")
	ui := flag.String("ui", ":8090", "адрес страницы-телевизора")
	e2dir := flag.String("e2dir", "/etc/enigma2", "")
	epgdir := flag.String("epgdir", "/etc/epgimport", "")
	epgEvery := flag.Duration("epg-every", 0, "запускать EPGImport автоматически с этим интервалом (0 — только кнопкой)")
	flag.Parse()

	b := &Box{e2dir: *e2dir, epgdir: *epgdir, epg: map[string][]Event{}}
	b.Reload()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/statusinfo", b.statusinfo)
	mux.HandleFunc("GET /api/servicelistreload", b.servicelistreload)
	mux.HandleFunc("GET /api/zap", b.apiZap)
	mux.HandleFunc("GET /api/powerstate", b.powerstate)

	mux.HandleFunc("GET /tv/state", b.tvState)
	mux.HandleFunc("GET /tv/stream", stream)
	mux.HandleFunc("POST /tv/zap", func(w http.ResponseWriter, r *http.Request) {
		if err := b.zap(r.URL.Query().Get("ref")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	mux.HandleFunc("POST /tv/power", func(w http.ResponseWriter, r *http.Request) {
		r.URL.RawQuery = "newstate=0"
		b.powerstate(w, r)
	})
	mux.HandleFunc("POST /tv/epgimport", func(w http.ResponseWriter, r *http.Request) {
		go b.RunEPGImport(context.Background())
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(tvHTML)
	})

	if *epgEvery > 0 {
		go func() {
			for range time.Tick(*epgEvery) {
				b.RunEPGImport(context.Background())
			}
		}()
	}

	go func() { log.Fatal(http.ListenAndServe(*ui, mux)) }()
	log.Printf("fakebox: OpenWebif на %s, телевизор на %s", *webif, *ui)
	log.Fatal(http.ListenAndServe(*webif, mux))
}
