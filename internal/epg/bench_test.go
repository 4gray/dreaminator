package epg

// Cost of programme guides on a large provider guide (3000 channels, seven
// days, ≈178 MB like the user's): the nightly filter as it is, the same pass
// that also indexes every playlist channel, and reading our own epg.xml back
// into the in-memory index. Run on a computer:
//
//	go test ./internal/epg -run '^$' -bench . -benchtime 1x
//
// The generated guide is streamed through a pipe; to time the parsing alone,
// write it once (EPG_SYNTH_OUT=/path/synth.xml go test ./internal/epg -run
// WriteSynthGuide) and point EPG_BENCH_FILE at it, or at a real guide.
// On the receiver /tmp is RAM: the guide is generated on the fly, and the
// epg.xml benchmark reads the service's own file instead of writing one:
//
//	GOOS=linux GOARCH=mipsle GOMIPS=softfloat go test -c -o epg.test ./internal/epg
//	nice -n 10 ./epg.test -test.run '^$' -test.bench Filter -test.benchtime 1x
//	EPG_READ_FILE=/etc/dreaminator/epg.xml nice -n 10 ./epg.test -test.run '^$' -test.bench ReadGuide -test.benchtime 1x

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	benchChannels = 3000
	benchListed   = 300 // channels in the user's lists
)

var benchNow = time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC)

var benchTitles = []string{"Новости", "Время покажет", "Доброе утро", "Пусть говорят", "Вечерний Ургант", "Футбол. Премьер-лига",
	"Х/ф «Ирония судьбы, или С лёгким паром!»", "Т/с «Склифосовский»", "Документальный фильм", "Погода", "Мультфильмы",
	"Поле чудес", "Что? Где? Когда?", "Вести", "Местное время", "Сегодня", "Жди меня", "Модный приговор", "Давай поженимся!",
	"Контрольная закупка", "Утро России", "60 минут", "Т/с «След»", "Своя игра", "Следствие вели...", "Квартирный вопрос"}

const benchDesc = "Ведущие обсуждают главные события дня с экспертами и гостями студии. В программе — репортажи корреспондентов, " +
	"прямые включения и комментарии специалистов. "

// synthGuide writes a provider-sized XMLTV guide: seven days around benchNow.
func synthGuide(w io.Writer, channels int) {
	bw := bufio.NewWriterSize(w, 256<<10)
	bw.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tv generator-info-name=\"bench\">\n")
	for c := 0; c < channels; c++ {
		fmt.Fprintf(bw, "<channel id=\"ch%04d.tv\"><display-name lang=\"ru\">Канал %d HD</display-name><display-name>Channel %d</display-name>"+
			"<icon src=\"http://logo.example/%d.png\"/></channel>\n", c, c, c, c)
	}
	durations := []int{15, 30, 45, 60, 60, 90, 120, 40}
	desc := []rune(benchDesc)
	start := benchNow.Add(-24 * time.Hour).Truncate(time.Hour)
	end := start.Add(7 * 24 * time.Hour)
	for c := 0; c < channels; c++ {
		t := start
		for i := 0; t.Before(end); i++ {
			stop := t.Add(time.Duration(durations[(c+i)%len(durations)]) * time.Minute)
			title := benchTitles[(c*7+i*13)%len(benchTitles)]
			if i%3 == 0 {
				title = fmt.Sprintf("%s. %d серия", title, i%40+1)
			}
			fmt.Fprintf(bw, "<programme start=\"%s\" stop=\"%s\" channel=\"ch%04d.tv\"><title lang=\"ru\">%s</title>",
				t.Format("20060102150405 +0000"), stop.Format("20060102150405 +0000"), c, title)
			if i%10 < 7 {
				fmt.Fprintf(bw, "<desc lang=\"ru\">%s</desc>", string(desc[:20+(i*37)%(len(desc)-20)]))
			}
			bw.WriteString("<category lang=\"ru\">Развлекательные</category></programme>\n")
			t = stop
		}
	}
	bw.WriteString("</tv>\n")
	bw.Flush()
}

// benchGuide opens the guide: EPG_BENCH_FILE, or a generated one streamed
// through a pipe (the receiver's /tmp is RAM and too small for 178 MB).
func benchGuide(b *testing.B) (io.ReadCloser, func() int64) {
	b.Helper()
	if path := os.Getenv("EPG_BENCH_FILE"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		st, _ := f.Stat()
		return f, func() int64 { return st.Size() }
	}
	pr, pw := io.Pipe()
	cw := &counter{w: pw}
	go func() {
		synthGuide(cw, benchChannels)
		pw.Close()
	}()
	return pr, func() int64 { return cw.n }
}

type counter struct {
	w io.Writer
	n int64
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func benchWants(from, to int) []Want {
	var out []Want
	for c := from; c < to; c++ {
		w := Want{Ref: fmt.Sprintf("src/%d", c), Names: []string{fmt.Sprintf("Канал %d", c)}}
		if c%2 == 0 {
			w.TvgID = fmt.Sprintf("ch%04d.tv", c)
		}
		out = append(out, w)
	}
	return out
}

func heapMB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func allocMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.TotalAlloc) / (1 << 20)
}

func TestWriteSynthGuide(t *testing.T) {
	path := os.Getenv("EPG_SYNTH_OUT")
	if path == "" {
		t.Skip("EPG_SYNTH_OUT not set")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	synthGuide(f, benchChannels)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// BenchmarkFilterLists is the nightly pass as it is: 300 listed channels
// are copied out of the provider's guide.
func BenchmarkFilterLists(b *testing.B) {
	for i := 0; i < b.N; i++ {
		r, size := benchGuide(b)
		alloc := allocMB()
		res, err := Filter(r, "e0.", benchWants(0, benchListed), io.Discard, benchNow)
		r.Close()
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(size())/(1<<20), "guide-MB")
		b.ReportMetric(float64(res.Programmes), "written")
		b.ReportMetric(allocMB()-alloc, "alloc-MB")
	}
}

// BenchmarkFilterListsAndIndex is the same pass that also indexes every
// other channel of the playlist (name + time, 36 hours).
func BenchmarkFilterListsAndIndex(b *testing.B) {
	for i := 0; i < b.N; i++ {
		r, size := benchGuide(b)
		before, alloc := heapMB(), allocMB()
		idx := &Index{Wants: benchWants(benchListed, benchChannels), Builder: NewGuideBuilder(benchNow.Add(-2*time.Hour), benchNow.Add(34*time.Hour))}
		_, err := FilterIndex(r, "e0.", benchWants(0, benchListed), io.Discard, benchNow, idx)
		r.Close()
		if err != nil {
			b.Fatal(err)
		}
		g := idx.Builder.Build()
		b.ReportMetric(float64(size())/(1<<20), "guide-MB")
		b.ReportMetric(float64(len(idx.Refs)), "channels")
		b.ReportMetric(float64(g.Programmes()), "programmes")
		b.ReportMetric(float64(g.Bytes())/(1<<20), "index-MB")
		b.ReportMetric(heapMB()-before, "heap-MB")
		b.ReportMetric(allocMB()-alloc, "alloc-MB")
		runtime.KeepAlive(g)
	}
}

// BenchmarkReadGuide reads a filtered epg.xml for 2000 listed channels back
// into the index (at start, after each build, and when the day rolls on).
// EPG_READ_FILE reads an existing one (the receiver's) around the present.
func BenchmarkReadGuide(b *testing.B) {
	path, now := os.Getenv("EPG_READ_FILE"), time.Now()
	if path == "" {
		path, now = filepath.Join(b.TempDir(), "epg.xml"), benchNow
		f, err := os.Create(path)
		if err != nil {
			b.Fatal(err)
		}
		bw := bufio.NewWriterSize(f, 256<<10)
		bw.WriteString(Header)
		r, _ := benchGuide(b)
		if _, err := Filter(r, "e0.", benchWants(0, 2000), bw, benchNow); err != nil {
			b.Fatal(err)
		}
		r.Close()
		bw.WriteString(Footer)
		bw.Flush()
		f.Close()
	}
	st, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		before, alloc := heapMB(), allocMB()
		g, err := ReadGuide(path, now.Add(-3*time.Hour), now.Add(36*time.Hour), nil)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(st.Size())/(1<<20), "file-MB")
		b.ReportMetric(float64(g.Channels()), "channels")
		b.ReportMetric(float64(g.Programmes()), "programmes")
		b.ReportMetric(float64(g.Bytes())/(1<<20), "index-MB")
		b.ReportMetric(heapMB()-before, "heap-MB")
		b.ReportMetric(allocMB()-alloc, "alloc-MB")
		if p, ok := g.At("e0.ch0000.tv", benchNow); ok && !strings.HasPrefix(g.Describe(p), "Ведущие") {
			b.Fatalf("describe %+v: %q", p, g.Describe(p))
		}
		runtime.KeepAlive(g)
	}
}
