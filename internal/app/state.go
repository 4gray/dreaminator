// Package app holds Dreaminator's state (sources, bouquets, settings) and the
// workflows around it: refreshing playlists, pushing bouquets to Enigma2 and
// building the EPG.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"dreaminator/internal/enigma"
	"dreaminator/internal/playlist"
	"dreaminator/internal/stalker"
)

var ErrNotFound = errors.New("не найдено")

type Config struct {
	DataDir      string
	E2Dir        string
	EPGImportDir string
	PluginDir    string // where Enigma2 plugins live, to detect EPGImport
	PublicURL    string // how EPGImport on the box reaches us
	Port         string // web UI port, for the tablet address
	// PiconCandidates overrides the folders searched for picons (tests, dev).
	PiconCandidates []string
	Version         string
}

type Settings struct {
	ServiceType   int    `json:"serviceType"`   // 4097 GStreamer, 5001/5002 ServiceApp
	StreamFormat  string `json:"streamFormat"`  // Xtream: ts | m3u8
	RefreshHour   int    `json:"refreshHour"`   // -1 disables the nightly refresh
	BouquetsOnTop bool   `json:"bouquetsOnTop"` // our bouquets before or after the existing ones
	WebifURL      string `json:"webifUrl"`
	WebifUser     string `json:"webifUser"`
	WebifPassword string `json:"webifPassword"`
	InsecureTLS   bool   `json:"insecureTls"`
	UserAgent     string `json:"userAgent"`
	Picons        bool   `json:"picons"`   // channel logos as Enigma2 picons
	PiconDir      string `json:"piconDir"` // "" = find the folder Enigma2 uses
}

func DefaultSettings() Settings {
	return Settings{
		ServiceType:   4097,
		StreamFormat:  "ts",
		RefreshHour:   4,
		BouquetsOnTop: true,
		WebifURL:      "http://127.0.0.1",
		UserAgent:     "VLC/3.0.20 LibVLC/3.0.20",
		Picons:        true,
	}
}

func (s *Settings) normalize() {
	switch s.ServiceType {
	case 4097, 5001, 5002:
	default:
		s.ServiceType = 4097
	}
	if s.StreamFormat != "m3u8" {
		s.StreamFormat = "ts"
	}
	if s.RefreshHour < -1 || s.RefreshHour > 23 {
		s.RefreshHour = 4
	}
}

type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // m3u | file | xtream | stalker
	URL      string `json:"url,omitempty"`
	EPGURL   string `json:"epgUrl,omitempty"`
	Server   string `json:"server,omitempty"` // Xtream server or Stalker portal
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Stalker portal identity; see internal/stalker.
	MAC       string           `json:"mac,omitempty"`
	Serial    string           `json:"serial,omitempty"`
	DeviceID  string           `json:"deviceId,omitempty"`
	DeviceID2 string           `json:"deviceId2,omitempty"`
	Stalker   *stalker.Session `json:"stalker,omitempty"`
	Account   *Account         `json:"account,omitempty"`

	DetectedEPGURLs []string `json:"detectedEpgUrls,omitempty"`
	// DetectedEPGURL is the single-guide field of v0.1 states; read only.
	DetectedEPGURL string     `json:"detectedEpgUrl,omitempty"`
	ChannelCount   int        `json:"channelCount"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
	Error          string     `json:"error,omitempty"`
	Refreshing     bool       `json:"refreshing"`
}

// epgURLs lists the guides to try, best first: the ones typed by the user
// win over the ones the playlist or provider advertises.
func (s *Source) epgURLs() []string {
	if s.EPGURL != "" {
		return playlist.SplitURLs(s.EPGURL)
	}
	if len(s.DetectedEPGURLs) > 0 {
		return s.DetectedEPGURLs
	}
	return playlist.SplitURLs(s.DetectedEPGURL)
}

type ChannelRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

func (r ChannelRef) key() string { return r.Source + "/" + r.ID }

type Bouquet struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Channels []ChannelRef `json:"channels"`
}

type EPGStatus struct {
	Running    bool       `json:"running"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
	Wanted     int        `json:"wanted"`
	Matched    int        `json:"matched"`
	Programmes int        `json:"programmes"`
	Sig        string     `json:"sig,omitempty"`
}

type State struct {
	Sources         []*Source   `json:"sources"`
	Bouquets        []*Bouquet  `json:"bouquets"`
	Settings        Settings    `json:"settings"`
	Dirty           bool        `json:"dirty"`
	LastApply       *time.Time  `json:"lastApply,omitempty"`
	LastApplyError  string      `json:"lastApplyError,omitempty"`
	LastAutoRefresh *time.Time  `json:"lastAutoRefresh,omitempty"`
	EPG             EPGStatus   `json:"epg"`
	Picons          PiconStatus `json:"picons"`
}

type App struct {
	cfg Config
	log *log.Logger
	bg  context.Context

	mu       sync.Mutex
	st       State
	channels map[string][]playlist.Channel // by source id
	index    map[string]map[string]int     // source id -> channel id -> position
	seq      uint64                        // bumped on every change that affects the TV

	applyMu       sync.Mutex
	epgMu         sync.Mutex
	piconMu       sync.Mutex
	refreshingAll atomic.Bool
	stalkers      map[string]*stalkerEntry // by source id
	bgWork        sync.WaitGroup           // background EPG builds, so tests and shutdown can wait
}

func New(cfg Config, defaults Settings, logger *log.Logger) (*App, error) {
	a := &App{
		cfg:      cfg,
		log:      logger,
		bg:       context.Background(),
		channels: map[string][]playlist.Channel{},
		index:    map[string]map[string]int{},
		stalkers: map[string]*stalkerEntry{},
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "channels"), 0o755); err != nil {
		return nil, err
	}
	a.st.Settings = defaults
	data, err := os.ReadFile(a.statePath())
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &a.st); err != nil {
			return nil, err
		}
	case !os.IsNotExist(err):
		return nil, err
	}
	a.st.Settings.normalize()
	a.st.EPG.Running = false
	a.st.Picons.Running = false
	for _, s := range a.st.Sources {
		s.Refreshing = false
		a.loadChannels(s.ID)
	}
	if len(a.st.Bouquets) == 0 {
		a.st.Bouquets = []*Bouquet{{ID: newID(), Name: "Избранное"}}
	}
	return a, a.save()
}

func (a *App) statePath() string { return filepath.Join(a.cfg.DataDir, "state.json") }

func (a *App) cachePath(sourceID string) string {
	return filepath.Join(a.cfg.DataDir, "channels", sourceID+".json")
}

// save persists state; callers hold a.mu.
func (a *App) save() error {
	data, err := json.MarshalIndent(&a.st, "", "  ")
	if err != nil {
		return err
	}
	if err := enigma.WriteFileAtomic(a.statePath(), data); err != nil {
		a.log.Printf("не удалось сохранить состояние: %v", err)
		return err
	}
	return nil
}

func (a *App) loadChannels(sourceID string) {
	data, err := os.ReadFile(a.cachePath(sourceID))
	if err != nil {
		return
	}
	var chans []playlist.Channel
	if err := json.Unmarshal(data, &chans); err != nil {
		a.log.Printf("кэш каналов %s повреждён: %v", sourceID, err)
		return
	}
	a.setChannels(sourceID, chans)
}

func (a *App) setChannels(sourceID string, chans []playlist.Channel) {
	idx := make(map[string]int, len(chans))
	for i, c := range chans {
		idx[c.ID] = i
	}
	a.channels[sourceID] = chans
	a.index[sourceID] = idx
}

func (a *App) channel(sourceID, id string) *playlist.Channel {
	i, ok := a.index[sourceID][id]
	if !ok {
		return nil
	}
	return &a.channels[sourceID][i]
}

func (a *App) source(id string) *Source {
	for _, s := range a.st.Sources {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (a *App) bouquet(id string) *Bouquet {
	for _, b := range a.st.Bouquets {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// touch records a change that the TV does not know about yet.
func (a *App) touch() {
	a.seq++
	a.st.Dirty = true
}

func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- read-only views for the API ----

type RefView struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Logo    string `json:"logo,omitempty"`
	Missing bool   `json:"missing,omitempty"`
}

type BouquetView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Channels []RefView `json:"channels"`
}

type StateView struct {
	Version            string        `json:"version"`
	Sources            []*Source     `json:"sources"`
	Bouquets           []BouquetView `json:"bouquets"`
	Settings           Settings      `json:"settings"`
	Dirty              bool          `json:"dirty"`
	LastApply          *time.Time    `json:"lastApply,omitempty"`
	LastApplyError     string        `json:"lastApplyError,omitempty"`
	EPG                EPGStatus     `json:"epg"`
	EPGImportInstalled bool          `json:"epgImportInstalled"`
	Picons             PiconStatus   `json:"picons"`
	TabletURL          string        `json:"tabletUrl,omitempty"`
}

func (a *App) View() StateView {
	urls := a.TabletURLs()
	a.mu.Lock()
	defer a.mu.Unlock()
	v := StateView{
		Version:            a.cfg.Version,
		Sources:            make([]*Source, len(a.st.Sources)),
		Settings:           a.st.Settings,
		Dirty:              a.st.Dirty,
		LastApply:          a.st.LastApply,
		LastApplyError:     a.st.LastApplyError,
		EPG:                a.st.EPG,
		EPGImportInstalled: a.epgImportInstalled(),
		Picons:             a.st.Picons,
	}
	if len(urls) > 0 {
		v.TabletURL = urls[0]
	}
	for i, s := range a.st.Sources {
		cp := *s
		v.Sources[i] = &cp
	}
	for _, b := range a.st.Bouquets {
		bv := BouquetView{ID: b.ID, Name: b.Name, Channels: make([]RefView, 0, len(b.Channels))}
		for _, r := range b.Channels {
			rv := RefView{Source: r.Source, ID: r.ID, Name: r.Name}
			if ch := a.channel(r.Source, r.ID); ch != nil {
				rv.Name, rv.Logo = ch.Name, ch.Logo
			} else {
				rv.Missing = true
			}
			bv.Channels = append(bv.Channels, rv)
		}
		v.Bouquets = append(v.Bouquets, bv)
	}
	return v
}

func (a *App) epgImportInstalled() bool {
	if a.cfg.PluginDir != "" {
		if _, err := os.Stat(filepath.Join(a.cfg.PluginDir, "EPGImport")); err == nil {
			return true
		}
	}
	st, err := os.Stat(a.cfg.EPGImportDir)
	return err == nil && st.IsDir()
}
