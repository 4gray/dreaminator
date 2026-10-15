package app

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/epg"
	"dreaminator/internal/fetch"
)

func (a *App) webif(s Settings) enigma.WebIf {
	return enigma.WebIf{BaseURL: s.WebifURL, User: s.WebifUser, Password: s.WebifPassword}
}

// buildBouquets resolves bouquet refs against the channel cache; callers hold a.mu.
func (a *App) buildBouquets() []enigma.Bouquet {
	out := make([]enigma.Bouquet, 0, len(a.st.Bouquets))
	for _, b := range a.st.Bouquets {
		eb := enigma.Bouquet{ID: b.ID, Name: b.Name}
		for _, r := range b.Channels {
			if ch := a.channel(r.Source, r.ID); ch != nil {
				eb.Services = append(eb.Services, enigma.Service{Key: r.key(), URL: ch.URL, Name: ch.Name})
			}
		}
		out = append(out, eb)
	}
	return out
}

// Apply writes the bouquets to Enigma2 and asks it to reload them.
func (a *App) Apply(ctx context.Context) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()

	a.mu.Lock()
	seq := a.seq
	bouquets := a.buildBouquets()
	s := a.st.Settings
	a.mu.Unlock()

	err := enigma.WriteBouquets(a.cfg.E2Dir, s.ServiceType, bouquets, s.BouquetsOnTop)
	if err != nil {
		err = fmt.Errorf("не удалось записать списки: %w", err)
	} else if w := a.webif(s); w.Enabled() {
		err = w.ReloadBouquets(ctx)
	}

	a.mu.Lock()
	now := time.Now()
	if err != nil {
		a.st.LastApplyError = err.Error()
	} else {
		a.st.LastApplyError = ""
		a.st.LastApply = &now
		if a.seq == seq {
			a.st.Dirty = false
		}
	}
	a.save()
	a.mu.Unlock()

	if err == nil {
		a.bgWork.Add(1)
		go func() {
			defer a.bgWork.Done()
			a.RebuildEPG(a.bg, false)
		}()
	}
	return err
}

func (a *App) Zap(ctx context.Context, sourceID, channelID string) error {
	a.mu.Lock()
	ch := a.channel(sourceID, channelID)
	s := a.st.Settings
	var svc enigma.Service
	if ch != nil {
		svc = enigma.Service{Key: sourceID + "/" + channelID, URL: ch.URL, Name: ch.Name}
	}
	a.mu.Unlock()
	if ch == nil {
		return ErrNotFound
	}
	return a.webif(s).Zap(ctx, enigma.ServiceRef(s.ServiceType, svc))
}

type BoxView struct {
	OK      bool   `json:"ok"`
	Standby bool   `json:"standby"`
	Current string `json:"current,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (a *App) Box(ctx context.Context) BoxView {
	a.mu.Lock()
	s := a.st.Settings
	a.mu.Unlock()
	st, err := a.webif(s).Status(ctx)
	if err != nil {
		return BoxView{Error: err.Error()}
	}
	return BoxView{OK: true, Standby: st.Standby, Current: st.Current}
}

// ---- EPG ----

type epgJob struct {
	URL   string
	Wants []epg.Want
}

// epgJobs groups the channels in bouquets by guide URL. A channel whose
// source lists several guides appears in each of them; buildEPG takes the
// first guide that knows it.
func (a *App) epgJobs() []epgJob {
	var jobs []epgJob
	pos := map[string]int{}
	seen := map[string]bool{}
	for _, b := range a.st.Bouquets {
		for _, r := range b.Channels {
			if seen[r.key()] {
				continue
			}
			ch, src := a.channel(r.Source, r.ID), a.source(r.Source)
			if ch == nil || src == nil {
				continue
			}
			seen[r.key()] = true
			want := epg.Want{
				Ref:   enigma.EPGRef(r.key()),
				TvgID: ch.TvgID,
				Names: []string{ch.Name, ch.TvgName},
				Shift: time.Duration(ch.TvgShift * float64(time.Hour)),
			}
			for _, u := range src.epgURLs() {
				i, ok := pos[u]
				if !ok {
					i = len(jobs)
					pos[u] = i
					jobs = append(jobs, epgJob{URL: u})
				}
				jobs[i].Wants = append(jobs[i].Wants, want)
			}
		}
	}
	return jobs
}

func jobsSig(jobs []epgJob) string {
	h := sha1.New()
	for _, j := range jobs {
		fmt.Fprintf(h, "%s\n", j.URL)
		for _, w := range j.Wants {
			fmt.Fprintf(h, "%s|%s|%q|%s\n", w.Ref, w.TvgID, w.Names, w.Shift)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *App) EPGPath() string { return filepath.Join(a.cfg.DataDir, "epg.xml") }

// RebuildEPG downloads the guides of all sources in use, keeps only our
// channels and publishes the result to EPGImport. Unless forced it is skipped
// when nothing changed since the last successful build today.
func (a *App) RebuildEPG(ctx context.Context, force bool) error {
	// An explicit request waits for a build in progress; an automatic one just
	// skips, since the running build already covers it.
	if force {
		a.epgMu.Lock()
	} else if !a.epgMu.TryLock() {
		return nil
	}
	defer a.epgMu.Unlock()

	a.mu.Lock()
	jobs := a.epgJobs()
	sig := jobsSig(jobs)
	s := a.st.Settings
	e := a.st.EPG
	if !force && e.Error == "" && e.Sig == sig && e.UpdatedAt != nil && time.Since(*e.UpdatedAt) < 20*time.Hour {
		a.mu.Unlock()
		return nil
	}
	a.st.EPG.Running = true
	a.mu.Unlock()

	status, err := a.buildEPG(ctx, jobs, s)
	status.Sig = sig

	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		status.Error = err.Error()
		a.log.Printf("программа передач: %v", err)
	}
	a.st.EPG = status
	a.save()
	return err
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}

func (a *App) buildEPG(ctx context.Context, jobs []epgJob, s Settings) (EPGStatus, error) {
	now := time.Now()
	status := EPGStatus{UpdatedAt: &now}
	wanted := map[string]bool{}
	for _, j := range jobs {
		for _, w := range j.Wants {
			wanted[w.Ref] = true
		}
	}
	status.Wanted = len(wanted)

	path := a.EPGPath()
	f, err := os.Create(path + ".tmp")
	if err != nil {
		return status, err
	}
	bw := bufio.NewWriterSize(f, 256<<10)
	bw.WriteString(epg.Header)

	mapping := map[string][]string{}
	matched := map[string]bool{}
	var problems []string
	for i, j := range jobs {
		// Channels an earlier guide already covered are not looked up again,
		// so backup guides never duplicate programmes.
		var wants []epg.Want
		for _, w := range j.Wants {
			if !matched[w.Ref] {
				wants = append(wants, w)
			}
		}
		if len(wants) == 0 {
			continue
		}
		rc, err := fetch.Open(ctx, a.fetchOptions(s, 20*time.Minute), j.URL)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", hostOf(j.URL), err))
			continue
		}
		res, err := epg.Filter(rc, fmt.Sprintf("e%d.", i), wants, bw, time.Now())
		rc.Close()
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", hostOf(j.URL), err))
		}
		for id, refs := range res.Map {
			mapping[id] = append(mapping[id], refs...)
			for _, r := range refs {
				matched[r] = true
			}
		}
		status.Programmes += res.Programmes
	}
	status.Matched = len(matched)

	bw.WriteString(epg.Footer)
	if err := bw.Flush(); err != nil {
		f.Close()
		return status, err
	}
	f.Close()
	if err := os.Rename(path+".tmp", path); err != nil {
		return status, err
	}

	if err := enigma.WriteEPGImport(a.cfg.EPGImportDir, a.cfg.PublicURL+"/epg.xml", mapping); err != nil {
		problems = append(problems, err.Error())
	} else if err := enigma.EnableEPGImportSource(ctx, a.cfg.EPGImportDir); err != nil {
		a.log.Printf("не удалось включить источник в EPGImport: %v", err)
	}
	a.log.Printf("программа передач: %d из %d каналов, %d передач", status.Matched, status.Wanted, status.Programmes)
	if len(problems) > 0 {
		return status, errors.New(strings.Join(problems, "; "))
	}
	return status, nil
}

// ---- nightly refresh ----

func (a *App) Start(ctx context.Context) {
	a.bg = ctx
	go a.scheduler(ctx)
}

func (a *App) scheduler(ctx context.Context) {
	// Give the network a moment after boot, then catch up if the receiver
	// was off at refresh time.
	select {
	case <-ctx.Done():
		return
	case <-time.After(45 * time.Second):
	}
	a.mu.Lock()
	last, hour := a.st.LastAutoRefresh, a.st.Settings.RefreshHour
	a.mu.Unlock()
	if hour >= 0 && (last == nil || time.Since(*last) > 26*time.Hour) {
		a.autoRefresh(ctx)
	}

	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			a.mu.Lock()
			last, hour := a.st.LastAutoRefresh, a.st.Settings.RefreshHour
			a.mu.Unlock()
			if hour >= 0 && now.Hour() == hour && (last == nil || now.Sub(*last) > 2*time.Hour) {
				a.autoRefresh(ctx)
			}
		}
	}
}

func (a *App) autoRefresh(ctx context.Context) {
	a.mu.Lock()
	ids := make([]string, 0, len(a.st.Sources))
	for _, s := range a.st.Sources {
		ids = append(ids, s.ID)
	}
	now := time.Now()
	a.st.LastAutoRefresh = &now
	a.save()
	a.mu.Unlock()
	if len(ids) == 0 {
		return
	}
	a.log.Printf("ночное обновление: %d плейлистов", len(ids))
	for _, id := range ids {
		a.RefreshSource(ctx, id)
	}
	a.mu.Lock()
	dirty := a.st.Dirty
	a.mu.Unlock()
	// Stream URLs may have changed; push them so the TV keeps working.
	if dirty {
		if err := a.Apply(ctx); err != nil {
			a.log.Printf("ночное обновление: %v", err)
		}
		return
	}
	a.RebuildEPG(ctx, true)
}

// Wait blocks until background work started by Apply has finished.
func (a *App) Wait() { a.bgWork.Wait() }
