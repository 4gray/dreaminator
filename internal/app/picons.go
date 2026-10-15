package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/fetch"
	"dreaminator/internal/picon"
)

// Picons: channel logos as Enigma2 picon files, for the channels in the
// user's bouquets. Only files listed in our manifest are ever replaced or
// removed, so satellite picons next to them are safe.

type PiconStatus struct {
	Running   bool       `json:"running"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Dir       string     `json:"dir,omitempty"`
	Channels  int        `json:"channels"` // in bouquets
	Done      int        `json:"done"`     // have a picon
	NoLogo    int        `json:"noLogo"`   // playlist has no logo for them
	Failed    []string   `json:"failed,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type piconManifest struct {
	Dir   string            `json:"dir"`
	Files map[string]string `json:"files"` // file name -> logo URL it came from
}

func (a *App) manifestPath() string { return filepath.Join(a.cfg.DataDir, "picons.json") }

func (a *App) loadManifest() piconManifest {
	m := piconManifest{Files: map[string]string{}}
	if data, err := os.ReadFile(a.manifestPath()); err == nil {
		json.Unmarshal(data, &m)
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m
}

func (a *App) piconDir(s Settings) string {
	if s.PiconDir != "" {
		return s.PiconDir
	}
	cands := a.cfg.PiconCandidates
	if len(cands) == 0 {
		cands = picon.Candidates
	}
	return picon.DetectDir(cands)
}

type piconJob struct {
	file, url, name string
}

// SyncPicons brings the picon folder in line with the bouquets. Unchanged
// logos are not downloaded again unless force is set.
func (a *App) SyncPicons(ctx context.Context, force bool) error {
	if force {
		a.piconMu.Lock()
	} else if !a.piconMu.TryLock() {
		return nil
	}
	defer a.piconMu.Unlock()

	a.mu.Lock()
	s := a.st.Settings
	var jobs []piconJob
	seen := map[string]bool{}
	status := PiconStatus{Running: true}
	for _, b := range a.st.Bouquets {
		for _, r := range b.Channels {
			ch := a.channel(r.Source, r.ID)
			if ch == nil || seen[r.key()] {
				continue
			}
			seen[r.key()] = true
			status.Channels++
			if ch.Logo == "" {
				status.NoLogo++
				continue
			}
			jobs = append(jobs, piconJob{file: picon.FileName(enigma.EPGRef(r.key())), url: ch.Logo, name: ch.Name})
		}
	}
	if !s.Picons {
		a.mu.Unlock()
		// Turned off: take our picons away, leave everyone else's.
		m := a.loadManifest()
		for f := range m.Files {
			os.Remove(filepath.Join(m.Dir, f))
		}
		if len(m.Files) > 0 {
			data, _ := json.Marshal(piconManifest{Dir: m.Dir, Files: map[string]string{}})
			enigma.WriteFileAtomic(a.manifestPath(), data)
		}
		a.mu.Lock()
		a.st.Picons = PiconStatus{}
		a.save()
		a.mu.Unlock()
		return nil
	}
	a.st.Picons.Running = true
	a.mu.Unlock()

	dir := a.piconDir(s)
	status.Dir = dir
	err := a.syncPicons(ctx, dir, jobs, force, s, &status)
	now := time.Now()
	status.Running, status.UpdatedAt = false, &now
	if err != nil {
		status.Error = err.Error()
		a.log.Printf("пиконы: %v", err)
	} else {
		a.log.Printf("пиконы: %d из %d каналов в %s", status.Done, status.Channels, dir)
	}
	a.mu.Lock()
	a.st.Picons = status
	a.save()
	a.mu.Unlock()
	return err
}

func (a *App) syncPicons(ctx context.Context, dir string, jobs []piconJob, force bool, s Settings, status *PiconStatus) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("папка пиконов %s недоступна: %w", dir, err)
	}
	m := a.loadManifest()
	if m.Dir != "" && m.Dir != dir {
		// The picon folder moved (e.g. a USB stick with picons appeared):
		// take ours out of the old one.
		for f := range m.Files {
			os.Remove(filepath.Join(m.Dir, f))
		}
		m.Files = map[string]string{}
	}
	m.Dir = dir

	var mu sync.Mutex
	want := map[string]bool{}
	for _, j := range jobs {
		want[j.file] = true
	}
	queue := make(chan piconJob)
	var wg sync.WaitGroup
	opts := a.fetchOptions(s, 30*time.Second)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				path := filepath.Join(dir, j.file)
				mu.Lock()
				have := m.Files[j.file] == j.url
				mu.Unlock()
				if _, err := os.Stat(path); err == nil && have && !force {
					mu.Lock()
					status.Done++
					mu.Unlock()
					continue
				}
				err := a.makePicon(ctx, opts, j.url, path)
				mu.Lock()
				if err != nil {
					status.Failed = append(status.Failed, fmt.Sprintf("%s: %v", j.name, err))
				} else {
					m.Files[j.file] = j.url
					status.Done++
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		select {
		case queue <- j:
		case <-ctx.Done():
		}
	}
	close(queue)
	wg.Wait()

	for f := range m.Files {
		if !want[f] {
			os.Remove(filepath.Join(dir, f))
			delete(m.Files, f)
		}
	}
	sort.Strings(status.Failed)
	if len(status.Failed) > 10 {
		status.Failed = append(status.Failed[:10], fmt.Sprintf("… и ещё %d", len(status.Failed)-10))
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return enigma.WriteFileAtomic(a.manifestPath(), data)
}

func (a *App) makePicon(ctx context.Context, opts fetch.Options, url, path string) error {
	rc, err := fetch.Open(ctx, opts, url)
	if err != nil {
		return err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 3<<20))
	if err != nil {
		return err
	}
	png, err := picon.Render(data)
	if err != nil {
		return err
	}
	return enigma.WriteFileAtomic(path, png)
}
