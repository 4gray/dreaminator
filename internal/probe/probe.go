// Package probe checks a live stream the way the receiver will see it: does
// it open, how fast, and which codecs it carries. It needs no ffmpeg: it
// reads the first second or two of MPEG-TS (directly or behind HLS) and
// decodes only the PAT/PMT tables that name the elementary streams.
package probe

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Result struct {
	OK          bool     `json:"ok"`
	Error       string   `json:"error,omitempty"`
	Status      int      `json:"status,omitempty"`
	FirstByteMS int      `json:"firstByteMs,omitempty"`
	Mbps        float64  `json:"mbps,omitempty"` // download speed of the sample
	Format      string   `json:"format,omitempty"`
	Video       string   `json:"video,omitempty"`
	Audio       []string `json:"audio,omitempty"`
	Host        string   `json:"host,omitempty"` // where the stream finally came from (no tokens)
	Warnings    []string `json:"warnings,omitempty"`
}

type Options struct {
	Client    *http.Client
	UserAgent string
	Sample    int           // bytes of media to read (default 1.5 MB)
	MaxTime   time.Duration // overall budget (default 15 s)
}

func (o *Options) defaults() {
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	if o.Sample <= 0 {
		o.Sample = 1536 << 10
	}
	if o.MaxTime <= 0 {
		o.MaxTime = 15 * time.Second
	}
}

// Probe checks rawURL. It never returns an error: problems end up in the
// result so the tablet can show them as they are.
func Probe(ctx context.Context, rawURL string, o Options) Result {
	o.defaults()
	ctx, cancel := context.WithTimeout(ctx, o.MaxTime)
	defer cancel()
	if i := strings.IndexByte(rawURL, '#'); i >= 0 {
		rawURL = rawURL[:i] // Enigma2 options like #User-Agent=
	}
	var r Result
	if err := r.run(ctx, rawURL, o); err != nil {
		r.OK = false
		r.Error = describe(err)
	}
	r.addWarnings()
	return r
}

func describe(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "поток не ответил вовремя"
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return "поток не ответил вовремя"
		}
		var dns *net.DNSError
		text := strings.ToLower(ue.Err.Error())
		switch {
		case errors.As(err, &dns):
			return "нет связи с сервером потока: адрес сервера не найден"
		case errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(text, "refused") || strings.Contains(text, "errno 111"):
			// errno 111 is what QEMU user mode reports for a refused connect.
			return "нет связи с сервером потока: подключение отклонено (канал выключен или ссылка устарела)"
		case strings.Contains(text, "certificate") || strings.Contains(text, "tls"):
			return "нет связи с сервером потока: ошибка HTTPS-сертификата (можно отключить проверку в настройках)"
		}
		return "нет связи с сервером потока"
	}
	return err.Error()
}

func (r *Result) get(ctx context.Context, u string, o Options) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("неверная ссылка: %w", err)
	}
	if o.UserAgent != "" {
		req.Header.Set("User-Agent", o.UserAgent)
	}
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	r.Status = resp.StatusCode
	r.Host = resp.Request.URL.Host
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("сервер потока ответил %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return resp, nil
}

var codecsAttr = regexp.MustCompile(`CODECS="([^"]+)"`)
var bandwidthAttr = regexp.MustCompile(`BANDWIDTH=(\d+)`)

func (r *Result) run(ctx context.Context, u string, o Options) error {
	start := time.Now()
	resp, err := r.get(ctx, u, o)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	br := bufio.NewReaderSize(resp.Body, 64<<10)
	head, _ := br.Peek(16)
	r.FirstByteMS = int(time.Since(start).Milliseconds())
	ct := strings.ToLower(resp.Header.Get("Content-Type"))

	switch {
	case bytes.HasPrefix(bytes.TrimLeft(head, "\ufeff \r\n"), []byte("#EXTM3U")):
		return r.hls(ctx, resp.Request.URL, br, o, "")
	case strings.Contains(ct, "dash+xml") || strings.HasSuffix(resp.Request.URL.Path, ".mpd"):
		r.Format = "DASH (.mpd)"
		return errors.New("это поток DASH — проигрыватель Enigma2 его обычно не показывает")
	}
	return r.media(br, o, start)
}

// hls follows a master playlist to one variant and then to a segment.
func (r *Result) hls(ctx context.Context, base *url.URL, body io.Reader, o Options, codecs string) error {
	lines, err := readLines(body, 2<<20)
	if err != nil {
		return err
	}
	var variant, segment, mapURI string
	bestBW := -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF"):
			bw := 0
			if m := bandwidthAttr.FindStringSubmatch(line); m != nil {
				bw, _ = strconv.Atoi(m[1])
			}
			// The variant a 100 Mbit receiver would most likely pick: the best
			// one up to ~8 Mbit/s.
			better := bestBW < 0 || (bestBW > 8_000_000 && bw < bestBW) || (bw <= 8_000_000 && bw > bestBW)
			if i+1 < len(lines) && better {
				bestBW, variant = bw, lines[i+1]
				if m := codecsAttr.FindStringSubmatch(line); m != nil {
					codecs = m[1]
				}
			}
		case strings.HasPrefix(line, "#EXT-X-MAP"):
			if m := regexp.MustCompile(`URI="([^"]+)"`).FindStringSubmatch(line); m != nil {
				mapURI = m[1]
			}
		case line != "" && !strings.HasPrefix(line, "#"):
			segment = line // the newest segment: the one live playback starts near
		}
	}
	resolve := func(ref string) (string, error) {
		u, err := base.Parse(strings.TrimSpace(ref))
		if err != nil {
			return "", fmt.Errorf("плейлист HLS с битой ссылкой")
		}
		return u.String(), nil
	}
	if variant != "" {
		next, err := resolve(variant)
		if err != nil {
			return err
		}
		resp, err := r.get(ctx, next, o)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return r.hls(ctx, resp.Request.URL, resp.Body, o, codecs)
	}
	if segment == "" {
		return errors.New("плейлист HLS пустой")
	}
	r.codecsFromHLS(codecs)
	if mapURI != "" {
		r.Format = "HLS (fMP4)"
	}
	segURL, err := resolve(segment)
	if err != nil {
		return err
	}
	start := time.Now()
	resp, err := r.get(ctx, segURL, o)
	if err != nil {
		return fmt.Errorf("сегмент потока: %w", err)
	}
	defer resp.Body.Close()
	if r.Format == "" {
		r.Format = "HLS"
	}
	return r.media(bufio.NewReaderSize(resp.Body, 64<<10), o, start)
}

func readLines(r io.Reader, limit int64) ([]string, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		out = append(out, strings.TrimSpace(l))
	}
	return out, nil
}

// codecsFromHLS reads RFC 6381 codec ids (avc1.64001f, hvc1…, mp4a.40.2).
func (r *Result) codecsFromHLS(codecs string) {
	for _, c := range strings.Split(codecs, ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		switch {
		case strings.HasPrefix(c, "avc"):
			r.setVideo("H.264")
		case strings.HasPrefix(c, "hvc"), strings.HasPrefix(c, "hev"):
			r.setVideo("H.265 (HEVC)")
		case strings.HasPrefix(c, "mp4a"):
			r.addAudio("AAC")
		case strings.HasPrefix(c, "ac-3"):
			r.addAudio("AC-3")
		case strings.HasPrefix(c, "ec-3"):
			r.addAudio("E-AC-3")
		}
	}
}

func (r *Result) setVideo(v string) {
	if r.Video == "" {
		r.Video = v
	}
}

func (r *Result) addAudio(a string) {
	for _, x := range r.Audio {
		if x == a {
			return
		}
	}
	r.Audio = append(r.Audio, a)
}

// media reads a sample of the stream, measures it and decodes TS tables.
func (r *Result) media(br *bufio.Reader, o Options, start time.Time) error {
	buf := make([]byte, 0, o.Sample)
	chunk := make([]byte, 32<<10)
	deadline := time.Now().Add(4 * time.Second)
	for len(buf) < o.Sample && time.Now().Before(deadline) {
		n, err := br.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
		// MPEG-TS: the tables are usually in the first packets.
		if len(buf) >= 512<<10 && r.parseTS(buf) {
			break
		}
	}
	if secs := time.Since(start).Seconds(); secs > 0 && len(buf) > 0 {
		r.Mbps = float64(len(buf)) * 8 / secs / 1e6
	}
	if len(buf) < 188 {
		return errors.New("поток открылся, но данных почти нет")
	}
	switch {
	case isTS(buf):
		if r.Format == "" {
			r.Format = "MPEG-TS"
		} else if r.Format == "HLS" {
			r.Format = "HLS (MPEG-TS)"
		}
		r.parseTS(buf)
	case len(buf) > 8 && (string(buf[4:8]) == "ftyp" || string(buf[4:8]) == "moof" || string(buf[4:8]) == "styp"):
		if r.Format == "" || r.Format == "HLS" {
			r.Format = "fMP4"
		}
	case r.Format == "":
		r.Format = "неизвестный формат"
	}
	r.OK = true
	return nil
}

func isTS(b []byte) bool {
	for off := 0; off < 188 && off+188*3 < len(b); off++ {
		if b[off] == 0x47 && b[off+188] == 0x47 && b[off+376] == 0x47 {
			return true
		}
	}
	return false
}

// parseTS finds the PAT, then the PMT, and names the streams. It returns
// true once a video codec is known.
func (r *Result) parseTS(b []byte) bool {
	off := 0
	for off < 188 && !(b[off] == 0x47 && off+188 < len(b) && b[off+188] == 0x47) {
		off++
	}
	pmtPIDs := map[int]bool{}
	for pass := 0; pass < 2; pass++ {
		for p := off; p+188 <= len(b); p += 188 {
			pkt := b[p : p+188]
			if pkt[0] != 0x47 || pkt[1]&0x40 == 0 { // need payload_unit_start
				continue
			}
			pid := int(pkt[1]&0x1f)<<8 | int(pkt[2])
			payload := pkt[4:]
			switch (pkt[3] >> 4) & 0x3 {
			case 2:
				continue // adaptation only
			case 3:
				if int(payload[0])+1 > len(payload) {
					continue
				}
				payload = payload[int(payload[0])+1:]
			}
			if len(payload) < 1 || int(payload[0])+1 > len(payload) {
				continue
			}
			section := payload[int(payload[0])+1:] // pointer_field
			if len(section) < 8 {
				continue
			}
			length := int(section[1]&0x0f)<<8 | int(section[2])
			end := min(3+length-4, len(section)) // without CRC
			switch {
			case pass == 0 && pid == 0 && section[0] == 0x00:
				for i := 8; i+4 <= end; i += 4 {
					program := int(section[i])<<8 | int(section[i+1])
					if program != 0 {
						pmtPIDs[int(section[i+2]&0x1f)<<8|int(section[i+3])] = true
					}
				}
			case pass == 1 && pmtPIDs[pid] && section[0] == 0x02:
				if len(section) < 12 {
					continue
				}
				i := 12 + (int(section[10]&0x0f)<<8 | int(section[11]))
				for i+5 <= end {
					st := section[i]
					esInfo := int(section[i+3]&0x0f)<<8 | int(section[i+4])
					desc := section[i+5 : min(i+5+esInfo, end)]
					r.streamType(st, desc)
					i += 5 + esInfo
				}
				return r.Video != ""
			}
		}
		if len(pmtPIDs) == 0 {
			return false
		}
	}
	return r.Video != ""
}

func (r *Result) streamType(st byte, desc []byte) {
	switch st {
	case 0x01, 0x02:
		r.setVideo("MPEG-2")
	case 0x1b:
		r.setVideo("H.264")
	case 0x24:
		r.setVideo("H.265 (HEVC)")
	case 0x03, 0x04:
		r.addAudio("MPEG Audio (MP2)")
	case 0x0f, 0x11:
		r.addAudio("AAC")
	case 0x81:
		r.addAudio("AC-3")
	case 0x87:
		r.addAudio("E-AC-3")
	case 0x06:
		for i := 0; i+2 <= len(desc); i += 2 + int(desc[i+1]) {
			switch desc[i] {
			case 0x6a:
				r.addAudio("AC-3")
			case 0x7a:
				r.addAudio("E-AC-3")
			case 0x7b:
				r.addAudio("DTS")
			}
		}
	}
}

func (r *Result) addWarnings() {
	if r.Video == "H.265 (HEVC)" {
		r.Warnings = append(r.Warnings, "Видео в H.265: DM520 поддерживает HEVC до Full HD. Совместимость конкретного профиля и потока зависит от прошивки; проверьте воспроизведение на ресивере.")
	}
	if strings.Contains(r.Format, "fMP4") {
		r.Warnings = append(r.Warnings, "Поток в формате fMP4: старые версии GStreamer на ресивере могут его не показать — попробуйте проигрыватель ServiceApp в настройках.")
	}
	if r.OK && r.FirstByteMS > 5000 {
		r.Warnings = append(r.Warnings, "Сервер долго отвечает — канал будет включаться медленно.")
	}
	if r.OK && strings.HasPrefix(r.Format, "HLS") && r.Mbps > 0 && r.Mbps < 3 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Загрузка всего %.1f Мбит/с — возможны подтормаживания.", r.Mbps))
	}
}
